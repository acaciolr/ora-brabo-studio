// Visualizador de objeto: abas por tipo (Colunas, Dados, Constraints, DDL...),
// ações do tipo por letra e edição de dados por ROWID. by • DBA BRABO
package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/acaciolr/ora-brabo-studio/internal/catalog"
	"github.com/acaciolr/ora-brabo-studio/internal/ora"
)

// ObjView mostra um objeto.
type ObjView struct {
	*tview.Flex
	baseTab
	owner, name, typ string
	details          []catalog.Detail
	bar              *tview.TextView
	pages            *tview.Pages
	info             *tview.TextView
	acts             *tview.TextView
	grids            map[int]*Grid
	texts            map[int]*tview.TextView
	textOf           map[int]string
	cur              int
	where            string // filtro da aba Dados
}

func (a *App) openObject(c *Conn, owner, name, typ string) {
	title := name
	if owner != c.User {
		title = owner + "." + name
	}
	if i := a.tabs.Find(title, c); i >= 0 {
		a.tabs.Select(i)
		return
	}
	a.tabs.Add(NewObjView(a, c, owner, name, typ, title))
}

// NewObjView cria o visualizador.
func NewObjView(a *App, c *Conn, owner, name, typ, title string) *ObjView {
	o := &ObjView{Flex: tview.NewFlex().SetDirection(tview.FlexRow), owner: owner, name: name, typ: typ,
		details: catalog.Details(typ), grids: map[int]*Grid{}, texts: map[int]*tview.TextView{}, textOf: map[int]string{}}
	o.bar = tview.NewTextView().SetDynamicColors(true).SetRegions(true).SetWrap(false)
	o.pages = tview.NewPages()
	o.info = tview.NewTextView().SetDynamicColors(true)
	o.acts = tview.NewTextView().SetDynamicColors(true)
	o.bar.SetHighlightedFunc(func(added, _, _ []string) {
		if len(added) > 0 {
			if i, err := strconv.Atoi(added[0]); err == nil {
				o.selectDetail(i)
			}
		}
	})
	for i, d := range o.details {
		var p tview.Primitive
		if d.Text {
			tv := tview.NewTextView().SetScrollable(true).SetWrap(false)
			tv.SetInputCapture(o.keys)
			a.SetContext(tv, o.ctxFor(nil))
			a.mouseText(tv)
			o.texts[i] = tv
			p = tv
		} else {
			g := NewGrid(a)
			g.Keys = o.keys
			a.SetContext(g.Table, o.ctxFor(g))
			a.mouseTable(g.Table, func() {
				if i := g.Selected(); i >= 0 {
					_, col := g.Table.GetSelection()
					g.showCell(i, col)
				}
			})
			if d.Data {
				g.Source = catalog.Q2(owner, name)
			}
			o.grids[i] = g
			p = g
		}
		o.pages.AddPage(strconv.Itoa(i), p, true, i == 0)
	}
	var b strings.Builder
	for _, x := range catalog.ObjectActions(typ) {
		col := "[#2EBD59]"
		if x.Danger {
			col = "[red]"
		}
		fmt.Fprintf(&b, "%s%c[-] %s  ", col, x.Key, tview.Escape(x.Name))
	}
	b.WriteString("[#2EBD59]D[-] DDL  ")
	o.acts.SetText(b.String())
	o.AddItem(o.bar, 1, 0, false).AddItem(o.info, 1, 0, false).AddItem(o.pages, 0, 1, true).AddItem(o.acts, 1, 0, false)
	o.baseTab = baseTab{a: a, c: c, title: title}
	o.selectDetail(0)
	return o
}

func (o *ObjView) FocusPart(step int) bool {
	if step != 0 {
		return false
	}
	if g, ok := o.grids[o.cur]; ok {
		o.a.tv.SetFocus(g)
	} else if t, ok := o.texts[o.cur]; ok {
		o.a.tv.SetFocus(t)
	}
	return true
}

func (o *ObjView) Hint() string {
	h := L("[yellow]Tab / Shift+Tab[-] abas  [yellow]F5[-] atualiza  [yellow]letras[-] ações (linha de baixo)", "[yellow]Tab / Shift+Tab[-] tabs  [yellow]F5[-] refresh  [yellow]letters[-] actions (bottom line)")
	if o.details[o.cur].Data {
		h += L("  [yellow]Ctrl+E[-] editar célula  [yellow]Ctrl+N[-] nova linha  [yellow]Ctrl+D[-] apagar linha  [yellow]Ctrl+F[-] WHERE", "  [yellow]Ctrl+E[-] edit cell  [yellow]Ctrl+N[-] new row  [yellow]Ctrl+D[-] delete row  [yellow]Ctrl+F[-] WHERE")
	}
	return h + "  " + GridHint()
}

func (o *ObjView) drawBar() {
	var b strings.Builder
	for i, d := range o.details {
		if i == o.cur {
			fmt.Fprintf(&b, `["%d"][black:#9fd3c7] %s [-:-:-][""] `, i, tview.Escape(d.Title))
		} else {
			fmt.Fprintf(&b, `["%d"][#9fd3c7] %s [-][""] `, i, tview.Escape(d.Title))
		}
	}
	o.bar.SetText(b.String())
}

func (o *ObjView) selectDetail(i int) {
	if i < 0 || i >= len(o.details) {
		return
	}
	o.cur = i
	o.pages.SwitchToPage(strconv.Itoa(i))
	o.drawBar()
	o.FocusPart(0)
	if _, loaded := o.textOf[i]; !loaded {
		if g, ok := o.grids[i]; !ok || g.Table.GetRowCount() == 0 {
			o.load(i)
		}
	}
	o.a.hint()
}

func (o *ObjView) dataSQL() string {
	q := "select rowid, t.* from " + catalog.Q2(o.owner, o.name) + " t"
	if o.typ != "TABLE" {
		q = "select t.* from " + catalog.Q2(o.owner, o.name) + " t"
	}
	if w := strings.TrimSpace(o.where); w != "" {
		q += " where " + w
	}
	return q
}

func (o *ObjView) load(i int) {
	d := o.details[i]
	o.info.SetText(" " + L("carregando…", "loading…"))
	o.a.Go(func() func() {
		var r ora.Result
		var txt string
		var err error
		switch {
		case d.Data:
			r = Query(o.c, o.dataSQL(), nil, 1000)
		case d.Text && strings.Contains(d.SQL, "dbms_metadata"):
			txt, err = RunReport(o.c, catalog.DDLSetup()+"\n/\n"+d.SQL, []any{o.owner, o.name})
		default:
			r = Query(o.c, d.SQL, []any{o.owner, o.name}, 5000)
			if d.Text {
				txt = TextOf(r)
			}
		}
		if err != nil {
			r.Err = err
		}
		return func() {
			if r.Err != nil {
				o.info.SetText(" [red]" + tview.Escape(firstLine(r.Err.Error())) + "[-]")
				if t, ok := o.texts[i]; ok {
					t.SetText(r.Err.Error())
					o.textOf[i] = r.Err.Error()
				}
				return
			}
			if t, ok := o.texts[i]; ok {
				t.SetText(txt)
				t.ScrollToBeginning()
				o.textOf[i] = txt
				o.info.SetText(fmt.Sprintf(" %s.%s · %s", tview.Escape(o.owner), tview.Escape(o.name), strings.ToLower(o.typ)))
				return
			}
			g := o.grids[i]
			if d.Title == L("Detalhes", "Details") {
				r = vertical(r)
			}
			g.SetResult(r)
			more := ""
			if r.More {
				more = "+"
			}
			w := ""
			if d.Data && o.where != "" {
				w = " · WHERE " + tview.Escape(o.where)
			}
			o.info.SetText(fmt.Sprintf(L(" %s.%s · %d%s linha(s) · %s%s", " %s.%s · %d%s row(s) · %s%s"), tview.Escape(o.owner), tview.Escape(o.name), len(r.Rows), more, r.Elapsed.Round(time.Millisecond), w))
		}
	})
}

func (o *ObjView) keys(ev *tcell.EventKey) *tcell.EventKey {
	d := o.details[o.cur]
	switch ev.Key() {
	case tcell.KeyTab:
		o.selectDetail((o.cur + 1) % len(o.details))
		return nil
	case tcell.KeyBacktab:
		o.selectDetail((o.cur - 1 + len(o.details)) % len(o.details))
		return nil
	case tcell.KeyF5:
		delete(o.textOf, o.cur)
		o.load(o.cur)
		return nil
	case tcell.KeyCtrlE, tcell.KeyCtrlN, tcell.KeyCtrlD, tcell.KeyCtrlF:
		if d.Data {
			o.dataKey(ev.Key())
			return nil
		}
	case tcell.KeyRune:
		r := ev.Rune()
		if t, ok := o.texts[o.cur]; ok && t.HasFocus() && r == 'y' {
			Copy(o.textOf[o.cur])
			o.a.Msg(L("Texto copiado.", "Text copied."))
			return nil
		}
		if r == 'D' {
			o.a.openText(o.c, "DDL "+o.name, catalog.DDLSetup()+"\n/\n"+catalog.DDLQuery(o.typ), []any{o.owner, o.name})
			return nil
		}
		for _, x := range catalog.ObjectActions(o.typ) {
			if x.Key == r {
				o.a.RunAction(o.c, x, catalog.ObjRow(o.owner, o.name, o.typ), func() {
					for k := range o.textOf {
						delete(o.textOf, k)
					}
					o.load(o.cur)
				})
				return nil
			}
		}
	}
	return ev
}

// dataKey: edição de dados pela aba Dados (sempre com pré-visualização do DML).
func (o *ObjView) dataKey(k tcell.Key) {
	g := o.grids[o.cur]
	res := g.Result()
	refresh := func() { o.load(o.cur) }
	if k == tcell.KeyCtrlF {
		in := tview.NewInputField().SetLabel("WHERE ").SetText(o.where)
		in.SetBorder(true).SetTitle(" " + L("Filtro da aba Dados (vazio = tudo)", "Data tab filter (empty = all)") + " ")
		in.SetDoneFunc(func(key tcell.Key) {
			o.a.CloseModal("where")
			if key == tcell.KeyEnter {
				o.where = in.GetText()
				refresh()
			}
		})
		o.a.Modal("where", in, 90, 3)
		return
	}
	if o.typ != "TABLE" {
		o.a.Msg(L("Edição só em tabelas.", "Editing only on tables."))
		return
	}
	tbl := catalog.Q2(o.owner, o.name)
	row, col := g.Table.GetSelection()
	i := row - 1
	rowid := func() string {
		if i < 0 || i >= len(res.Rows) || len(res.Cols) == 0 || !strings.EqualFold(res.Cols[0], "ROWID") {
			return ""
		}
		for k, r := range g.rows[i] {
			if k == 0 {
				return r.V
			}
		}
		return ""
	}
	dml := func(name, sql string) {
		x := &catalog.Action{Name: name, Build: func(catalog.Row, catalog.Vals) (catalog.Script, error) { return catalog.Script{Text: sql}, nil }}
		sc := catalog.Script{Kind: catalog.SQL, Text: sql}
		o.a.preview(o.c, x, catalog.Row{}, sc, refresh)
	}
	switch k {
	case tcell.KeyCtrlE:
		rid := rowid()
		if rid == "" || col <= 0 {
			o.a.Msg(L("Escolha uma célula de dados.", "Pick a data cell."))
			return
		}
		cname := res.Cols[col]
		cur := g.rows[i][col]
		in := tview.NewInputField().SetLabel(cname + " = ").SetText(cur.V)
		in.SetBorder(true).SetTitle(" " + L("Novo valor (vazio = NULL; expressão: comece com =)", "New value (empty = NULL; expression: start with =)") + " ")
		in.SetDoneFunc(func(key tcell.Key) {
			o.a.CloseModal("cell")
			if key != tcell.KeyEnter {
				return
			}
			v := in.GetText()
			expr := "NULL"
			switch {
			case strings.HasPrefix(v, "="):
				expr = strings.TrimSpace(v[1:])
			case v == "":
			case g.isNum(col):
				expr = v
			case col < len(res.Types) && strings.Contains(strings.ToUpper(res.Types[col]), "DATE"):
				expr = "TO_DATE(" + catalog.Lit(v) + ", 'YYYY-MM-DD HH24:MI:SS')"
			default:
				expr = catalog.Lit(v)
			}
			dml(L("Alterar célula", "Update cell"), fmt.Sprintf("UPDATE %s SET %s = %s WHERE ROWID = %s;\nCOMMIT;", tbl, catalog.Ident(cname), expr, catalog.Lit(rid)))
		})
		o.a.Modal("cell", in, 100, 3)
	case tcell.KeyCtrlD:
		rid := rowid()
		if rid == "" {
			return
		}
		dml(L("Apagar linha", "Delete row"), fmt.Sprintf("DELETE FROM %s WHERE ROWID = %s;\nCOMMIT;", tbl, catalog.Lit(rid)))
	case tcell.KeyCtrlN:
		var cols, vals []string
		for _, c := range res.Cols {
			if strings.EqualFold(c, "ROWID") {
				continue
			}
			cols = append(cols, catalog.Ident(c))
			vals = append(vals, "NULL")
		}
		dml(L("Nova linha", "New row"), fmt.Sprintf("INSERT INTO %s (%s)\nVALUES (%s);\nCOMMIT;", tbl, strings.Join(cols, ", "), strings.Join(vals, ", ")))
	}
}

// ctxFor: clique direito no visualizador = ações do objeto (+ edição de dados e grade).
func (o *ObjView) ctxFor(g *Grid) ctxProvider {
	return func() []menuItem {
		var items []menuItem
		d := o.details[o.cur]
		if d.Data && o.typ == "TABLE" {
			items = append(items,
				item(L("Editar célula", "Edit cell"), "Ctrl+E", func() { o.dataKey(tcell.KeyCtrlE) }),
				item(L("Nova linha", "Insert row"), "Ctrl+N", func() { o.dataKey(tcell.KeyCtrlN) }),
				item(L("Apagar linha", "Delete row"), "Ctrl+D", func() { o.dataKey(tcell.KeyCtrlD) }),
				item(L("Filtro WHERE…", "WHERE filter…"), "Ctrl+F", func() { o.dataKey(tcell.KeyCtrlF) }),
				sep())
		}
		if g != nil {
			items = append(items, g.ctxItems()...)
			items = append(items, sep())
		} else if t, ok := o.texts[o.cur]; ok {
			txt := o.textOf[o.cur]
			_ = t
			items = append(items, item(L("Copiar tudo", "Copy all"), "y", func() { Copy(txt); o.a.Msg(L("Texto copiado.", "Text copied.")) }), sep())
		}
		items = append(items,
			item(L("Atualizar", "Refresh"), "F5", func() { delete(o.textOf, o.cur); o.load(o.cur) }),
			item(L("Mostrar DDL", "Show DDL"), "D", func() {
				o.a.openText(o.c, "DDL "+o.name, catalog.DDLSetup()+"\n/\n"+catalog.DDLQuery(o.typ), []any{o.owner, o.name})
			}), sep())
		for _, x := range catalog.ObjectActions(o.typ) {
			x := x
			lbl := x.Name
			if x.Danger {
				lbl = "⚠ " + lbl
			}
			items = append(items, item(lbl, string(x.Key), func() {
				o.a.RunAction(o.c, x, catalog.ObjRow(o.owner, o.name, o.typ), func() { o.load(o.cur) })
			}))
		}
		return items
	}
}
