// Grade de resultados: rolagem, valor da célula, cópia, exportação (CSV,
// INSERT, JSON) e filtro rápido. by • DBA BRABO
package ui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/acaciolr/ora-brabo-studio/internal/ora"
)

const maxColWidth = 40

var (
	colHead = tcell.NewRGBColor(159, 211, 199)
	colNull = tcell.NewRGBColor(110, 110, 110)
	colNum  = tcell.NewRGBColor(230, 200, 120)
)

// Grid mostra um ora.Result.
type Grid struct {
	*tview.Table
	a      *App
	res    ora.Result
	rows   [][]ora.Cell // visíveis (após filtro / ordem)
	filter string
	sortBy int
	desc   bool
	Source string // tabela de origem (para o INSERT exportado)
	// OnEnter troca o Enter (padrão: abre o valor da célula)
	OnEnter func(row int, r map[string]string)
	// Keys trata letras antes da grade
	Keys func(ev *tcell.EventKey) *tcell.EventKey
}

// NewGrid cria a grade.
func NewGrid(a *App) *Grid {
	g := &Grid{Table: tview.NewTable().SetFixed(1, 0).SetSelectable(true, true).SetSeparator('│'), a: a, sortBy: -1}
	g.Table.SetSelectedStyle(tcell.StyleDefault.Background(tcell.NewRGBColor(40, 80, 95)).Foreground(tcell.ColorWhite))
	g.Table.SetBorders(false)
	g.Table.SetInputCapture(g.keys)
	g.Table.SetSelectedFunc(func(row, col int) {
		if row <= 0 || row-1 >= len(g.rows) {
			return
		}
		if g.OnEnter != nil {
			g.OnEnter(row-1, g.RowMap(row-1))
			return
		}
		g.showCell(row-1, col)
	})
	return g
}

// SetResult mostra o resultado (mantém a linha selecionada se der).
func (g *Grid) SetResult(r ora.Result) {
	sel, _ := g.Table.GetSelection()
	g.res = r
	g.apply()
	if sel > 0 && sel < g.Table.GetRowCount() {
		g.Table.Select(sel, 0)
	} else if g.Table.GetRowCount() > 1 {
		g.Table.Select(1, 0)
	}
	g.Table.ScrollToBeginning()
	if sel > 0 && sel < g.Table.GetRowCount() {
		g.Table.Select(sel, 0)
	}
}

// Result devolve o resultado atual.
func (g *Grid) Result() ora.Result { return g.res }

func (g *Grid) apply() {
	g.rows = g.rows[:0]
	f := strings.ToLower(g.filter)
	for _, row := range g.res.Rows {
		if f != "" {
			hit := false
			for _, c := range row {
				if strings.Contains(strings.ToLower(c.V), f) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		g.rows = append(g.rows, row)
	}
	if g.sortBy >= 0 && g.sortBy < len(g.res.Cols) {
		k := g.sortBy
		num := g.isNum(k)
		sort.SliceStable(g.rows, func(i, j int) bool {
			a, b := g.rows[i][k], g.rows[j][k]
			var less bool
			if a.Null != b.Null {
				less = b.Null
			} else if num {
				x, _ := strconv.ParseFloat(a.V, 64)
				y, _ := strconv.ParseFloat(b.V, 64)
				less = x < y
			} else {
				less = a.V < b.V
			}
			if g.desc {
				return !less && a != b
			}
			return less
		})
	}
	g.draw()
}

func (g *Grid) isNum(i int) bool {
	if i < len(g.res.Types) {
		t := strings.ToUpper(g.res.Types[i])
		return strings.Contains(t, "NUMBER") || strings.Contains(t, "FLOAT") || strings.Contains(t, "INT") || strings.Contains(t, "DOUBLE")
	}
	return false
}

func (g *Grid) draw() {
	g.Table.Clear()
	for i, c := range g.res.Cols {
		h := c
		if i == g.sortBy {
			h += map[bool]string{true: " ▼", false: " ▲"}[g.desc]
		}
		g.Table.SetCell(0, i, tview.NewTableCell(tview.Escape(h)).SetTextColor(colHead).SetAttributes(tcell.AttrBold).SetSelectable(false).SetMaxWidth(maxColWidth))
	}
	for r, row := range g.rows {
		for i, c := range row {
			txt, col, al := c.V, tcell.ColorWhite, tview.AlignLeft
			if c.Null {
				txt, col = "(null)", colNull
			} else if g.isNum(i) {
				col, al = colNum, tview.AlignRight
			}
			txt = strings.ReplaceAll(strings.ReplaceAll(txt, "\r", ""), "\n", "⏎")
			g.Table.SetCell(r+1, i, tview.NewTableCell(tview.Escape(txt)).SetTextColor(col).SetAlign(al).SetMaxWidth(maxColWidth))
		}
	}
}

// Count devolve (linhas visíveis, total).
func (g *Grid) Count() (int, int) { return len(g.rows), len(g.res.Rows) }

// Selected devolve a linha selecionada (índice em rows) ou -1.
func (g *Grid) Selected() int {
	r, _ := g.Table.GetSelection()
	if r <= 0 || r-1 >= len(g.rows) {
		return -1
	}
	return r - 1
}

// RowMap devolve a linha i como coluna → valor ("" para null).
func (g *Grid) RowMap(i int) map[string]string {
	m := map[string]string{}
	if i < 0 || i >= len(g.rows) {
		return m
	}
	for k, c := range g.res.Cols {
		if k < len(g.rows[i]) && !g.rows[i][k].Null {
			m[strings.ToUpper(c)] = g.rows[i][k].V
		}
	}
	return m
}

func (g *Grid) showCell(r, c int) {
	if r < 0 || r >= len(g.rows) || c < 0 || c >= len(g.res.Cols) {
		return
	}
	v := g.rows[r][c]
	txt := v.V
	if v.Null {
		txt = "(null)"
	}
	g.a.TextBox(g.res.Cols[c], txt)
}

func (g *Grid) recordText(r int) string {
	var b strings.Builder
	w := 0
	for _, c := range g.res.Cols {
		if len(c) > w {
			w = len(c)
		}
	}
	for k, c := range g.res.Cols {
		v := g.rows[r][k]
		s := v.V
		if v.Null {
			s = "(null)"
		}
		fmt.Fprintf(&b, "%-*s : %s\n", w, c, s)
	}
	return b.String()
}

func (g *Grid) keys(ev *tcell.EventKey) *tcell.EventKey {
	if g.Keys != nil {
		if ev = g.Keys(ev); ev == nil {
			return nil
		}
	}
	row, col := g.Table.GetSelection()
	r := row - 1
	switch ev.Key() {
	case tcell.KeyCtrlC:
		if r >= 0 && r < len(g.rows) {
			Copy(g.rows[r][col].V)
			g.a.Msg(L("Célula copiada.", "Cell copied."))
		}
		return nil
	}
	if ev.Key() != tcell.KeyRune {
		return ev
	}
	switch ev.Rune() {
	case 'v':
		g.showCell(r, col)
		return nil
	case 'R':
		if r >= 0 && r < len(g.rows) {
			g.a.TextBox(L("Registro", "Record"), g.recordText(r))
		}
		return nil
	case 'y':
		if r >= 0 && r < len(g.rows) {
			Copy(g.rows[r][col].V)
			g.a.Msg(L("Célula copiada.", "Cell copied."))
		}
		return nil
	case 'Y':
		Copy(g.Export("tsv"))
		g.a.Msg(fmt.Sprintf(L("%d linha(s) copiadas (com cabeçalho, separadas por TAB).", "%d row(s) copied (with header, TAB separated)."), len(g.rows)))
		return nil
	case 'o':
		if col >= 0 {
			if g.sortBy == col {
				g.desc = !g.desc
			} else {
				g.sortBy, g.desc = col, false
			}
			g.apply()
		}
		return nil
	case '/':
		g.askFilter()
		return nil
	case 'x':
		g.exportDialog()
		return nil
	}
	return ev
}

func (g *Grid) askFilter() {
	in := tview.NewInputField().SetLabel(L("Filtro: ", "Filter: ")).SetText(g.filter)
	in.SetBorder(true)
	in.SetDoneFunc(func(k tcell.Key) {
		if k == tcell.KeyEnter {
			g.filter = in.GetText()
			g.apply()
		}
		g.a.CloseModal("filter")
		g.a.tv.SetFocus(g.Table)
	})
	g.a.Modal("filter", in, 60, 3)
}

// Export devolve o resultado (visível) no formato pedido.
func (g *Grid) Export(format string) string {
	var b strings.Builder
	val := func(c ora.Cell) string {
		if c.Null {
			return ""
		}
		return c.V
	}
	switch format {
	case "tsv":
		b.WriteString(strings.Join(g.res.Cols, "\t") + "\n")
		for _, row := range g.rows {
			p := make([]string, len(row))
			for i, c := range row {
				p[i] = strings.ReplaceAll(val(c), "\t", " ")
			}
			b.WriteString(strings.Join(p, "\t") + "\n")
		}
	case "csv":
		q := func(s string) string {
			if strings.ContainsAny(s, ",\"\n\r;") {
				return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
			}
			return s
		}
		hs := make([]string, len(g.res.Cols))
		for i, h := range g.res.Cols {
			hs[i] = q(h)
		}
		b.WriteString(strings.Join(hs, ",") + "\n")
		for _, row := range g.rows {
			p := make([]string, len(row))
			for i, c := range row {
				p[i] = q(val(c))
			}
			b.WriteString(strings.Join(p, ",") + "\n")
		}
	case "insert":
		t := g.Source
		if t == "" {
			t = "TABELA"
		}
		cols := strings.Join(g.res.Cols, ", ")
		for _, row := range g.rows {
			p := make([]string, len(row))
			for i, c := range row {
				switch {
				case c.Null:
					p[i] = "NULL"
				case g.isNum(i):
					p[i] = c.V
				case i < len(g.res.Types) && strings.Contains(strings.ToUpper(g.res.Types[i]), "DATE"):
					p[i] = "TO_DATE('" + c.V + "', 'YYYY-MM-DD HH24:MI:SS')"
				case i < len(g.res.Types) && strings.Contains(strings.ToUpper(g.res.Types[i]), "TIMESTAMP"):
					p[i] = "TO_TIMESTAMP('" + c.V + "', 'YYYY-MM-DD HH24:MI:SS.FF')"
				default:
					p[i] = "'" + strings.ReplaceAll(c.V, "'", "''") + "'"
				}
			}
			fmt.Fprintf(&b, "INSERT INTO %s (%s) VALUES (%s);\n", t, cols, strings.Join(p, ", "))
		}
	case "json":
		out := make([]map[string]any, 0, len(g.rows))
		for _, row := range g.rows {
			m := map[string]any{}
			for i, c := range row {
				if c.Null {
					m[g.res.Cols[i]] = nil
				} else if g.isNum(i) {
					if f, err := strconv.ParseFloat(c.V, 64); err == nil {
						m[g.res.Cols[i]] = f
						continue
					}
					m[g.res.Cols[i]] = c.V
				} else {
					m[g.res.Cols[i]] = c.V
				}
			}
			out = append(out, m)
		}
		j, _ := json.MarshalIndent(out, "", "  ")
		b.Write(j)
		b.WriteByte('\n')
	}
	return b.String()
}

func (g *Grid) exportDialog() {
	m := tview.NewModal().SetText(fmt.Sprintf(L("Exportar %d linha(s) visíveis como:", "Export %d visible row(s) as:"), len(g.rows))).
		AddButtons([]string{"CSV", "INSERT", "JSON", L("Cancelar", "Cancel")})
	m.SetDoneFunc(func(i int, _ string) {
		g.a.CloseModal("export")
		ext := []string{"csv", "insert", "json"}
		if i < 0 || i > 2 {
			return
		}
		name := g.Source
		if name == "" {
			name = "resultado"
		}
		file := sanitize(strings.ToLower(name)) + map[string]string{"csv": ".csv", "insert": ".sql", "json": ".json"}[ext[i]]
		g.a.SaveFile(file, g.Export(ext[i]))
	})
	g.a.root.AddPage("export", m, true, true)
	g.a.modals++
	g.a.tv.SetFocus(m)
}

// GridHint é a linha de atalhos da grade.
func GridHint() string {
	return L("[yellow]Enter/v[-] valor  [yellow]R[-] registro  [yellow]y[-] copia célula  [yellow]Y[-] copia tudo  [yellow]o[-] ordena  [yellow]/[-] filtro  [yellow]x[-] exportar",
		"[yellow]Enter/v[-] value  [yellow]R[-] record  [yellow]y[-] copy cell  [yellow]Y[-] copy all  [yellow]o[-] sort  [yellow]/[-] filter  [yellow]x[-] export")
}
