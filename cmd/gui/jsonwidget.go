package gui

import (
	"encoding/json"
	"fmt"
	"sort"

	l "gioui.org/layout"
	"github.com/atotto/clipboard"
	"golang.org/x/exp/shiny/materialdesign/icons"

	"github.com/p9c/pod/pkg/gui/p9"
)

type JSONElement struct {
	key   string
	value interface{}
}

type JSONElements []JSONElement

func (je JSONElements) Len() int {
	return len(je)
}

func (je JSONElements) Less(i, j int) bool {
	return je[i].key < je[j].key
}

func (je JSONElements) Swap(i, j int) {
	je[i], je[j] = je[j], je[i]
}

func GetJSONElements(in map[string]interface{}) (je JSONElements) {
	for i := range in {
		je = append(je, JSONElement{
			key:   i,
			value: in[i],
		})
	}
	sort.Sort(je)
	return
}

func (c *Console) getIndent(n int, size float32, widget l.Widget) (out l.Widget) {
	o := c.th.Flex()
	for i := 0; i < n; i++ {
		o.Rigid(c.th.Inset(size/2, p9.EmptySpace(0, 0)).Fn)
	}
	o.Rigid(widget)
	out = o.Fn
	return
}

func (c *Console) JSONWidget(color string, j []byte) (out []l.Widget) {
	var ifc interface{}
	var err error
	if err = json.Unmarshal(j, &ifc); Check(err) {
	}
	return c.jsonWidget(color, 0, "", ifc)
}

func (c *Console) jsonWidget(color string, depth int, key string, in interface{}) (out []l.Widget) {
	switch in.(type) {
	case []interface{}:
		if key != "" {
			out = append(out, c.getIndent(depth, 1,
				func(gtx l.Context) l.Dimensions {
					return c.th.Body1(key).Font("bariol bold").Color(color).Fn(gtx)
				},
			))
		}
		Debug("got type []interface{}")
		res := in.([]interface{})
		if len(res) == 0 {
			out = append(out, c.getIndent(depth+1, 1,
				func(gtx l.Context) l.Dimensions {
					return c.th.Body1("[]").Color(color).Fn(gtx)
				},
			))
		} else {
			for i := range res {
				// Debugs(res[i])
				out = append(out, c.jsonWidget(color, depth+1, fmt.Sprint(i), res[i])...)
			}
		}
	case map[string]interface{}:
		if key != "" {
			out = append(out, c.getIndent(depth, 1,
				func(gtx l.Context) l.Dimensions {
					return c.th.Body1(key).Font("bariol bold").Color(color).Fn(gtx)
				},
			))
		}
		Debug("got type map[string]interface{}")
		res := in.(map[string]interface{})
		je := GetJSONElements(res)
		// Debugs(je)
		if len(res) == 0 {
			out = append(out, c.getIndent(depth+1, 1,
				func(gtx l.Context) l.Dimensions {
					return c.th.Body1("{}").Color(color).Fn(gtx)
				},
			))
		} else {
			for i := range je {
				Debugs(je[i])
				out = append(out, c.jsonWidget(color, depth+1, je[i].key, je[i].value)...)
			}
		}
	case JSONElement:
		res := in.(JSONElement)
		key = res.key
		switch res.value.(type) {
		case string:
			Debug("got type string")
			res := res.value.(string)
			clk := c.th.WidgetPool.GetClickable()
			out = append(out,
				c.jsonElement(key, color, depth, func(gtx l.Context) l.Dimensions {
					return c.th.Flex().
						Rigid(c.th.Body1("\"" + res + "\"").Color(color).Fn).
						Rigid(c.th.Inset(0.25, p9.EmptySpace(0, 0)).Fn).
						Rigid(c.th.IconButton(clk).
							Background("").
							Inset(0).
							Color(color).
							Icon(c.th.Icon().Color("DocBg").Scale(1).Src(&icons.ContentContentCopy)).
							SetClick(func() {
								go clipboard.WriteAll(res)
							}).Fn,
						).Fn(gtx)
				}),
			)
		case float64:
			Debug("got type float64")
			res := res.value.(float64)
			clk := c.th.WidgetPool.GetClickable()
			out = append(out,
				c.jsonElement(key, color, depth, func(gtx l.Context) l.Dimensions {
					return c.th.Flex().
						Rigid(c.th.Body1(fmt.Sprint(res)).Color(color).Fn).
						Rigid(c.th.Inset(0.25, p9.EmptySpace(0, 0)).Fn).
						Rigid(c.th.IconButton(clk).
							Background("").
							Inset(0).
							Color(color).
							Icon(c.th.Icon().Color("DocBg").Scale(1).Src(&icons.ContentContentCopy)).
							SetClick(func() {
								go clipboard.WriteAll(fmt.Sprint(res))
							}).Fn,
						).Fn(gtx)
					// return c.th.ButtonLayout(clk).Embed(c.th.Body1().Color(color).Fn).Fn(gtx)
				}),
			)
		case bool:
			Debug("got type bool")
			res := res.value.(bool)
			out = append(out,
				c.jsonElement(key, color, depth, func(gtx l.Context) l.Dimensions {
					return c.th.Body1(fmt.Sprint(res)).Color(color).Fn(gtx)
				}),
			)
		}
	case string:
		Debug("got type string")
		res := in.(string)
		clk := c.th.WidgetPool.GetClickable()
		out = append(out,
			c.jsonElement(key, color, depth, func(gtx l.Context) l.Dimensions {
				return c.th.Flex().
					Rigid(c.th.Body1("\"" + res + "\"").Color(color).Fn).
					Rigid(c.th.Inset(0.25, p9.EmptySpace(0, 0)).Fn).
					Rigid(c.th.IconButton(clk).
						Background("").
						Inset(0).
						Color(color).
						Icon(c.th.Icon().Color("DocBg").Scale(1).Src(&icons.ContentContentCopy)).
						SetClick(func() {
							go clipboard.WriteAll(res)
						}).Fn,
					).Fn(gtx)
			}),
		)
	case float64:
		Debug("got type float64")
		res := in.(float64)
		clk := c.th.WidgetPool.GetClickable()
		out = append(out,
			c.jsonElement(key, color, depth, func(gtx l.Context) l.Dimensions {
				return c.th.Flex().
					Rigid(c.th.Body1(fmt.Sprint(res)).Color(color).Fn).
					Rigid(c.th.Inset(0.25, p9.EmptySpace(0, 0)).Fn).
					Rigid(c.th.IconButton(clk).
						Background("").
						Inset(0).
						Color(color).
						Icon(c.th.Icon().Color("DocBg").Scale(1).Src(&icons.ContentContentCopy)).
						SetClick(func() {
							go clipboard.WriteAll(fmt.Sprint(res))
						}).Fn,
					).Fn(gtx)
				// return c.th.ButtonLayout(clk).Embed(c.th.Body1(fmt.Sprint(res)).Color(color).Fn).Fn(gtx)
			}),
		)
	case bool:
		Debug("got type bool")
		res := in.(bool)
		out = append(out,
			c.jsonElement(key, color, depth, func(gtx l.Context) l.Dimensions {
				return c.th.Body1(fmt.Sprint(res)).Color(color).Fn(gtx)
			}),
		)
	default:
		Debugs(in)
	}
	return
}

func (c *Console) jsonElement(key, color string, depth int, w l.Widget) l.Widget {
	return func(gtx l.Context) l.Dimensions {
		return c.th.Flex().
			Rigid(c.getIndent(depth, 1,
				c.th.Body1(key).Font("bariol bold").Color(color).Fn)).
			Rigid(c.th.Inset(0.5, p9.EmptySpace(0, 0)).Fn).
			Rigid(w).
			Fn(gtx)
	}
}
