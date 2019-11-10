//+build ignore

// file generated by cmd/tools/genmsghandle/main.go DO NOT EDIT
/* {kopach controller.Blocks broadcast.TplBlock "github.com/p9c/pod/pkg/controller"} */
package kopach

import (
	"crypto/cipher"
	"github.com/p9c/pod/pkg/controller/broadcast"
	"github.com/p9c/pod/pkg/controller/gcm"
	"github.com/p9c/pod/pkg/log"
	"github.com/ugorji/go/codec"
	"net"
	"time"
)

type msgBuffer struct {
	buffers [][]byte
	first   time.Time
	decoded bool
}

type msgHandle struct {
	buffers   map[string]*msgBuffer
	ciph      *cipher.AEAD
	dec       *codec.Decoder
	decBuf    []byte
	returnChan chan *controllerold.Blocks
}

func newMsgHandle(password string, returnChan chan *controllerold.Blocks) (out *msgHandle) {
	out = &msgHandle{}
	out.buffers = make(map[string]*msgBuffer)
	ciph := gcm.GetCipher(password)
	out.ciph = &ciph
	var mh codec.MsgpackHandle
	out.decBuf = make([]byte, 0, broadcast.MaxDatagramSize)
	out.dec = codec.NewDecoderBytes(out.decBuf, &mh)
    out.returnChan = returnChan
	return
}

func (m *msgHandle) msgHandler(src *net.UDPAddr, n int, b []byte) {
	// remove any expired message bundles in the cache
	var deleters []string
	for i := range m.buffers {
		if time.Now().Sub(m.buffers[i].first) > time.Millisecond*50 {
			deleters = append(deleters, i)
		}
	}
	for i := range deleters {
		//log.TRACE("deleting old message buffer")
		delete(m.buffers, deleters[i])
	}
	b = b[:n]
	if n < 16 {
		log.ERROR("received short broadcast message")
		return
	}
	// snip off message magic bytes
	msgType := string(b[:8])
	b = b[8:]
	if msgType == broadcast.TplBlock {
		//log.TRACE(n, " bytes read from ", src, "message type", msgType)
		buffer := b
		nonce := string(b[:8])
		if x, ok := m.buffers[nonce]; ok {
			//log.TRACE("additional shard with nonce", hex.EncodeToString([]byte(nonce)))
			if !x.decoded {
				//log.TRACE("appending shard")
				x.buffers = append(x.buffers, buffer)
				lb := len(x.buffers)
				//log.TRACE("have", lb, "buffers")
				if lb > 2 {
					// try to decode it
					bytes, err := broadcast.Decode(*m.ciph, x.buffers)
					if err != nil {
						log.ERROR(err)
						return
					}
					m.dec.ResetBytes(bytes)
					message := &controllerold.Blocks{}
					err = m.dec.Decode(&message)
					if err != nil {
						log.ERROR(err)
					}
					x.decoded = true
					// send it back
					m.returnChan <- message
				}
			} else if x.buffers != nil {
				//log.TRACE("nilling buffers")
				x.buffers = nil
			} else {
				//log.TRACE("ignoring already decoded message shard")
			}
		} else {
			//log.TRACE("adding nonce", hex.EncodeToString([]byte(nonce)))
			m.buffers[nonce] = &msgBuffer{[][]byte{}, time.Now(), false}
			m.buffers[nonce].buffers = append(m.buffers[nonce].buffers, b)
		}
	}
}
