package blockchain

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/p9c/pkg/app/slog"
	"math"
	"math/big"
	"time"

	config "github.com/p9c/pod/pkg/chain/config"
	"github.com/p9c/pod/pkg/chain/config/netparams"
	"github.com/p9c/pod/pkg/chain/fork"
	"github.com/p9c/pod/pkg/chain/hardfork"
	chainhash "github.com/p9c/pod/pkg/chain/hash"
	txscript "github.com/p9c/pod/pkg/chain/tx/script"
	"github.com/p9c/pod/pkg/chain/wire"
	"github.com/p9c/pod/pkg/util"
)

const (
	// MaxTimeOffsetSeconds is the maximum number of seconds a block time is
	// allowed to be ahead of the current time.  This is currently 2 hours.
	MaxTimeOffsetSeconds = 2 * 60 * 60
	// MinCoinbaseScriptLen is the minimum length a coinbase script can be.
	MinCoinbaseScriptLen = 2
	// MaxCoinbaseScriptLen is the maximum length a coinbase script can be.
	MaxCoinbaseScriptLen = 100
	// medianTimeBlocks is the number of previous blocks which should be used
	// to calculate the median time used to validate block timestamps.
	medianTimeBlocks = 11
	// serializedHeightVersion is the block version which changed block
	// coinbases to start with the serialized block height.
	serializedHeightVersion = 2
)

var (
	// baseSubsidy is the starting subsidy amount for mined blocks.
	// This value is halved every SubsidyHalvingInterval blocks.
	baseSubsidy = 2 * util.SatoshiPerBitcoin
	// block91842Hash is one of the two nodes which violate the rules set
	// forth in BIP0030.  It is defined as a package level variable to avoid
	// the need to create a new instance every time a check is needed.
	block91842Hash = newHashFromStr(
		"00000000000a4d0a398161ffc163c503763b1f4360639393e0e4c8e300e0caec")
	// block91880Hash is one of the two nodes which violate the rules set
	// forth in BIP0030.  It is defined as a package level variable to avoid
	// the need to create a new instance every time a check is needed.
	block91880Hash = newHashFromStr(
		"00000000000743f190a18c5577a3c2d2a1f610ae9601ac046a38084ccb7cd721")
	// zeroHash is the zero value for a chainhash.
	// Hash and is defined as a package level variable to avoid the need to
	// create a new instance every time a check is needed.
	zeroHash chainhash.Hash
)

func // checkConnectBlock performs several checks to confirm connecting the
// passed block to the chain represented by the passed view does not violate
// any rules. In addition the passed view is updated to spend all of the
// referenced outputs and add all of the new utxos created by block.
// Thus the view will represent the state of the chain as if the block were
// actually connected and consequently the best hash for the view is also
// updated to passed block. An example of some of the checks performed are
// ensuring connecting the block would not cause any duplicate transaction
// hashes for old transactions that aren't already fully spent,
// double spends, exceeding the maximum allowed signature operations per
// block, invalid values in relation to the expected block subsidy,
// or fail transaction script validation.
// The CheckConnectBlockTemplate function makes use of this function to
// perform the bulk of its work.
// The only difference is this function accepts a node which may or may not
// require reorganization to connect it to the main chain whereas
// CheckConnectBlockTemplate creates a new node which specifically connects
// to the end of the current main chain and then calls this function with
// that node.
// This function MUST be called with the chain state lock held (
// for writes).
(b *BlockChain) checkConnectBlock(node *BlockNode, block *util.Block, view *UtxoViewpoint, stxos *[]SpentTxOut) (err error) {
	// Trace("check can connect block")
	// If the side chain blocks end up in the database,
	// a call to CheckBlockSanity should be done here in case a previous
	// version allowed a block that is no longer valid.  However,
	// since the implementation only currently uses memory for the side chain
	// blocks, it isn't currently necessary.
	// The coinbase for the Genesis block is not spendable,
	// so just return an error now.
	if node.hash.IsEqual(b.params.GenesisHash) {
		err = ruleError(ErrMissingTxOut, "the coinbase for the genesis block is not spendable")
		return
	}
	// Ensure the view is for the node being checked.
	parentHash := &block.MsgBlock().Header.PrevBlock
	if !view.BestHash().IsEqual(parentHash) {
		err = AssertError(fmt.Sprintf(
			"inconsistent view when checking block connection: best hash is %v instead of expected %v",
			view.BestHash(), parentHash))
		return
	}
	// BIP0030 added a rule to prevent blocks which contain duplicate
	// transactions that 'overwrite' older transactions which are not fully
	// spent.  See the documentation for checkBIP0030 for more details.
	// There are two blocks in the chain which violate this rule,
	// so the check must be skipped for those blocks.
	// The isBIP0030Node function is used to determine if this block is one
	// of the two blocks that must be skipped. In addition, as of BIP0034,
	// duplicate coinbases are no longer possible due to its requirement for including the block height in the coinbase and thus it is no longer possible to create transactions that 'overwrite' older ones.  Therefore, only enforce the rule if BIP0034 is not yet active.  This is a useful optimization because the BIP0030 check is expensive since it involves a ton of cache misses in the utxoset.
	if !isBIP0030Node(node) && (node.height < b.params.BIP0034Height) {
		if err = b.checkBIP0030(node, block, view); slog.Check(err) {
			return
		}
	}
	// Load all of the utxos referenced by the inputs for all transactions in
	// the block don't already exist in the utxo view from the database.
	// These utxo entries are needed for verification of things such as
	// transaction inputs, counting pay-to-script-hashes, and scripts.
	if err = view.fetchInputUtxos(b.db, block); slog.Check(err) {
		return
	}
	// BIP0016 describes a pay-to-script-hash type that is considered a
	// "standard" type.  The rules for this BIP only apply to transactions
	// after the timestamp defined by txscript.Bip16Activation.
	// See https://en.bitcoin.it/wiki/BIP_0016 for more details.
	enforceBIP0016 := node.timestamp >= txscript.Bip16Activation.Unix()
	// Query for the Version Bits state for the segwit soft-fork deployment.
	// If segwit is active, we'll switch over to enforcing all the new rules.
	var segwitState ThresholdState
	if segwitState, err = b.deploymentState(node.parent, config.DeploymentSegwit); slog.Check(err) {
		return
	}
	enforceSegWit := segwitState == ThresholdActive
	// The number of signature operations must be less than the maximum
	// allowed per block.  Note that the preliminary sanity checks on a block
	// also include a check similar to this one,
	// but this check expands the count to include a precise count of pay-to
	// -script-hash signature operations in each of the input transaction
	// public key scripts.
	transactions := block.Transactions()
	totalSigOpCost := 0
	for i, tx := range transactions {
		// Since the first (and only the first) transaction has already been
		// verified to be a coinbase transaction,
		// use i == 0 as an optimization for the flag to countP2SHSigOps for
		// whether or not the transaction is a coinbase transaction rather
		// than having to do a full coinbase check again.
		var sigOpCost int
		if sigOpCost, err = GetSigOpCost(tx, i == 0, view, enforceBIP0016, enforceSegWit); slog.Check(err) {
			return
		}
		// Check for overflow or going over the limits.
		// We have to do this on every loop iteration to avoid overflow.
		lastSigOpCost := totalSigOpCost
		totalSigOpCost += sigOpCost
		if totalSigOpCost < lastSigOpCost || totalSigOpCost > MaxBlockSigOpsCost {
			err = ruleError(ErrTooManySigOps, fmt.Sprintf(
				"block contains too many signature operations - got %v, max %v",
				totalSigOpCost, MaxBlockSigOpsCost))
			return
		}
	}
	// Perform several checks on the inputs for each transaction.
	// Also accumulate the total fees.
	// This could technically be combined with the loop above instead of
	// running another loop over the transactions,
	// but by separating it we can avoid running the more expensive (
	// though still relatively cheap as compared to running the scripts
	// ) checks against all the inputs when the signature operations are out
	// of bounds.
	var totalFees int64
	for _, tx := range transactions {
		var txFee int64
		if txFee, err = CheckTransactionInputs(tx, node.height, view, b.params); slog.Check(err) {
			return
		}
		// Sum the total fees and ensure we don't overflow the accumulator.
		lastTotalFees := totalFees
		totalFees += txFee
		if totalFees < lastTotalFees {
			err = ruleError(ErrBadFees, "total fees for block overflows accumulator")
			return
		}
		// Add all of the outputs for this transaction which are not provably unspendable as available utxos.  Also,
		// the passed spent txos slice is updated to contain an entry for each spent txout in the order each transaction
		// spends them.
		if err = view.connectTransaction(tx, node.height, stxos); slog.Check(err) {
			return
		}
	}
	// Trace("checking coinbase value")
	// The total output values of the coinbase transaction must not exceed
	// the expected subsidy value plus total transaction fees gained from
	// mining the block.  It is safe to ignore overflow and out of range
	// errors here because those error conditions would have already been
	// caught by checkTransactionSanity.
	var totalSatoshiOut int64
	for _, txOut := range transactions[0].MsgTx().TxOut {
		totalSatoshiOut += txOut.Value
	}
	expectedSatoshiOut := CalcBlockSubsidy(node.height, b.params, node.version) +
		totalFees
	if totalSatoshiOut > expectedSatoshiOut {
		err = ruleError(ErrBadCoinbaseValue, fmt.Sprintf(
			"coinbase transaction for block pays %v which is more than expected value of %v",
			totalSatoshiOut, expectedSatoshiOut))
		return
	}
	// Trace("checking for hard fork block")
	// if this is the hard fork activation height special disbursement coinbase
	// must match the specifications in pkg/chain/hardfork/subsidy.go
	if node.height == fork.List[1].ActivationHeight &&
		b.params.Net == wire.MainNet ||
		node.height == fork.List[1].TestnetStart &&
			b.params.Net == wire.TestNet3 {
		slog.Trace("checking contents of hardfork coinbase tx")
		var btx *util.Tx
		if btx, err = block.Tx(0); slog.Check(err) {
		}
		payees := hardfork.Payees
		if b.params.Net == wire.TestNet3 {
			payees = hardfork.TestnetPayees
		}
		txo := btx.MsgTx().TxOut
		for i := range payees {
			if txo[i].Value != int64(payees[i].Amount) {
				err = ruleError(ErrBadCoinbaseValue,
					"hardfork coinbase does not pay correct amount to payees list")
				return
			}
			// we will save processing not decode these, we know the addresses
			// should appear in a specific section of the pkscript
			proposed := txo[i].PkScript[3:23]
			proper := payees[i].Address.ScriptAddress()
			for i := range proposed {
				if proposed[i] != proper[i] {
					return ruleError(ErrBadCoinbaseValue,
						"hardfork coinbase does not pay to correct addresses")
				}
			}
		}
		remTx := txo[len(payees):]
		coreAmount := hardfork.CoreAmount
		if b.params.Net == wire.TestNet3 {
			coreAmount = hardfork.TestnetCoreAmount
		}
		if remTx[0].Value != int64(coreAmount) {
			return ruleError(ErrBadCoinbaseValue,
				"hardfork coinbase does not pay correct amount to dev fund multisig address")
		}

		corePubKeys := hardfork.CorePubkeyBytes
		if b.params.Net == wire.TestNet3 {
			corePubKeys = hardfork.TestnetCorePubkeyBytes
		}
		remainder := remTx[0].PkScript[2:]
		for i := range corePubKeys {
			if len(remainder) < len(corePubKeys[i]) {
				return ruleError(ErrBadCoinbaseValue,
					"hardfork coinbase is missing pubkeys for dev fund multisig address")
			}
			for j := range corePubKeys[i] {
				if remainder[j] != corePubKeys[i][j] {
					return ruleError(ErrBadCoinbaseValue,
						"hardfork coinbase has incorrect pubkey in dev fund multisig address")
				}
			}
			// before each pubkey is the length, one byte,
			// so we snip that off for
			// the next round
			remainder = remainder[len(corePubKeys[i])+1:]
		}
		// If the miner put a nonstandard reward in the remainder it will mean
		// the total is incorrect and it will be caught as being over the
		// allowed
		// coinbase value for this specific block.
		// Under, of course, doesn't matter.
	}
	// Trace("checking for checkpoints")
	// Don't run scripts if this node is before the latest known good
	// checkpoint since the validity is verified via the checkpoints (
	// all transactions are included in the merkle root hash and any changes
	// will therefore be detected by the next checkpoint).
	// This is a huge optimization because running the scripts is the most
	// time consuming portion of block handling.
	checkpoint := b.LatestCheckpoint()
	runScripts := true
	if checkpoint != nil && node.height <= checkpoint.Height {
		runScripts = false
	}
	// Trace("BlockC created after the BIP0016 activation time")
	// BlockC created after the BIP0016 activation time need to have the pay
	// -to-script-hash checks enabled.
	var scriptFlags txscript.ScriptFlags
	if enforceBIP0016 {
		scriptFlags |= txscript.ScriptBip16
	}
	// Trace("Enforce DER signatures")
	// Enforce DER signatures for block versions 3+ once the historical
	// activation threshold has been reached.  This is part of BIP0066.
	blockHeader := &block.MsgBlock().Header
	if blockHeader.Version >= 3 && node.height >= b.params.BIP0066Height {
		scriptFlags |= txscript.ScriptVerifyDERSignatures
	}
	// Trace("Enforce CHECKLOCKTIMEVERIFY")
	// Enforce CHECKLOCKTIMEVERIFY for block versions 4+ once the historical
	// activation threshold has been reached.  This is part of BIP0065.
	if blockHeader.Version >= 4 && node.height >= b.params.BIP0065Height {
		scriptFlags |= txscript.ScriptVerifyCheckLockTimeVerify
	}
	// Trace("Enforce CHECKSEQUENCEVERIFY during all block validation checks")
	// Enforce CHECKSEQUENCEVERIFY during all block validation checks once
	// the soft-fork deployment is fully active.
	var csvState ThresholdState
	if csvState, err = b.deploymentState(node.parent, config.DeploymentCSV); slog.Check(err) {
		return
	}
	if csvState == ThresholdActive {
		// If the CSV soft-fork is now active,
		// then modify the scriptFlags to ensure that the CSV op code is
		// properly validated during the script checks bleow.
		scriptFlags |= txscript.ScriptVerifyCheckSequenceVerify
		// We obtain the MTP of the *previous* block in order to determine if
		// transactions in the current block are final.
		medianTime := node.parent.CalcPastMedianTime()
		// Additionally, if the CSV soft-fork package is now active,
		// then we also enforce the relative sequence number based
		// lock-times within the inputs of all transactions in this candidate
		// block.
		for _, tx := range block.Transactions() {
			// A transaction can only be included within a block once the
			// sequence locks of *all* its inputs are active.
			var sequenceLock *SequenceLock
			if sequenceLock, err = b.calcSequenceLock(node, tx, view,
				false); slog.Check(err) {
				return
			}
			if !SequenceLockActive(sequenceLock, node.height, medianTime) {
				err = ruleError(ErrUnfinalizedTx, fmt.Sprintf(
					"block contains transaction whose input sequence locks are not met"))
				return
			}
		}
	}
	// Enforce the segwit soft-fork package once the soft-fork has shifted
	// into the "active" version bits state.
	if enforceSegWit {
		scriptFlags |= txscript.ScriptVerifyWitness
		scriptFlags |= txscript.ScriptStrictMultiSig
	}
	// Now that the inexpensive checks are done and have passed,
	// verify the transactions are actually allowed to spend the coins by
	// running the expensive ECDSA signature check scripts.
	// Doing this last helps prevent CPU exhaustion attacks.
	if runScripts {
		// Trace("running scripts")
		if err = checkBlockScripts(block, view, scriptFlags, b.sigCache,
			b.hashCache); slog.Check(err) {
			return
		}
	}
	// Trace("connecting block")
	// Update the best hash for view to include this block since all of its
	// transactions have been connected.
	view.SetBestHash(&node.hash)
	// Trace("block connected")
	return
}

// CheckConnectBlockTemplate fully validates that connecting the passed block
// to the main chain does not violate any consensus rules, aside from the proof
// of work requirement. The block must connect to the current tip of the main
// chain. This function is safe for concurrent access.
func (b *BlockChain) CheckConnectBlockTemplate(workerNumber uint32, block *util.Block) (err error) {
	algo := block.MsgBlock().Header.Version
	// Trace("algo ", algo)
	height := block.Height()
	// Trace("height ", height)
	algoName := fork.GetAlgoName(algo, height)
	// Trace("algoName ", algoName)
	powLimit := fork.GetMinDiff(algoName, height)
	slog.Tracef("CheckConnectBlockTemplate %08x %064x", block.MsgBlock().Header.Bits, powLimit)
	// Skip the proof of work check as this is just a block template.
	flags := BFNoPoWCheck
	// This only checks whether the block can be connected to the tip of the current chain.
	b.chainLock.Lock()
	defer b.chainLock.Unlock()
	tip := b.BestChain.Tip()
	slog.Tracef("%+v", tip)
	header := block.MsgBlock().Header
	if tip.hash != header.PrevBlock {
		err = ruleError(ErrPrevBlockNotBest, fmt.Sprintf(
			"previous block must be the current chain tip %v, instead got %v", tip.hash, header.PrevBlock))
		return
	}
	if err = checkBlockSanity(block, powLimit, b.timeSource, flags, false, block.Height()); slog.Check(err) {
		return
	}
	if err = b.checkBlockContext(workerNumber, block, tip, flags, true); slog.Check(err) {
		return
	}
	// Leave the spent txouts entry nil in the state since the information is
	// not needed and thus extra work can be avoided.
	view := NewUtxoViewpoint()
	view.SetBestHash(&tip.hash)
	newNode := NewBlockNode(&header, tip)
	return b.checkConnectBlock(newNode, block, view, nil)
}

// checkBIP0030 ensures blocks do not contain duplicate transactions which
// 'overwrite' older transactions that are not fully spent.
// This prevents an attack where a coinbase and all of its dependent
// transactions could be duplicated to effectively revert the overwritten
// transactions to a single confirmation thereby making them vulnerable to a
// double spend.
// For more details, see https://github.
// com/bitcoin/bips/blob/master/bip-0030.mediawiki and http://r6.
// ca/blog/20120206T005236Z.html.
// This function MUST be called with the chain state lock held (for reads).
func (b *BlockChain) checkBIP0030(node *BlockNode, block *util.Block, view *UtxoViewpoint) (err error) {
	// Fetch utxos for all of the transaction ouputs in this block.
	// Typically, there will not be any utxos for any of the outputs.
	fetchSet := make(map[wire.OutPoint]struct{})
	for _, tx := range block.Transactions() {
		prevOut := wire.OutPoint{Hash: *tx.Hash()}
		for txOutIdx := range tx.MsgTx().TxOut {
			prevOut.Index = uint32(txOutIdx)
			fetchSet[prevOut] = struct{}{}
		}
	}
	if err = view.fetchUtxos(b.db, fetchSet); slog.Check(err) {
		return
	}
	// Duplicate transactions are only allowed if the previous transaction is
	// fully spent.
	for outpoint := range fetchSet {
		utxo := view.LookupEntry(outpoint)
		if utxo != nil && !utxo.IsSpent() {
			err = ruleError(ErrOverwriteTx, fmt.Sprintf(
				"tried to overwrite transaction %v at block height %d that is not fully spent",
				outpoint.Hash, utxo.BlockHeight()))
			return
		}
	}
	return
}

// checkBlockContext peforms several validation checks on the block which
// depend on its position within the block chain.
// The flags modify the behavior of this function as follows:
// - BFFastAdd: The transaction are not checked to see if they are finalized
// and the somewhat expensive BIP0034 validation is not performed.
// The flags are also passed to checkBlockHeaderContext.
// See its documentation for how the flags modify its behavior.
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) checkBlockContext(workerNumber uint32, block *util.Block,
	prevNode *BlockNode, flags BehaviorFlags, DoNotCheckPow bool) (err error) {
	// Warn("checkBlockContext")
	// Perform all block header related validation checks.
	header := &block.MsgBlock().Header
	if err = b.checkBlockHeaderContext(workerNumber, header, prevNode, flags); slog.Check(err) {
		return
	}
	fastAdd := flags&BFFastAdd == BFFastAdd
	if !fastAdd {
		// Obtain the latest state of the deployed CSV soft-fork in order to
		// properly guard the new validation behavior based on the current BIP 9
		// version bits state.
		var csvState ThresholdState
		if csvState, err = b.deploymentState(prevNode, config.DeploymentCSV); slog.Check(err) {
			return
		}
		// Once the CSV soft-fork is fully active, we'll switch to using the
		// current median time past of the past block's timestamps for all
		// lock-time based checks.
		blockTime := header.Timestamp
		if csvState == ThresholdActive {
			blockTime = prevNode.CalcPastMedianTime()
		}
		// The height of this block is one more than the referenced previous
		// block.
		blockHeight := prevNode.height + 1
		// Ensure all transactions in the block are finalized.
		for _, tx := range block.Transactions() {
			if !IsFinalizedTransaction(tx, blockHeight, blockTime) {
				err = ruleError(ErrUnfinalizedTx, fmt.Sprintf(
					"block contains unfinalized transaction %v", tx.Hash()))
				slog.Debug(err)
				return
			}
		}
		// Ensure coinbase starts with serialized block heights for blocks whose
		// version is the serializedHeightVersion or newer once a majority of
		// the network has upgraded.  This is part of BIP0034.
		if ShouldHaveSerializedBlockHeight(header) && blockHeight >= b.params.BIP0034Height {
			coinbaseTx := block.Transactions()[0]
			if err = checkSerializedHeight(coinbaseTx, blockHeight); slog.Check(err) {
				return
			}
		}
		// Query for the Version Bits state for the segwit soft-fork deployment.
		// If segwit is active,
		// we'll switch over to enforcing all the new rules.
		var segwitState ThresholdState
		if segwitState, err = b.deploymentState(prevNode, config.DeploymentSegwit); slog.Check(err) {
			return err
		}
		// If segwit is active,
		// then we'll need to fully validate the new witness
		// commitment for adherence to the rules.
		if segwitState == ThresholdActive {
			// Validate the witness commitment (if any) within the block.  This
			// involves asserting that if the coinbase contains the special
			// commitment output,
			// then this merkle root matches a computed merkle
			// root of all the wtxid's of the transactions within the block. In
			// addition, various other checks against the coinbase's witness
			// stack.
			if err = ValidateWitnessCommitment(block); slog.Check(err) {
				return
			}
			// Once the witness commitment, witness nonce, and sig op cost have
			// been validated, we can finally assert that the block's weight
			// doesn't exceed the current consensus parameter.
			blockWeight := GetBlockWeight(block)
			if blockWeight > MaxBlockWeight {
				err = ruleError(ErrBlockWeightTooHigh, fmt.Sprintf(
					"block's weight metric is too high - got %v, max %v", blockWeight, MaxBlockWeight))
				slog.Error(err)
				return
			}
		}
	}
	return
}

// checkBlockHeaderContext performs several validation checks on the block
// header which depend on its position within the block chain.
// The flags modify the behavior of this function as follows:
//  - BFFastAdd: All checks except those involving comparing the header
//  against the checkpoints are not performed.
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) checkBlockHeaderContext(workerNumber uint32, header *wire.BlockHeader, prevNode *BlockNode,
	flags BehaviorFlags) (err error) {
	// Warn("checking block header context")
	if prevNode == nil {
		return
	}
	if !(flags&BFFastAdd == BFFastAdd) {
		// Ensure the difficulty specified in the block header matches the
		// calculated difficulty based on the previous block and difficulty
		// retarget rules.
		//slog.Infof("algo %s %d %8x %d",
		//	fork.GetAlgoName(header.Version, prevNode.height+1), header.Version, header.Bits, prevNode.height+1)
		var expectedDifficulty uint32
		if expectedDifficulty, err = b.calcNextRequiredDifficulty(
			workerNumber, prevNode, header.Timestamp, fork.GetAlgoName(header.Version, prevNode.height+1), true,
		); slog.Check(err) {
			return
		}
		blockDifficulty := header.Bits
		if blockDifficulty != expectedDifficulty {
			err = ruleError(ErrUnexpectedDifficulty,
				fmt.Sprintf("block difficulty of %08x %064x is not the expected value of %08x %064x",
					blockDifficulty, CompactToBig(blockDifficulty), expectedDifficulty, CompactToBig(expectedDifficulty)),
			)
			slog.Error(err)
			return
		}
		// Ensure the timestamp for the block header is after the median time
		// of the last several blocks (medianTimeBlocks).
		medianTime := prevNode.CalcPastMedianTime()
		if !header.Timestamp.After(medianTime) {
			err = ruleError(ErrTimeTooOld,
				fmt.Sprintf("block timestamp of %v is not after expected %v", header.Timestamp, medianTime))
			slog.Error(err)
			return
		}
	}

	// The height of this block is one more than the referenced previous block.
	blockHeight := prevNode.height + 1
	// Ensure chain matches up to predetermined checkpoints.
	blockHash := header.BlockHash()
	if !b.verifyCheckpoint(blockHeight, &blockHash) {
		err = ruleError(ErrBadCheckpoint,
			fmt.Sprintf("block at height %d does not match checkpoint hash", blockHeight))
		slog.Error(err)
		return
	}
	// Find the previous checkpoint and prevent blocks which fork the main
	// chain before it.  This prevents storage of new, otherwise valid,
	// blocks which build off of old blocks that are likely at a much easier
	// difficulty and therefore could be used to waste cache and disk space.
	var checkpointNode *BlockNode
	if checkpointNode, err = b.findPreviousCheckpoint(); slog.Check(err) {
		return
	}
	if checkpointNode != nil && blockHeight < checkpointNode.height {
		err = ruleError(ErrForkTooOld, fmt.Sprintf(
			"block at height %d forks the main chain before the previous checkpoint at height %d",
			blockHeight, checkpointNode.height),
		)
		slog.Error(err)
		return
	}
	// Reject outdated block versions once a majority of the network has
	// upgraded.  These were originally voted on by BIP0034, BIP0065,
	// and BIP0066.
	// netparams := b.netparams
	// if header.Version < 2 && blockHeight >= netparams.BIP0034Height ||
	// 	header.Version < 3 && blockHeight >= netparams.BIP0066Height ||
	// 	header.Version < 4 && blockHeight >= netparams.BIP0065Height {
	// 	str := "new blocks with version %d are no longer valid"
	// 	str = fmt.Sprintf(str, header.Version)
	// 	return ruleError(ErrBlockVersionTooOld, str)
	// }
	return
}

// CalcBlockSubsidy returns the subsidy amount a block at the provided
// height should have. This is mainly used for determining how much the
// coinbase for newly generated blocks awards as well as validating the
// coinbase for blocks has the expected value.
// The subsidy is halved every SubsidyReductionInterval blocks.
// Mathematically this is: baseSubsidy / 2^(
// height/SubsidyReductionInterval) At the target block generation rate for
// the main network, this is approximately every 4 years.
func CalcBlockSubsidy(height int32, chainParams *netparams.Params, version int32) (r int64) {
	if chainParams.SubsidyReductionInterval == 0 {
		return int64(baseSubsidy)
	}
	// Equivalent to: baseSubsidy / 2^(height/subsidyHalvingInterval)
	switch fork.GetCurrent(height) {
	case 0:
		return int64(baseSubsidy) >> uint64(height/chainParams.
			SubsidyReductionInterval)
	case 1:
		var total util.Amount
		if (chainParams.Net == wire.MainNet && height == fork.List[1].ActivationHeight) ||
			(chainParams.Net == wire.TestNet3 && height == fork.List[1].TestnetStart) {
			payees := hardfork.Payees
			if chainParams.Net == wire.TestNet3 {
				payees = hardfork.TestnetPayees
			}
			for i := range payees {
				total += payees[i].Amount
			}
			total += util.Amount(CalcBlockSubsidy(height+1, chainParams, version))
			total += hardfork.TestnetCoreAmount
			return int64(total)
		}
		// Plan 9 hard fork prescribes a smooth supply curve made using an
		// exponential decay formula adjusted to fit the previous halving
		// cycle and accounting for the block time difference
		ttpb := float64(fork.List[1].Algos[fork.GetAlgoName(version, height)].VersionInterval)
		r = int64(2.7 * ttpb / 300 * (math.Pow(2.7, -float64(height)*300*9/ttpb/375000.0)) * 100000000 / 9)
	}
	return
}

// CheckBlockSanity performs some preliminary checks on a block to
// ensure it is sane before continuing with block processing.
// These checks are context free.
func CheckBlockSanity(block *util.Block, powLimit *big.Int, timeSource MedianTimeSource, DoNotCheckPow bool,
	height int32) (err error) {
	slog.Trace("CheckBlockSanity powlimit %64x", powLimit)
	return checkBlockSanity(block, powLimit, timeSource, BFNone, DoNotCheckPow, height)
}

// CheckProofOfWork ensures the block header bits which indicate the target
// difficulty is in min/max range and that the block hash is less than the
// target difficulty as claimed.
func CheckProofOfWork(block *util.Block, powLimit *big.Int, height int32) (err error) {
	return checkProofOfWork(&block.MsgBlock().Header, powLimit, BFNone, height)
}

// CheckTransactionInputs performs a series of checks on the inputs to a transaction to ensure they are valid.
// An example of some of the checks include verifying all inputs exist, ensuring the coinbase seasoning requirements are
// met, detecting double spends, validating all values and fees are in the legal range and the total output
// amount doesn't exceed the input amount,  and verifying the signatures to prove the spender was the owner of the coins
// and therefore allowed to spend them.
//
// As it checks the inputs, it also calculates the total fees for the transaction and returns that value.
// NOTE: The transaction MUST have already been sanity checked with the CheckTransactionSanity function prior to calling
// this function.
func CheckTransactionInputs(tx *util.Tx, txHeight int32, utxoView *UtxoViewpoint, chainParams *netparams.
Params) (txFeeInSatoshi int64, err error) {
	// Coinbase transactions have no inputs.
	if IsCoinBase(tx) {
		return
	}
	txHash := tx.Hash()
	var totalSatoshiIn int64
	for txInIndex, txIn := range tx.MsgTx().TxIn {
		// Ensure the referenced input transaction is available.
		utxo := utxoView.LookupEntry(txIn.PreviousOutPoint)
		if utxo == nil || utxo.IsSpent() {
			err = ruleError(ErrMissingTxOut, fmt.Sprintf(
				"output %v referenced from transaction %s:%d either does not exist or has already been spent",
				txIn.PreviousOutPoint, tx.Hash(), txInIndex))
			slog.Debug(err)
			return
		}
		// Ensure the transaction is not spending coins which have not yet
		// reached the required coinbase maturity.
		if utxo.IsCoinBase() {
			originHeight := utxo.BlockHeight()
			blocksSincePrev := txHeight - originHeight
			coinbaseMaturity := int32(chainParams.CoinbaseMaturity)
			if blocksSincePrev < coinbaseMaturity {
				err = ruleError(ErrImmatureSpend, fmt.Sprintf(
					"tried to spend coinbase transaction output %v from height %v "+
						"at height %v before required maturity of %v blocks",
					txIn.PreviousOutPoint, originHeight, txHeight, coinbaseMaturity))
				slog.Error(err)
				return
			}
		}
		// Ensure the transaction amounts are in range.
		// Each of the output values of the input transactions must not be
		// negative or more than the max allowed per transaction.
		// All amounts in a transaction are in a unit value known as a
		// satoshi.  One bitcoin is a quantity of satoshi as defined by the
		// SatoshiPerBitcoin constant.
		originTxSatoshi := utxo.Amount()
		if originTxSatoshi < 0 {
			err = ruleError(ErrBadTxOutValue, fmt.Sprintf(
				"transaction output has negative value of %v", util.Amount(originTxSatoshi)))
			slog.Error(err)
			return
		}
		if originTxSatoshi > int64(util.MaxSatoshi) {
			err = ruleError(ErrBadTxOutValue, fmt.Sprintf(
				"transaction output value of %v is higher than max allowed value of %v",
				util.Amount(originTxSatoshi), util.MaxSatoshi))
			slog.Error(err)
			return
		}
		// The total of all outputs must not be more than the max allowed per
		// transaction.  Also,
		// we could potentially overflow the accumulator so check for overflow.
		lastSatoshiIn := totalSatoshiIn
		totalSatoshiIn += originTxSatoshi
		if totalSatoshiIn < lastSatoshiIn || totalSatoshiIn > int64(util.MaxSatoshi) {
			err = ruleError(ErrBadTxOutValue, fmt.Sprintf(
				"total value of all transaction inputs is %v which is higher than max allowed value of %v",
				totalSatoshiIn, util.MaxSatoshi))
			slog.Error(err)
			return
		}
	}
	// Calculate the total output amount for this transaction.
	// It is safe to ignore overflow and out of range errors here because
	// those error conditions would have already been caught by
	// checkTransactionSanity.
	var totalSatoshiOut int64
	for _, txOut := range tx.MsgTx().TxOut {
		totalSatoshiOut += txOut.Value
	}
	// Ensure the transaction does not spend more than its inputs.
	if totalSatoshiIn < totalSatoshiOut {
		err = ruleError(ErrSpendTooHigh, fmt.Sprintf(
			"total value of all transaction inputs for transaction %v is %v which is less than the amount "+
				"spent of %v", txHash, totalSatoshiIn, totalSatoshiOut))
		slog.Error(err)
		return
	}
	// NOTE: bitcoind checks if the transaction fees are < 0 here,
	// but that is an impossible condition because of the check above that
	// ensures the inputs are >= the outputs.
	txFeeInSatoshi = totalSatoshiIn - totalSatoshiOut
	return
}

// CheckTransactionSanity performs some preliminary checks on a transaction
// to ensure it is sane.  These checks are context free.
func CheckTransactionSanity(tx *util.Tx) (err error) {
	// A transaction must have at least one input.
	msgTx := tx.MsgTx()
	if len(msgTx.TxIn) == 0 {
		err = ruleError(ErrNoTxInputs, "transaction has no inputs")
		slog.Error(err)
		return
	}
	// A transaction must have at least one output.
	if len(msgTx.TxOut) == 0 {
		err = ruleError(ErrNoTxOutputs, "transaction has no outputs")
		slog.Error(err)
		return
	}
	// A transaction must not exceed the maximum allowed block payload when serialized.
	serializedTxSize := tx.MsgTx().SerializeSizeStripped()
	if serializedTxSize > MaxBlockBaseSize {
		err = ruleError(ErrTxTooBig, fmt.Sprintf(
			"serialized transaction is too big - got %d, max %d", serializedTxSize, MaxBlockBaseSize))
		slog.Error(err)
		return
	}
	// Ensure the transaction amounts are in range.
	// Each transaction output must not be negative or more than the max
	// allowed per transaction.  Also,
	// the total of all outputs must abide by the same restrictions.
	// All amounts in a transaction are in a unit value known as a satoshi.
	// One bitcoin is a quantity of satoshi as defined by the
	// SatoshiPerBitcoin constant.
	var totalSatoshi int64
	for _, txOut := range msgTx.TxOut {
		satoshi := txOut.Value
		if satoshi < 0 {
			err = ruleError(ErrBadTxOutValue, fmt.Sprintf("transaction output has negative value of %v", satoshi))
			slog.Error(err)
			return
		}
		if satoshi > int64(util.MaxSatoshi) {
			err = ruleError(ErrBadTxOutValue, fmt.Sprintf(
				"transaction output value of %v is higher than max allowed value of %v",
				satoshi, util.MaxSatoshi))
			slog.Error(err)
			return
		}
		// Two's complement int64 overflow guarantees that any overflow is
		// detected and reported.  This is impossible for Bitcoin,
		// but perhaps possible if an alt increases the total money supply.
		totalSatoshi += satoshi
		if totalSatoshi < 0 {
			err = ruleError(ErrBadTxOutValue, fmt.Sprintf(
				"total value of all transaction outputs exceeds max allowed value of %v", util.MaxSatoshi))
			slog.Error(err)
			return
		}
		if totalSatoshi > int64(util.MaxSatoshi) {
			err = ruleError(ErrBadTxOutValue, fmt.Sprintf(
				"total value of all transaction outputs is %v which is higher than max allowed value of %v",
				totalSatoshi, util.MaxSatoshi))
			slog.Error(err)
			return
		}
	}
	// Check for duplicate transaction inputs.
	existingTxOut := make(map[wire.OutPoint]struct{})
	for _, txIn := range msgTx.TxIn {
		if _, exists := existingTxOut[txIn.PreviousOutPoint]; exists {
			return ruleError(ErrDuplicateTxInputs, "transaction "+
				"contains duplicate inputs")
		}
		existingTxOut[txIn.PreviousOutPoint] = struct{}{}
	}
	// Coinbase script length must be between min and max length.
	if IsCoinBase(tx) {
		scriptLength := len(msgTx.TxIn[0].SignatureScript)
		if scriptLength < MinCoinbaseScriptLen || scriptLength > MaxCoinbaseScriptLen {
			err = ruleError(ErrBadCoinbaseScriptLen, fmt.Sprintf(
				"coinbase transaction script length of %d is out of range (min: %d, max: %d)",
				scriptLength, MinCoinbaseScriptLen, MaxCoinbaseScriptLen))
			slog.Error(err)
			return
		}
	} else {
		// Previous transaction outputs referenced by the inputs to this
		// transaction must not be null.
		for _, txIn := range msgTx.TxIn {
			if isNullOutpoint(&txIn.PreviousOutPoint) {
				err = ruleError(ErrBadTxInput, "transaction input refers to previous output that is null")
				slog.Error(err)
				return
			}
		}
	}
	return
}

// CountP2SHSigOps returns the number of signature operations for all input
// transactions which are of the pay-to-script-hash type.
// This uses the precise,
// signature operation counting mechanism from the script engine which
// requires access to the input transaction scripts.
func CountP2SHSigOps(tx *util.Tx, isCoinBaseTx bool, utxoView *UtxoViewpoint) (totalSigOps int, err error) {
	// Coinbase transactions have no interesting inputs.
	if isCoinBaseTx {
		return
	}
	// Accumulate the number of signature operations in all transaction inputs.
	msgTx := tx.MsgTx()
	for txInIndex, txIn := range msgTx.TxIn {
		// Ensure the referenced input transaction is available.
		utxo := utxoView.LookupEntry(txIn.PreviousOutPoint)
		if utxo == nil || utxo.IsSpent() {
			err = ruleError(ErrMissingTxOut, fmt.Sprintf(
				"output %v referenced from transaction %s:%d either does not exist or has already been spent",
				txIn.PreviousOutPoint, tx.Hash(), txInIndex))
			slog.Error(err)
			return
		}
		// We're only interested in pay-to-script-hash types,
		// so skip this input if it's not one.
		pkScript := utxo.PkScript()
		if !txscript.IsPayToScriptHash(pkScript) {
			continue
		}
		// Count the precise number of signature operations in the referenced
		// public key script.
		sigScript := txIn.SignatureScript
		numSigOps := txscript.GetPreciseSigOpCount(sigScript, pkScript, true)
		// We could potentially overflow the accumulator so check for overflow.
		lastSigOps := totalSigOps
		totalSigOps += numSigOps
		if totalSigOps < lastSigOps {
			err = ruleError(ErrTooManySigOps, fmt.Sprintf(
				"the public key script from output %v contains too many signature operations - overflow",
				txIn.PreviousOutPoint))
			slog.Error(err)
			return
		}
	}
	return
}

// CountSigOps returns the number of signature operations for all
// transaction input and output scripts in the provided transaction.
// This uses the quicker but imprecise signature operation counting mechanism
// from txscript.
func CountSigOps(tx *util.Tx) int {
	msgTx := tx.MsgTx()
	// Accumulate the number of signature operations in all transaction inputs.
	totalSigOps := 0
	for _, txIn := range msgTx.TxIn {
		numSigOps := txscript.GetSigOpCount(txIn.SignatureScript)
		totalSigOps += numSigOps
	}
	// Accumulate the number of signature operations in all transaction
	// outputs.
	for _, txOut := range msgTx.TxOut {
		numSigOps := txscript.GetSigOpCount(txOut.PkScript)
		totalSigOps += numSigOps
	}
	return totalSigOps
}

// ExtractCoinbaseHeight attempts to extract the height of the block
// from the scriptSig of a coinbase transaction.
// Coinbase heights are only present in blocks of version 2 or later.
// This was added as part of BIP0034.
func ExtractCoinbaseHeight(coinbaseTx *util.Tx) (height int32, err error) {
	sigScript := coinbaseTx.MsgTx().TxIn[0].SignatureScript
	if len(sigScript) < 1 {
		err = ruleError(ErrMissingCoinbaseHeight, fmt.Sprintf(
			"the coinbase signature script for blocks of version %d or greater must start with the "+
				"length of the serialized block height", serializedHeightVersion))
		slog.Error(err)
		return
	}
	// Detect the case when the block height is a small integer encoded with
	// as single byte.
	opcode := int(sigScript[0])
	if opcode == txscript.OP_0 {
		return
	}
	if opcode >= txscript.OP_1 && opcode <= txscript.OP_16 {
		return int32(opcode - (txscript.OP_1 - 1)), nil
	}
	// Otherwise, the opcode is the length of the following bytes which
	// encode in the block height.
	serializedLen := int(sigScript[0])
	if len(sigScript[1:]) < serializedLen {
		err = ruleError(ErrMissingCoinbaseHeight, fmt.Sprintf(
			"the coinbase signature script for blocks of version %d or greater must start with the "+
				"serialized block height", serializedLen))
		slog.Error(err)
		return
	}
	serializedHeightBytes := make([]byte, 8)
	copy(serializedHeightBytes, sigScript[1:serializedLen+1])
	serializedHeight := binary.LittleEndian.Uint64(serializedHeightBytes)
	return int32(serializedHeight), nil
}

// IsCoinBase determines whether or not a transaction is a coinbase.
// A coinbase is a special transaction created by miners that has no inputs.
// This is represented in the block chain by a transaction with a single
// input that has a previous output transaction index set to the maximum
// value along with a zero hash.
// This function only differs from IsCoinBaseTx in that it works with a
// higher level util transaction as opposed to a raw wire transaction.
func IsCoinBase(tx *util.Tx) bool {
	return IsCoinBaseTx(tx.MsgTx())
}

// IsCoinBaseTx determines whether or not a transaction is a coinbase.
// A coinbase is a special transaction created by miners that has no inputs.
// This is represented in the block chain by a transaction with a single
// input that has a previous output transaction index set to the maximum
// value along with a zero hash.
// This function only differs from IsCoinBase in that it works with a raw
// wire transaction as opposed to a higher level util transaction.
func IsCoinBaseTx(msgTx *wire.MsgTx) bool {
	// A coin base must only have one transaction input.
	if len(msgTx.TxIn) != 1 {
		return false
	}
	// The previous output of a coin base must have a max value index and a
	// zero hash.
	prevOut := &msgTx.TxIn[0].PreviousOutPoint
	if prevOut.Index != math.MaxUint32 || prevOut.Hash != zeroHash {
		return false
	}
	return true
}

// IsFinalizedTransaction determines whether or not a transaction is
// finalized.
func IsFinalizedTransaction(tx *util.Tx, blockHeight int32, blockTime time.Time) bool {
	msgTx := tx.MsgTx()
	// Lock time of zero means the transaction is finalized.
	lockTime := msgTx.LockTime
	if lockTime == 0 {
		return true
	}
	// The lock time field of a transaction is either a block height at which
	// the transaction is finalized or a timestamp depending on if the value
	// is before the txscript.LockTimeThreshold.
	// When it is under the threshold it is a block height.
	blockTimeOrHeight := int64(0)
	if lockTime < txscript.LockTimeThreshold {
		blockTimeOrHeight = int64(blockHeight)
	} else {
		blockTimeOrHeight = blockTime.Unix()
	}
	if int64(lockTime) < blockTimeOrHeight {
		return true
	}
	// At this point, the transaction's lock time hasn't occurred yet,
	// but the transaction might still be finalized if the sequence number
	// for all transaction inputs is maxed out.
	for _, txIn := range msgTx.TxIn {
		if txIn.Sequence != math.MaxUint32 {
			return false
		}
	}
	return true
}

// SequenceLockActive determines if a transaction's sequence locks have
// been met, meaning that all the inputs of a given transaction have reached a
// height or time sufficient for their relative lock-time maturity.
func SequenceLockActive(sequenceLock *SequenceLock, blockHeight int32, medianTimePast time.Time) bool {
	// If either the seconds,
	// or height relative-lock time has not yet reached,
	// then the transaction is not yet mature according to its sequence locks.
	if sequenceLock.Seconds >= medianTimePast.Unix() ||
		sequenceLock.BlockHeight >= blockHeight {
		return false
	}
	return true
}

// ShouldHaveSerializedBlockHeight determines if a block should have a
// serialized block height embedded within the scriptSig of its coinbase
// transaction. Judgement is based on the block version in the block header.
// BlockC with version 2 and above satisfy this criteria.
// See BIP0034 for further information.
func ShouldHaveSerializedBlockHeight(header *wire.BlockHeader) bool {
	return header.Version >= serializedHeightVersion
}

// checkBlockHeaderSanity performs some preliminary checks on a block
// header to ensure it is sane before continuing with processing.
// These checks are context free.
// The flags do not modify the behavior of this function directly,
// however they are needed to pass along to checkProofOfWork.
func checkBlockHeaderSanity(
	header *wire.BlockHeader, powLimit *big.Int, timeSource MedianTimeSource, flags BehaviorFlags, height int32,
) (err error) {
	// Tracef("checkBlockHeaderSanity %064x %+v", powLimit, header)
	// Ensure the proof of work bits in the block header is in min/max range and
	// the block hash is less than the target value described by the bits.
	if err = checkProofOfWork(header, powLimit, flags, height); slog.Check(err) {
		slog.Errorf("%+v %v", header, err)
		return err
	}
	// A block timestamp must not have a greater precision than one second. This
	// check is necessary because Go time.Time values support nanosecond
	// precision whereas the consensus rules only apply to seconds and it's much
	// nicer to deal with standard Go time values instead of converting to
	// seconds everywhere.
	if !header.Timestamp.Equal(time.Unix(header.Timestamp.Unix(), 0)) {
		err = ruleError(ErrInvalidTime, fmt.Sprintf(
			"block timestamp of %v has a higher precision than one second", header.Timestamp))
		slog.Error(err)
		return
	}
	// Ensure the block time is not too far in the future.
	maxTimestamp := timeSource.AdjustedTime().Add(time.Second *
		MaxTimeOffsetSeconds)
	if header.Timestamp.After(maxTimestamp) {
		err = ruleError(ErrTimeTooNew, fmt.Sprintf(
			"block timestamp of %v is too far in the future", header.Timestamp))
		slog.Error(err)
		return
	}
	return
}

// checkBlockSanity performs some preliminary checks on a block to
// ensure it is sane before continuing with block processing.
// These checks are context free.
// The flags do not modify the behavior of this function directly,
// however they are needed to pass along to checkBlockHeaderSanity.
func checkBlockSanity(block *util.Block, powLimit *big.Int, timeSource MedianTimeSource, flags BehaviorFlags,
	DoNotCheckPow bool, height int32) (err error) {
	slog.Tracef("checkBlockSanity %08x %064x", block.MsgBlock().Header.Bits, powLimit)
	msgBlock := block.MsgBlock()
	header := &msgBlock.Header
	if err = checkBlockHeaderSanity(header, powLimit, timeSource, flags, height); slog.Check(err) {
		slog.Debug("block processing error: ", block.MsgBlock().Header.Version, err)
		return
	}
	// A block must have at least one transaction.
	numTx := len(msgBlock.Transactions)
	if numTx == 0 {
		err = ruleError(ErrNoTransactions, "block does not contain any transactions")
		slog.Error(err)
		return
	}
	// A block must not have more transactions than the max block payload or
	// else it is certainly over the weight limit.
	if numTx > MaxBlockBaseSize {
		err = ruleError(ErrBlockTooBig, fmt.Sprintf(
			"block contains too many transactions - got %d, max %d", numTx, MaxBlockBaseSize))
		slog.Error(err)
		return
	}
	// A block must not exceed the maximum allowed block payload when
	// serialized.
	serializedSize := msgBlock.SerializeSizeStripped()
	if serializedSize > MaxBlockBaseSize {
		err = ruleError(ErrBlockTooBig, fmt.Sprintf(
			"serialized block is too big - got %d, max %d", serializedSize, MaxBlockBaseSize))
		slog.Error(err)
		return
	}
	// The first transaction in a block must be a coinbase.
	transactions := block.Transactions()
	if !IsCoinBase(transactions[0]) {
		err = ruleError(ErrFirstTxNotCoinbase, "first transaction in block is not a coinbase")
		slog.Error(err)
		return
	}
	// A block must not have more than one coinbase.
	for i, tx := range transactions[1:] {
		if IsCoinBase(tx) {
			err = ruleError(ErrMultipleCoinbases, fmt.Sprintf("block contains second coinbase at index %d", i+1))
			slog.Error(err)
			return
		}
	}
	// Do some preliminary checks on each transaction to ensure they are sane
	// before continuing.
	for _, tx := range transactions {
		if err = CheckTransactionSanity(tx); slog.Check(err) {
			return
		}
	}
	// Build merkle tree and ensure the calculated merkle root matches the entry
	// in the block header.  This also has the effect of caching all of the
	// transaction hashes in the block to speed up future hash checks.  Bitcoind
	// builds the tree here and checks the merkle root after the following
	// checks, but there is no reason not to check the merkle root matches here.
	merkles := BuildMerkleTreeStore(block.Transactions(), false)
	calculatedMerkleRoot := merkles[len(merkles)-1]
	if !header.MerkleRoot.IsEqual(calculatedMerkleRoot) {
		err = ruleError(ErrBadMerkleRoot, fmt.Sprintf(
			"block merkle root is invalid - block header indicates %v, but calculated value is %v",
			header.MerkleRoot, calculatedMerkleRoot))
		slog.Error(err)
		return
	}
	// Check for duplicate transactions.  This check will be fairly quick since
	// the transaction hashes are already cached due to building the merkle tree
	// above.
	existingTxHashes := make(map[chainhash.Hash]struct{})
	for _, tx := range transactions {
		hash := tx.Hash()
		if _, exists := existingTxHashes[*hash]; exists {
			err = ruleError(ErrDuplicateTx, fmt.Sprintf(
				"block contains duplicate transaction %v", hash))
			slog.Error(err)
			return
		}
		existingTxHashes[*hash] = struct{}{}
	}
	// The number of signature operations must be less than the maximum allowed
	// per block.
	totalSigOps := 0
	for _, tx := range transactions {
		// We could potentially overflow the accumulator so check for overflow.
		lastSigOps := totalSigOps
		totalSigOps += CountSigOps(tx) * WitnessScaleFactor
		if totalSigOps < lastSigOps || totalSigOps > MaxBlockSigOpsCost {
			err = ruleError(ErrTooManySigOps, fmt.Sprintf(
				"block contains too many signature operations - got %v, max %v",
				totalSigOps, MaxBlockSigOpsCost))
			slog.Error(err)
			return
		}
	}
	return
}

// checkProofOfWork ensures the block header bits which indicate the target
// difficulty is in min/max range and that the block hash is less than the
// target difficulty as claimed.
// The flags modify the behavior of this function as follows:
//  - BFNoPoWCheck: The check to ensure the block hash is less than the
//  target difficulty is not performed.
func checkProofOfWork(header *wire.BlockHeader, powLimit *big.Int, flags BehaviorFlags,
	height int32) (err error) {
	// Tracef("hash %d %s", height, header.BlockHashWithAlgos(height))
	// The target difficulty must be larger than zero.
	if powLimit == nil {
		err = errors.New("PoW limit was not set")
		slog.Error(err)
		return
	}
	target := fork.CompactToBig(header.Bits)
	// Tracef("target %064x %08x", target, header.Bits)
	// Tracef("header: %+v", header)
	if target.Sign() <= 0 {
		err = ruleError(ErrUnexpectedDifficulty, fmt.Sprintf("block target difficulty of %064x is too low", target))
		slog.Error(err)
		return
	}
	// Tracef("checkProofOfWork powLimit %064x %064x", powLimit, target)
	// The target difficulty must be less than the maximum allowed.
	if target.Cmp(powLimit) > 0 {
		err = ruleError(ErrUnexpectedDifficulty, fmt.Sprintf(
			"height %d block target difficulty of %064x is higher than max of %064x", height, target, powLimit))
		slog.Error(err)
		return
	}
	// The block hash must be less than the claimed target unless the flag to
	// avoid proof of work checks is set.
	if flags&BFNoPoWCheck == 0 {
		// The block hash must be less than the claimed target.
		// Unless there is less than 10 previous with the same version (algo)...
		// Debug("height", height, fork.IsTestnet)
		hash := header.BlockHashWithAlgos(height)
		// Debug("blockhashwithalgos", hash, fork.IsTestnet)
		bigHash := HashToBig(&hash)
		if bigHash.Cmp(target) > 0 {
			err = ruleError(ErrHighHash, fmt.Sprintf("block hash of %d"+
				"%064x is higher than expected max of "+
				"%064x", height, bigHash, target))
			slog.Error(err)
			return
		}
	}
	return
}

// checkSerializedHeight checks if the signature script in the passed
// transaction starts with the serialized block height of wantHeight.
func checkSerializedHeight(coinbaseTx *util.Tx, wantHeight int32) (err error) {
	var serializedHeight int32
	if serializedHeight, err = ExtractCoinbaseHeight(coinbaseTx); slog.Check(err) {
		return
	}
	if serializedHeight != wantHeight {
		err = ruleError(ErrBadCoinbaseHeight, fmt.Sprintf(
			"the coinbase signature script serialized block height is %d when %d was expected",
			serializedHeight, wantHeight))
		slog.Error(err)
		return
	}
	return
}

// isBIP0030Node returns whether or not the passed node represents one
// of the two blocks that violate the BIP0030 rule which prevents
// transactions from overwriting old ones.
func isBIP0030Node(node *BlockNode) bool {
	if node.height == 91842 && node.hash.IsEqual(block91842Hash) {
		return true
	}
	if node.height == 91880 && node.hash.IsEqual(block91880Hash) {
		return true
	}
	return false
}

// isNullOutpoint determines whether or not a previous transaction output
// point is set.
func isNullOutpoint(outpoint *wire.OutPoint) bool {
	if outpoint.Index == math.MaxUint32 && outpoint.Hash == zeroHash {
		return true
	}
	return false
}
