// SPDX-License-Identifier: Unlicense OR MIT

// A simple app used for gogio's end-to-end tests.
package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"runtime"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op/paint"
)

func main() {
	go func() {
		w := app.NewWindow()
		if err := loop(w); err != nil {
			log.Fatal(err)
		}
	}()
	app.Main()
}

type notifyFrame int

const (
	notifyNone notifyFrame = iota
	notifyInvalidate
	notifyPrint
)

// notify keeps track of whether we want to print to stdout to notify the user
// when a frame is ready. Initially we want to notify about the first frame.
var notify = notifyInvalidate

func loop(w *app.Window) error {
	topLeft := quarterWidget{
		color: color.RGBA{R: 0xde, G: 0xad, B: 0xbe, A: 0xff},
	}
	topRight := quarterWidget{
		color: color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
	}
	botLeft := quarterWidget{
		color: color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xff},
	}
	botRight := quarterWidget{
		color: color.RGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x80},
	}

	gtx := &layout.Context{
		Queue: w.Queue(),
	}
	for {
		e := <-w.Events()
		switch e := e.(type) {
		case system.DestroyEvent:
			return e.Err
		case system.FrameEvent:

			gtx.Reset(e.Config, e.Size)
			rows := layout.Flex{Axis: layout.Vertical}
			r1 := rows.Flex(gtx, 0.5, func() {
				columns := layout.Flex{Axis: layout.Horizontal}
				r1c1 := columns.Flex(gtx, 0.5, func() { topLeft.Layout(gtx) })
				r1c2 := columns.Flex(gtx, 0.5, func() { topRight.Layout(gtx) })
				columns.Layout(gtx, r1c1, r1c2)
			})
			r2 := rows.Flex(gtx, 0.5, func() {
				columns := layout.Flex{Axis: layout.Horizontal}
				r2c1 := columns.Flex(gtx, 0.5, func() { botLeft.Layout(gtx) })
				r2c2 := columns.Flex(gtx, 0.5, func() { botRight.Layout(gtx) })
				columns.Layout(gtx, r2c1, r2c2)
			})
			rows.Layout(gtx, r1, r2)

			e.Frame(gtx.Ops)

			if runtime.GOOS == "js" {
				// TODO(mvdan): Unfortunately, printing to
				// stdout crashes the js/wasm port. Use the
				// prints once the issue is fixed:
				// https://github.com/golang/go/issues/35256
				break
			}
			switch notify {
			case notifyInvalidate:
				notify = notifyPrint
				w.Invalidate()
			case notifyPrint:
				notify = notifyNone
				fmt.Println("frame ready")
			}
		}
	}
}

// quarterWidget paints a quarter of the screen with one color. When clicked, it
// turns red, going back to its normal color when clicked again.
type quarterWidget struct {
	color color.RGBA

	clicked bool
}

var red = color.RGBA{R: 0xff, G: 0x00, B: 0x00, A: 0xff}

func (w *quarterWidget) Layout(gtx *layout.Context) {
	if w.clicked {
		paint.ColorOp{Color: red}.Add(gtx.Ops)
	} else {
		paint.ColorOp{Color: w.color}.Add(gtx.Ops)
	}
	paint.PaintOp{Rect: f32.Rectangle{Max: f32.Point{
		X: float32(gtx.Constraints.Width.Max),
		Y: float32(gtx.Constraints.Height.Max),
	}}}.Add(gtx.Ops)

	pointer.RectAreaOp{Rect: image.Rectangle{
		Max: image.Pt(gtx.Constraints.Width.Max, gtx.Constraints.Height.Max),
	}}.Add(gtx.Ops)
	pointer.InputOp{Key: w}.Add(gtx.Ops)

	for _, e := range gtx.Events(w) {
		if e, ok := e.(pointer.Event); ok && e.Type == pointer.Press {
			w.clicked = !w.clicked
			// notify when we're done updating the frame.
			notify = notifyInvalidate
		}
	}
}
