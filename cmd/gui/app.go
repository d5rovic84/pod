package gui

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"golang.org/x/exp/shiny/materialdesign/icons"

	l "gioui.org/layout"
	"gioui.org/text"

	"github.com/p9c/pod/app/save"
	"github.com/p9c/pod/pkg/gui/cfg"
	p9icons "github.com/p9c/pod/pkg/gui/ico/svg"
	"github.com/p9c/pod/pkg/gui/p9"
)

func (wg *WalletGUI) GetAppWidget() (a *p9.App) {
	a = wg.th.App(wg.w["main"].Width)
	wg.App = a
	wg.App.ThemeHook(func() {
		Debug("theme hook")
		// Debug(wg.bools)
		*wg.cx.Config.DarkTheme = *wg.th.Dark
		a := wg.configs["config"]["DarkTheme"].Slot.(*bool)
		*a = *wg.th.Dark
		if wgb, ok := wg.config.Bools["DarkTheme"]; ok {
			wgb.Value(*wg.th.Dark)
		}
		save.Pod(wg.cx.Config)
	})
	wg.size = a.Size
	wg.config = cfg.New(wg.cx, wg.th)
	wg.configs = wg.config.Config()
	a.Pages(map[string]l.Widget{
		"main": wg.Page("overview", p9.Widgets{
			// p9.WidgetSize{Widget: p9.EmptyMaxHeight()},
			p9.WidgetSize{Widget: wg.OverviewPage()},
		}),
		"send": wg.Page("send", p9.Widgets{
			// p9.WidgetSize{Widget: p9.EmptyMaxHeight()},
			p9.WidgetSize{Widget: wg.SendPage()},
		}),
		"receive": wg.Page("receive", p9.Widgets{
			// p9.WidgetSize{Widget: p9.EmptyMaxHeight()},
			p9.WidgetSize{Widget: wg.ReceivePage()},
		}),
		"history": wg.Page("history", p9.Widgets{
			// p9.WidgetSize{Widget: p9.EmptyMaxHeight()},
			p9.WidgetSize{Widget: wg.HistoryPage()},
		}),
		"settings": wg.Page("settings", p9.Widgets{
			// p9.WidgetSize{Widget: p9.EmptyMaxHeight()},
			p9.WidgetSize{Widget: func(gtx l.Context) l.Dimensions {
				return wg.configs.Widget(wg.config)(gtx)
			}},
		}),
		"console": wg.Page("console", p9.Widgets{
			// p9.WidgetSize{Widget: p9.EmptyMaxHeight()},
			p9.WidgetSize{Widget: wg.console.Fn},
		}),
		"help": wg.Page("help", p9.Widgets{
			p9.WidgetSize{Widget: p9.EmptyMaxHeight()},
		}),
		"log": wg.Page("log", p9.Widgets{
			p9.WidgetSize{Widget: p9.EmptyMaxHeight()},
		}),
		"quit": wg.Page("quit", p9.Widgets{
			p9.WidgetSize{Widget: func(gtx l.Context) l.Dimensions {
				return wg.th.VFlex().
					SpaceEvenly().
					AlignMiddle().
					Rigid(
						wg.th.H4("are you sure?").Color(wg.App.BodyColorGet()).Alignment(text.Middle).Fn,
					).
					Rigid(
						wg.th.Flex().
							// SpaceEvenly().
							Flexed(0.5, p9.EmptyMaxWidth()).
							Rigid(
								wg.th.Button(wg.clickables["quit"].SetClick(func() {
									close(wg.quit)
								})).Color("Light").TextScale(2).Text("yes!!!").Fn,
							).
							Flexed(0.5, p9.EmptyMaxWidth()).
							Fn,
					).
					Fn(gtx)
			},
			},
		}),
		"goroutines": wg.Page("log", p9.Widgets{
			// p9.WidgetSize{Widget: p9.EmptyMaxHeight()},

			p9.WidgetSize{Widget: func(gtx l.Context) l.Dimensions {
				le := func(gtx l.Context, index int) l.Dimensions {
					return wg.State.goroutines[index](gtx)
				}
				return func(gtx l.Context) l.Dimensions {
					return wg.th.Inset(0.25,
						wg.th.Fill("DocBg",
							wg.lists["recent"].
								Vertical().
								// Background("DocBg").Color("DocText").Active("Primary").
								Length(len(wg.State.goroutines)).
								ListElement(le).
								Fn,
						).Fn,
					).
						Fn(gtx)
				}(gtx)
				// wg.ShellRunCommandChan <- "stop"
				// consume.Kill(wg.Worker)
				// consume.Kill(wg.cx.StateCfg.Miner)
				// close(wg.cx.NodeKill)
				// close(wg.cx.KillAll)
				// time.Sleep(time.Second*3)
				// interrupt.Request()
				// os.Exit(0)
				// return l.Dimensions{}
			}},
		}),
		"mining": wg.Page("mining", p9.Widgets{
			p9.WidgetSize{Widget: func(gtx l.Context) l.Dimensions {
				return wg.th.VFlex().
					AlignMiddle().
					SpaceSides().
					Rigid(
						wg.th.Flex().
							Flexed(0.5, p9.EmptyMaxWidth()).
							Rigid(
								wg.th.H1("Mining").Fn,
							).
							Flexed(0.5, p9.EmptyMaxWidth()).
							Fn,
					).
					Fn(gtx)
			},
			},
		}),
		"explorer": wg.Page("explorer", p9.Widgets{
			p9.WidgetSize{Widget: func(gtx l.Context) l.Dimensions {
				return wg.th.VFlex().
					AlignMiddle().
					SpaceSides().
					Rigid(
						wg.th.Flex().
							Flexed(0.5, p9.EmptyMaxWidth()).
							Rigid(
								wg.th.H1("explorer").Fn,
							).
							Flexed(0.5, p9.EmptyMaxWidth()).
							Fn,
					).
					Fn(gtx)
			},
			},
		}),
	})
	a.SideBar([]l.Widget{
		wg.SideBarButton("overview", "main", 0),
		wg.SideBarButton("send", "send", 1),
		wg.SideBarButton("receive", "receive", 2),
		wg.SideBarButton("history", "history", 3),
		wg.SideBarButton("explorer", "explorer", 6),
		wg.SideBarButton("mining", "mining", 7),
		wg.SideBarButton("console", "console", 9),
		wg.SideBarButton("settings", "settings", 5),
		wg.SideBarButton("log", "log", 10),
		wg.SideBarButton("help", "help", 8),
		wg.SideBarButton("quit", "quit", 11),
	})
	a.ButtonBar([]l.Widget{
		wg.PageTopBarButton("lock", 4, &icons.ActionLockOpen, func(name string) {
			// wg.unlockPage.ActivePage(name)
			wg.unlockPassword.Wipe()
			*wg.walletLocked = true
		}, a, ""),
		wg.PageTopBarButton("console", 2, &p9icons.Terminal, func(name string) {
			wg.App.ActivePage(name)
		}, a, ""),
		wg.PageTopBarButton("goroutines", 0, &icons.ActionBugReport, func(name string) {
			wg.App.ActivePage(name)
		}, a, ""),
		wg.PageTopBarButton("help", 1, &icons.ActionHelp, func(name string) {
			wg.App.ActivePage(name)
		}, a, ""),
		wg.PageTopBarButton("quit", 3, &icons.ActionExitToApp, func(name string) {
			wg.App.ActivePage(name)
		}, a, ""),
	})
	a.StatusBar([]l.Widget{
		// func(gtx l.Context) l.Dimensions { return wg.RunStatusPanel(gtx) },
		wg.RunStatusPanel,
		wg.th.Flex().Rigid(
			wg.StatusBarButton("log", 4, &icons.ActionList, func(name string) {
				Debug("click on button", name)
				if wg.App.MenuOpen {
					wg.App.MenuOpen = false
				}
				wg.App.ActivePage(name)
			}, a),
		).Rigid(
			wg.StatusBarButton("settings", 5, &icons.ActionSettings, func(name string) {
				Debug("click on button", name)
				if wg.App.MenuOpen {
					wg.App.MenuOpen = false
				}
				wg.App.ActivePage(name)
			}, a),
		).Fn,
	})
	a.AddOverlay(wg.toasts.DrawToasts())
	a.AddOverlay(wg.dialog.DrawDialog())
	return
}

func (wg *WalletGUI) Page(title string, widget p9.Widgets) func(gtx l.Context) l.Dimensions {
	a := wg.th
	return func(gtx l.Context) l.Dimensions {
		return a.Fill(wg.App.BodyBackgroundGet(),
			a.VFlex().
				SpaceEvenly().
				Rigid(
					a.Responsive(*wg.Size, p9.Widgets{
						p9.WidgetSize{
							Widget: a.Inset(0.25, a.H5(title).Color(wg.App.BodyColorGet()).Fn).Fn,
						},
						p9.WidgetSize{
							Size:   800,
							Widget: p9.EmptySpace(0, 0),
							// a.Inset(0.25, a.Caption(title).Color(wg.BodyColorGet()).Fn).Fn,
						},
					}).Fn,
				).
				Flexed(1,
					a.Inset(0.25,
						a.Responsive(*wg.Size, widget).Fn,
					).Fn,
				).Fn,
		).Fn(gtx)
	}
}

func (wg *WalletGUI) SideBarButton(title, page string, index int) func(gtx l.Context) l.Dimensions {
	return func(gtx l.Context) l.Dimensions {
		// gtx.Constraints.Max.X = int(wg.App.SideBarSize.V)
		// gtx.Constraints.Min.X = int(wg.App.SideBarSize.V)
		background := "DocBg"
		color := "DocText"
		if wg.App.ActivePageGet() == page {
			background = "PanelBg"
			color = "PanelText"
		}
		return wg.th.Fill(background, wg.th.ButtonLayout(wg.sidebarButtons[index]).Embed(
			func(gtx l.Context) l.Dimensions {
				gtx.Constraints.Min.X =
					gtx.Constraints.Max.X
				var pad float32 = 0.5
				return wg.th.Flex().Rigid(
					wg.th.Inset(pad,
						wg.th.H6(title).
							Color(color).
							TextScale(p9.Scales["Body1"]).
							Fn,
					).Fn,
				).Fn(gtx)
			},
		).
			Background("Transparent").
			SetClick(
				func() {
					if wg.App.MenuOpen {
						wg.App.MenuOpen = false
					}
					wg.App.ActivePage(page)
				}).
			Fn,
		).Fn(gtx)
	}
}

func (wg *WalletGUI) PageTopBarButton(name string, index int, ico *[]byte, onClick func(string), app *p9.App,
	highlightColor string) func(gtx l.Context) l.Dimensions {
	return func(gtx l.Context) l.Dimensions {
		background := app.TitleBarBackgroundGet()
		color := app.MenuColorGet()

		if app.ActivePageGet() == name {
			color = "PanelText"
			background = "PanelBg"
		}
		if highlightColor != "" {
			color = highlightColor
		}
		ic := wg.th.Icon().
			Scale(p9.Scales["H5"]).
			Color(color).
			Src(ico).
			Fn
		return wg.th.Flex().Rigid(
			// wg.Inset(0.25,
			wg.th.ButtonLayout(wg.buttonBarButtons[index]).
				CornerRadius(0).
				Embed(
					wg.th.Inset(0.375,
						ic,
					).Fn,
				).
				Background(background).
				SetClick(func() { onClick(name) }).
				Fn,
			// ).Fn,
		).Fn(gtx)
	}
}

func (wg *WalletGUI) StatusBarButton(name string, index int, ico *[]byte, onClick func(string), app *p9.App) func(gtx l.Context) l.Dimensions {
	return func(gtx l.Context) l.Dimensions {
		background := app.StatusBarBackgroundGet()
		color := app.StatusBarColorGet()
		if app.ActivePageGet() == name {
			background = "PanelBg"
			// color = "Danger"
		}
		ic := wg.th.Icon().
			Scale(p9.Scales["H5"]).
			Color(color).
			Src(ico).
			Fn
		return wg.th.Flex().
			Rigid(
				wg.th.ButtonLayout(wg.statusBarButtons[index]).
					CornerRadius(0).
					Embed(
						wg.th.Inset(0.25, ic).Fn,
					).
					Background(background).
					SetClick(func() { onClick(name) }).
					Fn,
			).Fn(gtx)
	}
}

func (wg *WalletGUI) SetRunState(b bool) {
	go func() {
		Debug("run state is now", b)
		if b {
			wg.ShellRunCommandChan <- "run"
			// wg.running = b
		} else {
			wg.ShellRunCommandChan <- "stop"
			// wg.running = b
		}
	}()
}

func (wg *WalletGUI) RunStatusPanel(gtx l.Context) l.Dimensions {
	return func(gtx l.Context) l.Dimensions {
		t, f := &p9icons.Link, &p9icons.LinkOff
		var runningIcon *[]byte
		if wg.running {
			runningIcon = t
		} else {
			runningIcon = f
		}
		miningIcon := &p9icons.Mine
		if !wg.mining {
			miningIcon = &p9icons.NoMine
		}
		return wg.th.Flex().AlignMiddle().
			Rigid(
				wg.th.ButtonLayout(wg.statusBarButtons[0]).
					CornerRadius(0).
					Embed(
						wg.th.Inset(0.25,
							wg.th.Icon().
								Scale(p9.Scales["H5"]).
								Color("DocText").
								Src(runningIcon).
								Fn,
						).Fn,
					).
					Background("Transparent").
					SetClick(
						func() {
							go wg.SetRunState(!wg.running)
						}).
					Fn,
			).
			// Rigid(
			// 	wg.th.Inset(0.25,
			// 		p9.If(wg.running,
			// 			wg.th.Indefinite().Scale(p9.Scales["H5"]).Fn,
			// 			wg.th.Icon().
			// 				Scale(p9.Scales["H5"]).
			// 				Color("Primary").
			// 				Src(&icons.ActionCheckCircle).
			// 				Fn,
			// 		),
			// 	).Fn,
			// ).
			// Rigid(wg.th.
			// 	Inset(0.25,
			// 		wg.Icon().
			// 			Scale(p9.Scales["H5"]).
			// 			Color("DocText").
			// 			Src(&icons.DeviceWidgets).
			// 			Fn,
			// 	).
			// 	Fn,
			// ).
			Rigid(
				wg.th.Inset(0.33,
					wg.th.Body1(fmt.Sprintf("%d", wg.State.bestBlockHeight)).
						Font("go regular").TextScale(p9.Scales["Caption"]).
						Color("DocText").
						Fn,
				).Fn,
			).
			Rigid(
				wg.th.ButtonLayout(wg.statusBarButtons[1]).
					CornerRadius(0).
					Embed(wg.th.
						Inset(0.25, wg.th.
							Icon().
							Scale(p9.Scales["H5"]).
							Color("DocText").
							Src(miningIcon).Fn,
						).Fn,
					).
					Background("Transparent").
					SetClick(
						func() {
							go func() {
								Debug("clicked miner control stop/start button", wg.mining)
								wg.mining = !wg.mining
								if *wg.cx.Config.GenThreads == 0 {
									Debug("was zero threads")
									wg.mining = false
									// wg.MinerThreadsChan <- 1
									// wg.MinerRunCommandChan <- "run"
									// wg.incdecs["generatethreads"].SetCurrent(1)
									return
								}
								if !wg.mining {
									wg.MinerRunCommandChan <- "stop"
								} else {
									wg.MinerRunCommandChan <- "run"
								}
							}()
						}).
					Fn,
			).
			Rigid(
				wg.incdecs["generatethreads"].
					Color("DocText").
					Background("DocBg").
					Fn,
			).
			Rigid(
				func(gtx l.Context) l.Dimensions {
					// background := wg.App.StatusBarBackgroundGet()
					color := wg.App.StatusBarColorGet()
					ic := wg.th.Icon().
						Scale(p9.Scales["H5"]).
						Color(color).
						Src(&icons.NavigationRefresh).
						Fn
					return wg.th.Flex().
						Rigid(
							wg.th.ButtonLayout(wg.statusBarButtons[2]).
								CornerRadius(0).
								Embed(
									wg.th.Inset(0.25, ic).Fn,
								).
								Background("Transparent").
								SetClick(
									func() {
										Debug("clicked reset wallet button")
										go func() {
											var err error
											wasRunning := wg.running
											wasMining := wg.mining
											Debug("was running", wasRunning)
											if wasRunning {
												wg.ShellRunCommandChan <- "stop"
											}
											if wasMining {
												wg.MinerRunCommandChan <- "stop"
											}
											args := []string{os.Args[0], "-D", *wg.cx.Config.DataDir,
												"--pipelog", "shell", "drophistory"}
											runner := exec.Command(args[0], args[1:]...)
											runner.Stderr = os.Stderr
											runner.Stdout = os.Stderr
											// for security with apps launching the wallet, the public password can be set with a file that is deleted after
											walletPassPath := *wg.cx.Config.DataDir + slash + wg.cx.ActiveNet.Params.Name + slash + "wp.txt"
											Debug("runner", walletPassPath)
											wp := *wg.cx.Config.WalletPass
											b := []byte(wp)
											if err = ioutil.WriteFile(walletPassPath, b, 0700); Check(err) {
											}
											Debug("created password cookie")
											if err := runner.Run(); Check(err) {
											}
											if wasRunning {
												wg.ShellRunCommandChan <- "run"
											}
											time.Sleep(time.Second*3)
											if wasMining {
												wg.MinerRunCommandChan <- "run"
											}
										}()
									}).
								Fn,
						).Fn(gtx)
				},
			).
			Fn(gtx)
	}(gtx)
}
