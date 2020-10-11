package p9

import (
	"gioui.org/layout"
)

type BoolHook func(b bool)

type Bool struct {
	th          *Theme
	value       bool
	clk         *Clickable
	changed     bool
	changeState BoolHook
}

// GetValue gets the boolean value stored in the widget
func (b *Bool) GetValue() bool {
	return b.value
}

// Value sets the value of the boolean stored in the widget
func (b *Bool) Value(value bool) {
	b.value = value
}

// Bool creates a new boolean widget
func (th *Theme) Bool(value bool) *Bool {
	return &Bool{
		th:          th,
		value:       value,
		clk:         NewClickable(),
		changed:     false,
		changeState: func(b bool) {},
	}
}

// SetOnChange sets the callback function to run when the state changes
func (b *Bool) SetOnChange(fn BoolHook) *Bool {
	b.changeState = fn
	return b
}

// Changed reports whether value has changed since the last call to Changed
func (b *Bool) Changed() bool {
	changed := b.changed
	b.changed = false
	return changed
}

// History returns the history of presses in the buffer
func (b *Bool) History() []press {
	return b.clk.History()
}

// Fn renders the events of the boolean widget
func (b *Bool) Fn(gtx layout.Context) layout.Dimensions {
	dims := b.clk.Fn(gtx)
	for b.clk.Clicked() {
		b.value = !b.value
		b.changed = true
		b.changeState(b.value)
	}
	return dims
}
