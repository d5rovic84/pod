package rcd

import (
	"github.com/p9c/pod/cmd/node/rpc"
	"github.com/p9c/pod/pkg/log"
	"github.com/p9c/pod/pkg/pod"
	"github.com/p9c/pod/pkg/wallet"
	"time"

	"github.com/p9c/pod/cmd/gui/mvc/controller"
	"github.com/p9c/pod/cmd/gui/mvc/model"
	"github.com/p9c/pod/pkg/conte"
	"github.com/p9c/pod/pkg/rpc/btcjson"
)

type RcVar struct {
	cx              *conte.Xt
	db              *DuoUIdb
	Boot            *Boot
	Quit            chan struct{}
	Ready           chan struct{}
	Events          chan Event
	UpdateTrigger   chan struct{}
	Status          *model.DuoUIstatus
	Dialog          *model.DuoUIdialog
	Log             *model.DuoUIlog
	CommandsHistory *model.DuoUIcommandsHistory

	Settings          *model.DuoUIsettings
	Sent              bool
	Toasts            []model.DuoUItoast
	Localhost         model.DuoUIlocalHost
	Uptime            int
	Peers             []*btcjson.GetPeerInfoResult `json:"peers"`
	Blocks            []model.DuoUIblock
	AddressBook       model.DuoUIaddressBook
	ShowPage          string
	PassPhrase        string
	ConfirmPassPhrase string
	NodeChan          chan *rpc.Server
	WalletChan        chan *wallet.Wallet
}

type Boot struct {
	IsBoot     bool   `json:"boot"`
	IsFirstRun bool   `json:"firstrun"`
	IsBootMenu bool   `json:"menu"`
	IsBootLogo bool   `json:"logo"`
	IsLoading  bool   `json:"loading"`
	IsScreen   string `json:"screen"`
}

// type rcVar interface {
//	GetDuoUItransactions(sfrom, count int, cat string)
//	GetDuoUIbalance()
//	GetDuoUItransactionsExcerpts()
//	DuoSend(wp string, ad string, am float64)
//	GetDuoUItatus()
//	PushDuoUIalert(t string, m interface{}, at string)
//	GetDuoUIblockHeight()
//	GetDuoUIblockCount()
//	GetDuoUIconnectionCount()
// }

func RcInit(cx *conte.Xt) (r *RcVar) {
	b := Boot{
		IsBoot:     true,
		IsFirstRun: false,
		IsBootMenu: false,
		IsBootLogo: false,
		IsLoading:  false,
		IsScreen:   "",
	}

	// d := models.DuoUIdialog{
	//	Show:   true,
	//	Ok:     func() { r.Dialog.Show = false },
	//	Cancel: func() { r.Dialog.Show = false },
	//	Title:  "Dialog!",
	//	Text:   "Dialog text",
	// }
	l := new(model.DuoUIlog)
	settings := &model.DuoUIsettings{
		Abbrevation: "DUO",
		Tabs: &model.DuoUIconfTabs{
			Current:  "wallet",
			TabsList: make(map[string]*controller.Button),
		},
		Daemon: &model.DaemonConfig{
			Config: cx.Config,
			Schema: pod.GetConfigSchema(),
		},
	}

	// Settings tabs

	settingsFields := make(map[string]interface{})
	for _, group := range settings.Daemon.Schema.Groups {
		settings.Tabs.TabsList[group.Legend] = new(controller.Button)
		for _, field := range group.Fields {
			switch field.Type {
			case "array":
				settingsFields[field.Name] = new(controller.Button)
			case "input":
				settingsFields[field.Name] = &controller.Editor{
					SingleLine: true,
					Submit:     true,
				}
			case "switch":
				settingsFields[field.Name] = new(controller.CheckBox)
			case "radio":
				settingsFields[field.Name] = new(controller.Enum)
			default:
				settingsFields[field.Name] = new(controller.Button)
			}
		}
	}

	settings.Daemon.Widgets = settingsFields

	r = &RcVar{
		cx:    cx,
		Quit:  make(chan struct{}),
		Ready: make(chan struct{}, 1),
		Boot:  &b,
		Status: &model.DuoUIstatus{
			Node: &model.NodeStatus{},
			Wallet: &model.WalletStatus{
				WalletVersion: make(map[string]btcjson.VersionResult),
				Transactions:  &model.DuoUItransactions{},
				Txs:           &model.DuoUItransactionsExcerpts{},
				LastTxs:       &model.DuoUItransactions{},
			},
		},
		Dialog:   &model.DuoUIdialog{},
		Settings: settings,
		Log:      l,
		CommandsHistory: &model.DuoUIcommandsHistory{
			Commands: []model.DuoUIcommand{
				model.DuoUIcommand{
					ComID:    "input",
					Category: "input",
					Time:     time.Now(),

					// Out: input(duo),
				},
			},
			CommandsNumber: 1,
		},
		Sent:       false,
		Localhost:  model.DuoUIlocalHost{},
		ShowPage:   "OVERVIEW",
		NodeChan:   make(chan *rpc.Server),
		WalletChan: make(chan *wallet.Wallet),
	}
	return
}

func (r *RcVar) DuoUIservices() (err error) {
	// Start Node
	err = r.DuoUInode()
	if err != nil {
		log.ERROR(err)
	}
	log.DEBUG("waiting for nodeChan")
	r.cx.RPCServer = <-r.NodeChan
	log.DEBUG("nodeChan sent")
	r.cx.Node.Store(true)

	// Start wallet
	err = r.DuoUIwallet()
	if err != nil {
		log.ERROR(err)
	}
	log.DEBUG("waiting for walletChan")
	r.cx.WalletServer = <-r.WalletChan
	log.DEBUG("walletChan sent")
	r.cx.Wallet.Store(true)
	r.Boot.IsBoot = false
	r.Ready <- struct{}{}
	return
}
