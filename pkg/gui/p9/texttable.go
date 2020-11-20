package p9

import l "gioui.org/layout"

type TextTableHeaderItem struct {
	Text     string
	Priority int
}

type TextTableHeader []TextTableHeaderItem

type TextTableRow []string

type TextTableBody []TextTableRow

type TextTable struct {
	*Theme
	Header           TextTableHeader
	Body             TextTableBody
	HeaderColor      string
	HeaderBackground string
	HeaderFont       string
	HeaderFontScale  float32
	CellColor        string
	CellBackground   string
	CellFont         string
	CellFontScale    float32
	Inset            float32
	List             *List
}

func (tt *TextTable) Fn(gtx l.Context) l.Dimensions {
	// set defaults if unset
	if tt.HeaderColor == "" {
		tt.HeaderColor = "PanelText"
	}
	if tt.HeaderBackground == "" {
		tt.HeaderBackground = "PanelBg"
	}
	if tt.HeaderFont == "" {
		tt.HeaderFont = "bariol regular"
	}
	if tt.HeaderFontScale == 0 {
		tt.HeaderFontScale = Scales["Body1"]
	}
	if tt.CellColor == "" {
		tt.CellColor = "DocText"
	}
	if tt.CellBackground == "" {
		tt.CellBackground = "DocBg"
	}
	if tt.CellFont == "" {
		tt.CellFont = "bariol regular"
	}
	if tt.CellFontScale == 0 {
		tt.CellFontScale = Scales["Body1"]
	}
	// we assume the caller has intended a zero inset if it is zero
	var header CellRow
	for i := range tt.Header {
		header = append(header, Cell{
			Widget: tt.Theme.Fill(tt.HeaderBackground,
				tt.Theme.Inset(tt.Inset,
					tt.Theme.Body1(tt.Header[i].Text).
						Color(tt.HeaderColor).
						TextScale(tt.HeaderFontScale).
						Font(tt.HeaderFont).
						Fn,
				).Fn,
			).Fn,
		})
	}
	var body CellGrid
	for i := range tt.Body {
		row := CellRow{}
		for j := range tt.Body[i] {
			row = append(row, Cell{
				Widget: tt.Theme.Fill(tt.CellBackground,
					tt.Theme.Inset(tt.Inset,
						tt.Theme.Body1(tt.Body[i][j]).
							Color(tt.CellColor).
							TextScale(tt.CellFontScale).
							Font(tt.CellFont).
							Fn,
					).Fn,
				).Fn,
			})
		}
		body = append(body, row)
	}
	table := Table{
		th:     tt.Theme,
		header: header,
		body:   body,
		list:   tt.List,
	}
	return table.Fn(gtx)
}