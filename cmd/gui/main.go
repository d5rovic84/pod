package gui

import (
	"gioui.org/app"
	"github.com/urfave/cli"

	"github.com/p9c/pod/app/conte"
	"github.com/p9c/pod/pkg/gui/cfg"
	"github.com/p9c/pod/pkg/gui/f"
	"github.com/p9c/pod/pkg/gui/fonts/p9fonts"
	"github.com/p9c/pod/pkg/gui/p9"
	"github.com/p9c/pod/pkg/util/interrupt"
)

func Main(cx *conte.Xt, c *cli.Context) (err error) {
	var size int
	wg := &WalletGUI{
		cx:         cx,
		c:          c,
		invalidate: make(chan struct{}),
		quit:       cx.KillAll,
		size:       &size,
	}
	return wg.Run()
}

type WalletGUI struct {
	cx   *conte.Xt
	c    *cli.Context
	w    *f.Window
	th   *p9.Theme
	size *int
	*p9.App
	sidebarButtons   []*p9.Clickable
	buttonBarButtons []*p9.Clickable
	statusBarButtons []*p9.Clickable
	bools            map[string]*p9.Bool
	quitClickable    *p9.Clickable
	lists            map[string]*p9.List
	checkables       map[string]*p9.Checkable
	clickables       map[string]*p9.Clickable
	passwords        map[string]*p9.Password
	configs          cfg.GroupsMap
	config           *cfg.Config
	running          bool
	invalidate       chan struct{}
	quit             chan struct{}

	sendAddresses []*SendAddress
}

func (wg *WalletGUI) Run() (err error) {
	wg.th = p9.NewTheme(p9fonts.Collection(), wg.quit)
	wg.th.Colors.SetTheme(wg.th.Dark)
	wg.sidebarButtons = make([]*p9.Clickable, 9)
	for i := range wg.sidebarButtons {
		wg.sidebarButtons[i] = wg.th.Clickable()
	}
	wg.buttonBarButtons = make([]*p9.Clickable, 4)
	for i := range wg.buttonBarButtons {
		wg.buttonBarButtons[i] = wg.th.Clickable()
	}
	wg.statusBarButtons = make([]*p9.Clickable, 3)
	for i := range wg.statusBarButtons {
		wg.statusBarButtons[i] = wg.th.Clickable()
	}
	wg.lists = map[string]*p9.List{
		"createwallet": wg.th.List(),
		"overview":     wg.th.List(),
		"send":         wg.th.List(),
		"settings":     wg.th.List(),
	}
	wg.clickables = map[string]*p9.Clickable{
		"createwallet": wg.th.Clickable(),
		"quit":         wg.th.Clickable(),
		"send":         wg.th.Clickable(),
		"clearall":     wg.th.Clickable(),
		"addrecipient": wg.th.Clickable(),
	}
	wg.bools = map[string]*p9.Bool{
		"runstate":   wg.th.Bool(wg.running),
		"encryption": wg.th.Bool(false),
		"seed":       wg.th.Bool(false),
		"testnet":    wg.th.Bool(false),
	}

	pass := "password"

	wg.passwords = map[string]*p9.Password{
		"passEditor":        wg.th.Password(&pass, "Primary", "DocText", 25, func(pass string) {}),
		"confirmPassEditor": wg.th.Password(&pass, "Primary", "DocText", 25, func(pass string) {}),
	}

	wg.quitClickable = wg.th.Clickable()
	wg.w = f.NewWindow()
	wg.App = wg.GetAppWidget()

	wg.CreateSendAddressItem()
	go func() {
		if err := wg.w.
			Size(640, 480).
			Title("ParallelCoin Wallet").
			Open().
			Run(
				wg.Fn(),
				//wg.InitWallet(),
				func() {
					Debug("quitting wallet gui")
					interrupt.Request()
				}); Check(err) {
		}
	}()
	// tickers and triggers
	go func() {
	out:
		for {
			select {
			case <-wg.invalidate:
				wg.w.Window.Invalidate()
			case <-wg.quit:
				break out
			}
		}
	}()
	app.Main()
	return
}
