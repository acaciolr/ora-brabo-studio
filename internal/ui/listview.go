// Tela do catálogo DBA: lista (grade) + ações por letra, atualização manual
// (F5) ou automática (F4), drill para outras telas. by • DBA BRABO
package ui

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/acaciolr/ora-brabo-studio/internal/catalog"
	"github.com/acaciolr/ora-brabo-studio/internal/ora"
)

// ListView mostra uma View do catálogo.
type ListView struct {
	*tview.Flex
	baseTab
	v       *catalog.View
	grid    *Grid
	text    *tview.TextView
	info    *tview.TextView
	acts    *tview.TextView
	loading bool
	auto    bool
	stop    chan struct{}
	once    sync.Once
	last    time.Time
}

// NewListView cria a tela e carrega.
func NewListView(a *App, c *Conn, v *catalog.View) *ListView {
	l := &ListView{Flex: tview.NewFlex().SetDirection(tview.FlexRow), v: v, stop: make(chan struct{})}
	l.grid = NewGrid(a)
	l.text = tview.NewTextView().SetScrollable(true)
	l.info = tview.NewTextView().SetDynamicColors(true)
	l.acts = tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	main := tview.Primitive(l.grid)
	if v.Special == "text" {
		main = l.text
	}
	l.AddItem(l.info, 1, 0, false).AddItem(main, 0, 1, true).AddItem(l.acts, l.actsHeight(), 0, false)
	l.baseTab = baseTab{a: a, c: c, title: v.Title, main: main}
	l.grid.Keys = l.keys
	l.text.SetInputCapture(l.keys)
	l.grid.OnEnter = func(i int, _ map[string]string) {
		if v.Special == "record" {
			l.grid.showCell(i, 1)
			return
		}
		a.TextBox(L("Registro", "Record"), l.grid.recordText(i))
	}
	a.SetContext(l.grid.Table, l.ctxItems)
	a.SetContext(l.text, l.ctxItems)
	a.mouseTable(l.grid.Table, func() {
		if i := l.grid.Selected(); i >= 0 {
			l.grid.OnEnter(i, nil)
		}
	})
	a.mouseText(l.text)
	l.drawActs()
	l.Load()
	return l
}

// ctxItems: clique direito numa tela DBA = as ações da tela + atualizar/exportar.
func (l *ListView) ctxItems() []menuItem {
	row := l.row()
	var items []menuItem
	for _, x := range l.v.Actions {
		x := x
		lbl := x.Name
		if x.Danger {
			lbl = "⚠ " + lbl
		}
		if x.Drill != nil {
			lbl += " ›"
		}
		items = append(items, itemIf(!x.NeedRow || row != nil, lbl, string(x.Key), func() {
			l.a.RunAction(l.c, x, l.row(), func() { l.Load() })
		}))
	}
	if len(items) > 0 {
		items = append(items, sep())
	}
	auto := L("Ligar atualização automática", "Auto refresh on")
	if l.auto {
		auto = L("Desligar atualização automática", "Auto refresh off")
	}
	items = append(items,
		item(L("Atualizar", "Refresh"), "F5", l.Load),
		item(auto, "F4", l.toggleAuto),
		itemIf(row != nil, L("Registro (vertical)", "Single record view"), "Enter", func() {
			if i := l.grid.Selected(); i >= 0 {
				l.grid.OnEnter(i, nil)
			}
		}),
		item(L("Copiar tudo", "Copy all"), "Ctrl+Y", func() { Copy(l.grid.Export("tsv")); l.a.Msg(L("Lista copiada.", "List copied.")) }),
		item(L("Exportar…", "Export…"), "Ctrl+X", l.grid.exportDialog),
		item(L("Filtrar…", "Filter…"), "/", l.grid.askFilter),
		item(L("Ajuda da tela", "Screen help"), "?", l.help),
	)
	return items
}

func (l *ListView) actsHeight() int {
	n := 1
	w := 0
	for _, x := range l.v.Actions {
		w += len([]rune(x.Name)) + 5
	}
	n += w / 110
	if n > 4 {
		n = 4
	}
	return n
}

func (l *ListView) drawActs() {
	var b strings.Builder
	for _, x := range l.v.Actions {
		col := "[#2EBD59]"
		if x.Danger {
			col = "[red]"
		}
		fmt.Fprintf(&b, "%s%c[-] %s  ", col, x.Key, tview.Escape(x.Name))
	}
	if len(l.v.Actions) == 0 {
		b.WriteString("[gray]" + L("(só consulta)", "(read only)") + "[-]")
	}
	l.acts.SetText(b.String())
}

func (l *ListView) Hint() string {
	return L("[yellow]F5[-] atualiza  [yellow]F4[-] auto  [yellow]Enter[-] registro  [yellow]/[-] filtro  [yellow]Ctrl+X[-] exportar  [yellow]Ctrl+Y[-] copia tudo  [yellow]?[-] ajuda da tela  [yellow]letras[-] ações (linha de baixo)",
		"[yellow]F5[-] refresh  [yellow]F4[-] auto  [yellow]Enter[-] record  [yellow]/[-] filter  [yellow]Ctrl+X[-] export  [yellow]Ctrl+Y[-] copy all  [yellow]?[-] screen help  [yellow]letters[-] actions (bottom line)")
}

// Close para a atualização automática.
func (l *ListView) Close() { l.once.Do(func() { close(l.stop) }) }

// Load (re)lê a lista.
func (l *ListView) Load() {
	if l.loading {
		return
	}
	l.loading = true
	l.drawInfo(L("carregando…", "loading…"))
	c, v := l.c, l.v
	l.a.Go(func() func() {
		if q := v.RequiresSQL(); q != "" {
			if r := Query(c, q, nil, 1); r.Err == nil && len(r.Rows) > 0 && r.Rows[0][0].V == "0" {
				return func() {
					l.loading = false
					l.drawInfo("[yellow]" + L("Não disponível nesta base", "Not available on this database") + "[-]")
					msg := v.Missing
					l.text.SetText(msg)
					l.grid.SetResult(ora.Result{Cols: []string{L("AVISO", "NOTICE")}, Rows: [][]ora.Cell{{{V: msg}}}})
				}
			}
		}
		r := Query(c, v.SQL, v.Args, 5000)
		return func() {
			l.loading = false
			l.last = time.Now()
			if r.Err != nil {
				l.drawInfo("[red]" + tview.Escape(firstLine(r.Err.Error())) + "[-]")
				l.text.SetText(r.Err.Error())
				l.grid.SetResult(ora.Result{Cols: []string{L("ERRO", "ERROR")}, Rows: [][]ora.Cell{{{V: r.Err.Error()}}}})
				return
			}
			switch v.Special {
			case "record":
				r = vertical(r)
			case "text":
				l.text.SetText(TextOf(r))
			}
			l.grid.SetResult(r)
			more := ""
			if r.More {
				more = "+"
			}
			l.drawInfo(fmt.Sprintf(L("%d%s linha(s) · %s · %s", "%d%s row(s) · %s · %s"), len(r.Rows), more, r.Elapsed.Round(time.Millisecond), l.last.Format("15:04:05")))
		}
	})
}

// vertical vira uma linha em pares coluna/valor.
func vertical(r ora.Result) ora.Result {
	out := ora.Result{Cols: []string{L("ITEM", "ITEM"), L("VALOR", "VALUE")}, Elapsed: r.Elapsed}
	if len(r.Rows) == 0 {
		return out
	}
	for i, c := range r.Cols {
		out.Rows = append(out.Rows, []ora.Cell{{V: c}, r.Rows[0][i]})
	}
	return out
}

func (l *ListView) drawInfo(s string) {
	auto := ""
	if l.auto {
		auto = fmt.Sprintf(" [yellow]AUTO %ds[-]", l.interval())
	}
	pk := ""
	if l.v.Pack != "" {
		pk = " [orange]" + map[string]string{"diag": "Diagnostics Pack", "tuning": "Tuning Pack"}[l.v.Pack] + "[-]"
	}
	l.info.SetText(" " + s + auto + pk)
}

func (l *ListView) interval() int {
	if l.v.Refresh > 0 {
		return l.v.Refresh
	}
	return 10
}

func (l *ListView) toggleAuto() {
	l.auto = !l.auto
	l.drawInfo("")
	if !l.auto {
		return
	}
	go func() {
		t := time.NewTicker(time.Duration(l.interval()) * time.Second)
		defer t.Stop()
		for {
			select {
			case <-l.stop:
				return
			case <-t.C:
				stop := false
				l.a.tv.QueueUpdateDraw(func() {
					if !l.auto {
						stop = true
						return
					}
					if l.a.tabs.Current() == Tab(l) && l.a.modals == 0 {
						l.Load()
					}
				})
				if stop {
					return
				}
			}
		}
	}()
	l.Load()
}

// row devolve a linha selecionada como catalog.Row (nil se nenhuma).
func (l *ListView) row() catalog.Row {
	if l.v.Special == "record" {
		g := l.grid.Result()
		if len(g.Rows) == 0 {
			return nil
		}
		m := catalog.Row{}
		for _, r := range g.Rows {
			if !r[1].Null {
				m[strings.ToUpper(r[0].V)] = r[1].V
			}
		}
		return m
	}
	i := l.grid.Selected()
	if i < 0 {
		return nil
	}
	return catalog.Row(l.grid.RowMap(i))
}

func (l *ListView) keys(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyF5:
		l.Load()
		return nil
	case tcell.KeyF4:
		l.toggleAuto()
		return nil
	case tcell.KeyCtrlX:
		l.grid.exportDialog()
		return nil
	case tcell.KeyCtrlY:
		Copy(l.grid.Export("tsv"))
		l.a.Msg(L("Lista copiada (TAB).", "List copied (TAB)."))
		return nil
	case tcell.KeyRune:
	default:
		return ev
	}
	if ev.Rune() == '?' {
		l.help()
		return nil
	}
	if x := l.v.FindAction(ev.Rune()); x != nil {
		row := l.row()
		l.a.RunAction(l.c, x, row, func() { l.Load() })
		return nil
	}
	// letras sem ação: deixa a grade tratar (v, y, o, /...)
	return ev
}

func (l *ListView) help() {
	var b strings.Builder
	b.WriteString(l.v.Title + "\n\n")
	if l.v.Help != "" {
		b.WriteString(l.v.Help + "\n\n")
	}
	if l.v.Pack != "" {
		b.WriteString(L("Licença: ", "License: ") + map[string]string{"diag": "Oracle Diagnostics Pack", "tuning": "Oracle Tuning Pack (+ Diagnostics)"}[l.v.Pack] + "\n\n")
	}
	acts := append([]*catalog.Action{}, l.v.Actions...)
	sort.SliceStable(acts, func(i, j int) bool { return acts[i].Key < acts[j].Key })
	for _, x := range acts {
		tag := ""
		switch {
		case x.Danger:
			tag = L("  [destrutiva: confirma digitando]", "  [destructive: type to confirm]")
		case x.Drill != nil:
			tag = L("  [abre outra tela]", "  [opens another screen]")
		case x.Auto:
			tag = L("  [relatório]", "  [report]")
		}
		fmt.Fprintf(&b, "  %c  %s%s\n", x.Key, x.Name, tag)
	}
	b.WriteString("\n" + L("Consulta da tela:", "Screen query:") + "\n\n" + strings.TrimSpace(l.v.SQL) + "\n")
	l.a.TextBox(L("Ajuda da tela", "Screen help"), b.String())
}
