// Worksheet: editor SQL com sessão própria (sem autocommit, como no SQL
// Developer), execução do comando (F9) ou do script (F5), plano (F10),
// autotrace (F6), commit/rollback, binds, histórico e completar. by • DBA BRABO
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/acaciolr/ora-brabo-studio/internal/ora"
	"github.com/acaciolr/ora-brabo-studio/internal/store"
)

var wsSeq = map[string]int{}

// Worksheet é uma aba de SQL.
type Worksheet struct {
	*tview.Flex
	baseTab
	sess    *ora.Session
	tb      *tview.TextView // barra de ferramentas da worksheet
	ed      *tview.TextArea
	rbar    *tview.TextView // abas do resultado
	out     *tview.Pages
	grid    *Grid
	texts   map[string]*tview.TextView
	bufs    map[string]*strings.Builder
	info    *tview.TextView
	running bool
	file    string
	showing string // grid, script, plan, trace, dbms
	edW     int    // peso do editor (resultado = 10 - edW)
	lower   *tview.Flex
}

// abas do resultado, como no SQL Developer
var outPages = []string{"grid", "script", "plan", "trace", "dbms"}

func outTitle(p string) string {
	switch p {
	case "grid":
		return L("Resultado", "Query Result")
	case "script":
		return L("Saída do script", "Script Output")
	case "plan":
		return L("Plano", "Explain Plan")
	case "trace":
		return "Autotrace"
	}
	return "DBMS Output"
}

// NewWorksheet abre uma worksheet na conexão.
func NewWorksheet(a *App, c *Conn, text string) (*Worksheet, error) {
	s, err := ora.NewSession(c.DB)
	if err != nil {
		return nil, err
	}
	wsSeq[c.Info.Name]++
	title := c.Info.Name
	if n := wsSeq[c.Info.Name]; n > 1 {
		title = fmt.Sprintf("%s (%d)", c.Info.Name, n)
	}
	w := &Worksheet{Flex: tview.NewFlex().SetDirection(tview.FlexRow), sess: s, edW: 5,
		texts: map[string]*tview.TextView{}, bufs: map[string]*strings.Builder{}}
	w.baseTab = baseTab{a: a, c: c, title: title}
	w.tb = tview.NewTextView().SetDynamicColors(true).SetRegions(true).SetWrap(false)
	w.tb.SetBackgroundColor(tcell.NewRGBColor(24, 38, 46))
	w.tb.SetHighlightedFunc(func(added, _, _ []string) {
		if len(added) > 0 {
			w.tb.Highlight()
			w.toolbar(added[0])
		}
	})
	w.ed = tview.NewTextArea().SetPlaceholder(L("Digite o SQL. F9 executa o comando do cursor, F5 o script inteiro. F1 ajuda.",
		"Type SQL. F9 runs the statement at the cursor, F5 the whole script. F1 help."))
	w.ed.SetText(text, false)
	w.ed.SetBorder(true).SetBorderColor(tcell.NewRGBColor(70, 90, 100))
	w.ed.SetTitle(" " + L("Worksheet", "Worksheet") + " ").SetTitleAlign(tview.AlignLeft)
	w.ed.SetFocusFunc(func() { w.ed.SetBorderColor(tcell.NewRGBColor(46, 189, 89)); a.hint() })
	w.ed.SetBlurFunc(func() { w.ed.SetBorderColor(tcell.NewRGBColor(70, 90, 100)) })
	w.ed.SetClipboard(func(t string) { Copy(t) }, func() string { return clipboard })
	w.rbar = tview.NewTextView().SetDynamicColors(true).SetRegions(true).SetWrap(false)
	w.rbar.SetHighlightedFunc(func(added, _, _ []string) {
		if len(added) > 0 {
			w.show(added[0])
			w.focusOut()
		}
	})
	w.info = tview.NewTextView().SetDynamicColors(true)
	w.grid = NewGrid(a)
	w.out = tview.NewPages().AddPage("grid", w.grid, true, true)
	for _, p := range outPages[1:] {
		tv := tview.NewTextView().SetScrollable(true).SetWrap(false)
		tv.SetInputCapture(w.outKeys)
		w.texts[p], w.bufs[p] = tv, &strings.Builder{}
		w.out.AddPage(p, tv, true, false)
		a.SetContext(tv, w.textCtx(p))
		a.mouseText(tv)
	}
	w.showing = "grid"
	lower := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(w.rbar, 1, 0, false).AddItem(w.out, 0, 1, false).AddItem(w.info, 1, 0, false)
	w.AddItem(w.tb, 1, 0, false).AddItem(w.ed, 0, w.edW, true).AddItem(lower, 0, 10-w.edW, false)
	w.lower = lower
	w.main = w.ed
	w.ed.SetInputCapture(w.edKeys)
	w.grid.Keys = w.outKeys
	w.grid.OnEnter = func(i int, _ map[string]string) {
		_, col := w.grid.Table.GetSelection()
		w.grid.showCell(i, col)
	}
	a.SetContext(w.ed, w.edCtx)
	a.SetContext(w.grid.Table, w.grid.ctxItems)
	a.mouseText(w.ed)
	a.mouseTable(w.grid.Table, func() {
		if i := w.grid.Selected(); i >= 0 {
			_, col := w.grid.Table.GetSelection()
			w.grid.showCell(i, col)
		}
	})
	w.drawToolbar()
	w.drawRbar()
	w.drawInfo("")
	return w, nil
}

func (w *Worksheet) drawToolbar() {
	ac := "[gray]autocommit[-]"
	if w.sess.AutoCommit {
		ac = "[yellow::b]AUTOCOMMIT[-::-]"
	}
	b := []struct{ id, txt string }{
		{"trun", "[#2EBD59]▶[-] " + L("Executar", "Run") + " [gray]F9[-]"},
		{"tscript", "[#2EBD59]▷[-] Script [gray]F5[-]"},
		{"tplan", "[#9fd3c7]⇲[-] " + L("Plano", "Explain") + " [gray]F10[-]"},
		{"ttrace", "[#9fd3c7]⚑[-] Autotrace [gray]F6[-]"},
		{"tcommit", "[green]✓[-] Commit [gray]F11[-]"},
		{"troll", "[red]✗[-] Rollback [gray]F12[-]"},
		{"tstop", "[red]■[-] " + L("Cancelar", "Cancel")},
		{"tclear", "⌫ " + L("Limpar", "Clear")},
		{"thist", "⟲ " + L("Histórico", "History")},
		{"tac", ac},
	}
	var sb strings.Builder
	sb.WriteString(" ")
	for i, x := range b {
		if i == 2 || i == 4 || i == 6 || i == 8 {
			sb.WriteString("[gray]│[-] ")
		}
		fmt.Fprintf(&sb, `["%s"]%s[""]  `, x.id, x.txt)
	}
	w.tb.SetText(sb.String())
}

// toolbar executa um botão da barra (também chamado pela barra de cima).
func (w *Worksheet) toolbar(id string) {
	switch id {
	case "trun":
		w.runCurrent(false)
	case "tscript":
		w.runScript()
	case "tplan":
		w.explain()
	case "ttrace":
		w.runCurrent(true)
	case "tcommit":
		w.commit(true)
	case "troll":
		w.commit(false)
	case "tstop":
		if w.running {
			w.sess.Cancel()
			w.drawInfo(L("cancelando…", "cancelling…"))
		}
	case "tclear":
		w.ed.SetText("", false)
	case "thist":
		w.history()
	case "tac":
		w.toggleAutocommit()
	}
}

func (w *Worksheet) runScriptSafe() { w.runScript() }

func (w *Worksheet) toggleAutocommit() {
	if !w.sess.AutoCommit && w.sess.Pending {
		w.a.Confirm(L("Há alterações pendentes. Ligar o autocommit faz COMMIT delas. Continuar?", "There are pending changes. Turning autocommit on COMMITs them. Continue?"), func() {
			if err := w.sess.Commit(); err != nil {
				w.a.Errf("%v", err)
				return
			}
			w.sess.AutoCommit = true
			w.drawToolbar()
			w.drawInfo("")
		})
		return
	}
	w.sess.AutoCommit = !w.sess.AutoCommit
	w.drawToolbar()
	w.drawInfo("")
}

func (w *Worksheet) drawRbar() {
	var sb strings.Builder
	sb.WriteString(" ")
	for _, p := range outPages {
		t := outTitle(p)
		if p != "grid" && w.bufs[p].Len() > 0 && p != w.showing {
			t += " •"
		}
		if p == w.showing {
			fmt.Fprintf(&sb, `["%s"][black:#9fd3c7] %s [-:-:-][""] `, p, t)
		} else {
			fmt.Fprintf(&sb, `["%s"][#9fd3c7:#1e2d37] %s [-:-:-][""] `, p, t)
		}
	}
	w.rbar.SetText(sb.String())
}

// Close devolve a sessão (rollback do que estiver pendente).
func (w *Worksheet) Close() {
	if w.sess != nil {
		w.sess.Cancel()
		go w.sess.Close()
	}
}

func (w *Worksheet) focusOut() {
	if w.showing == "grid" {
		w.a.tv.SetFocus(w.grid)
	} else {
		w.a.tv.SetFocus(w.texts[w.showing])
	}
}

// FocusPart: 0 = editor, 1 = alterna editor/resultado.
func (w *Worksheet) FocusPart(step int) bool {
	if step == 0 {
		w.a.tv.SetFocus(w.ed)
		return true
	}
	if w.ed.HasFocus() {
		w.focusOut()
		return true
	}
	return false
}

func (w *Worksheet) Hint() string {
	if !w.ed.HasFocus() {
		return GridHint() + L("  [gray]│[-]  [yellow]Ctrl+PgUp/PgDn[-] abas do resultado  [gray]│[-]  [yellow]Esc[-] editor", "  [gray]│[-]  [yellow]Ctrl+PgUp/PgDn[-] result tabs  [gray]│[-]  [yellow]Esc[-] editor")
	}
	return L("[yellow]F9[-] executa  [yellow]F5[-] script  [yellow]F10[-] plano  [yellow]F6[-] autotrace  [yellow]F11[-]/[yellow]F12[-] commit/rollback  [yellow]Ctrl+Space[-] completar  [yellow]Ctrl+R[-] histórico  [yellow]Ctrl+↑↓[-] tamanho  [yellow]clique direito[-] menu",
		"[yellow]F9[-] run  [yellow]F5[-] script  [yellow]F10[-] plan  [yellow]F6[-] autotrace  [yellow]F11[-]/[yellow]F12[-] commit/rollback  [yellow]Ctrl+Space[-] complete  [yellow]Ctrl+R[-] history  [yellow]Ctrl+↑↓[-] size  [yellow]right-click[-] menu")
}

func (w *Worksheet) drawInfo(msg string) {
	flags := []string{}
	if w.sess.Pending {
		flags = append(flags, "[red::b]"+L("PENDENTE (sem commit)", "PENDING (uncommitted)")+"[-::-]")
	}
	if w.running {
		flags = append(flags, "[yellow]"+L("executando… (Esc cancela)", "running… (Esc cancels)")+"[-]")
	}
	if w.file != "" {
		flags = append(flags, "[gray]"+tview.Escape(filepath.Base(w.file))+"[-]")
	}
	w.info.SetText(" " + strings.Join(flags, "  ") + "  " + msg)
	w.drawToolbar()
}

func (w *Worksheet) resize(d int) {
	w.edW = min(max(w.edW+d, 1), 9)
	w.ResizeItem(w.ed, 0, w.edW).ResizeItem(w.lower, 0, 10-w.edW)
}

func (w *Worksheet) edKeys(ev *tcell.EventKey) *tcell.EventKey {
	ctrl := ev.Modifiers()&tcell.ModCtrl != 0
	switch ev.Key() {
	case tcell.KeyF9, tcell.KeyCtrlE:
		w.runCurrent(false)
		return nil
	case tcell.KeyF5:
		w.runScript()
		return nil
	case tcell.KeyF10:
		w.explain()
		return nil
	case tcell.KeyF6:
		w.runCurrent(true)
		return nil
	case tcell.KeyF11:
		w.commit(true)
		return nil
	case tcell.KeyF12:
		w.commit(false)
		return nil
	case tcell.KeyEscape:
		if w.running {
			w.sess.Cancel()
			w.drawInfo(L("cancelando…", "cancelling…"))
		}
		return nil
	case tcell.KeyCtrlSpace:
		w.complete()
		return nil
	case tcell.KeyCtrlR:
		w.history()
		return nil
	case tcell.KeyCtrlO:
		w.openFile()
		return nil
	case tcell.KeyCtrlS:
		w.saveFile()
		return nil
	case tcell.KeyCtrlT:
		w.toggleAutocommit()
		return nil
	case tcell.KeyUp, tcell.KeyDown:
		if ctrl {
			w.resize(map[tcell.Key]int{tcell.KeyUp: -1, tcell.KeyDown: 1}[ev.Key()])
			return nil
		}
	case tcell.KeyPgUp, tcell.KeyPgDn:
		if ctrl {
			return ev // troca de aba (global)
		}
	}
	return ev
}

func (w *Worksheet) outKeys(ev *tcell.EventKey) *tcell.EventKey {
	ctrl := ev.Modifiers()&tcell.ModCtrl != 0
	switch {
	case ev.Key() == tcell.KeyEscape:
		w.a.tv.SetFocus(w.ed)
		return nil
	case ctrl && (ev.Key() == tcell.KeyPgDn || ev.Key() == tcell.KeyPgUp):
		d := 1
		if ev.Key() == tcell.KeyPgUp {
			d = -1
		}
		i := 0
		for k, p := range outPages {
			if p == w.showing {
				i = k
			}
		}
		w.show(outPages[(i+d+len(outPages))%len(outPages)])
		w.focusOut()
		return nil
	case ctrl && (ev.Key() == tcell.KeyUp || ev.Key() == tcell.KeyDown):
		w.resize(map[tcell.Key]int{tcell.KeyUp: -1, tcell.KeyDown: 1}[ev.Key()])
		return nil
	case ev.Key() == tcell.KeyRune && ev.Rune() == 'y' && w.showing != "grid":
		Copy(w.bufs[w.showing].String())
		w.a.Msg(L("Saída copiada.", "Output copied."))
		return nil
	case ev.Key() == tcell.KeyCtrlC && w.showing != "grid":
		Copy(w.bufs[w.showing].String())
		w.a.Msg(L("Saída copiada.", "Output copied."))
		return nil
	}
	return ev
}

func (w *Worksheet) show(p string) {
	w.showing = p
	w.out.SwitchToPage(p)
	w.drawRbar()
	w.a.hint()
}

// edCtx é o clique direito do editor.
func (w *Worksheet) edCtx() []menuItem {
	k := func(key tcell.Key) func() {
		return func() {
			w.ed.InputHandler()(tcell.NewEventKey(key, 0, tcell.ModNone), func(p tview.Primitive) { w.a.tv.SetFocus(p) })
		}
	}
	return []menuItem{
		item(L("Executar comando", "Run statement"), "F9", func() { w.runCurrent(false) }),
		item(L("Executar script", "Run script"), "F5", w.runScript),
		item(L("Plano de execução", "Explain plan"), "F10", w.explain),
		item("Autotrace", "F6", func() { w.runCurrent(true) }),
		sep(),
		item(L("Recortar", "Cut"), "Ctrl+X", k(tcell.KeyCtrlX)),
		item(L("Copiar", "Copy"), "Ctrl+C", k(tcell.KeyCtrlQ)),
		item(L("Colar", "Paste"), "Ctrl+V", k(tcell.KeyCtrlV)),
		item(L("Selecionar tudo", "Select all"), "Ctrl+L", k(tcell.KeyCtrlL)),
		sep(),
		item(L("Completar", "Complete"), "Ctrl+Space", w.complete),
		item(L("Histórico de SQL", "SQL history"), "Ctrl+R", w.history),
		item(L("Salvar script", "Save script"), "Ctrl+S", w.saveFile),
	}
}

func (w *Worksheet) textCtx(p string) ctxProvider {
	return func() []menuItem {
		return []menuItem{
			item(L("Copiar tudo", "Copy all"), "y", func() { Copy(w.bufs[p].String()); w.a.Msg(L("Saída copiada.", "Output copied.")) }),
			item(L("Salvar em arquivo…", "Save to file…"), "", func() { w.a.SaveFile(sanitize(outTitle(p))+".txt", w.bufs[p].String()) }),
			item(L("Abrir numa worksheet", "Open in a worksheet"), "", func() {
				if nw, err := NewWorksheet(w.a, w.c, w.bufs[p].String()); err == nil {
					w.a.tabs.Add(nw)
				}
			}),
			item(L("Limpar", "Clear"), "", func() { w.clearOut(p); w.drawRbar() }),
		}
	}
}

// selection devolve (texto, início) da seleção ou ("", cursor).
func (w *Worksheet) selection() (string, int) {
	t, s, _ := w.ed.GetSelection()
	return t, s
}

// current acha o comando: a seleção, ou o do cursor.
func (w *Worksheet) current() (ora.Stmt, bool) {
	if sel, _ := w.selection(); strings.TrimSpace(sel) != "" {
		sts := ora.Split(sel)
		if len(sts) > 0 {
			return sts[0], true
		}
	}
	_, pos := w.selection()
	return ora.AtCursor(w.ed.GetText(), pos)
}

// appendOut escreve numa aba de texto do resultado.
func (w *Worksheet) appendOut(p, s string) {
	w.bufs[p].WriteString(s)
	fmt.Fprint(w.texts[p], s)
	w.texts[p].ScrollToEnd()
	w.drawRbar()
}

func (w *Worksheet) clearOut(p string) {
	w.bufs[p].Reset()
	w.texts[p].Clear()
}

func (w *Worksheet) setOut(p, s string) {
	w.clearOut(p)
	w.appendOut(p, s)
	w.texts[p].ScrollToBeginning()
}

// withBinds pede os valores das :variáveis e chama run.
func (w *Worksheet) withBinds(sts []ora.Stmt, run func(map[string]string)) {
	seen := map[string]bool{}
	var names []string
	for _, st := range sts {
		for _, b := range ora.Binds(st) {
			k := strings.ToUpper(b)
			if !seen[k] {
				seen[k] = true
				names = append(names, k)
			}
		}
	}
	if len(names) == 0 {
		run(nil)
		return
	}
	f := tview.NewForm()
	f.SetBorder(true).SetTitle(" " + L("Variáveis (vazio = NULL)", "Bind variables (empty = NULL)") + " ")
	for _, n := range names {
		f.AddInputField(":"+n, bindMemo[n], 40, nil, nil)
	}
	f.AddButton("OK", func() {
		vals := map[string]string{}
		for i, n := range names {
			vals[n] = f.GetFormItem(i).(*tview.InputField).GetText()
			bindMemo[n] = vals[n]
		}
		w.a.CloseModal("binds")
		run(vals)
	})
	f.AddButton(L("Cancelar", "Cancel"), func() { w.a.CloseModal("binds") })
	f.SetCancelFunc(func() { w.a.CloseModal("binds") })
	w.a.Modal("binds", f, 64, 5+2*len(names))
}

var bindMemo = map[string]string{}

func bindArgs(st ora.Stmt, vals map[string]string) []any {
	var out []any
	for _, b := range ora.Binds(st) {
		v := vals[strings.ToUpper(b)]
		if v == "" {
			out = append(out, nil)
		} else {
			out = append(out, v)
		}
	}
	return out
}

func (w *Worksheet) busy() bool {
	if w.running {
		w.a.Msg(L("Ainda executando: Esc cancela.", "Still running: Esc cancels."))
		return true
	}
	return false
}

// runCurrent executa o comando do cursor (trace = autotrace).
func (w *Worksheet) runCurrent(trace bool) {
	if w.busy() {
		return
	}
	st, ok := w.current()
	if !ok {
		w.a.Msg(L("Nada para executar no cursor.", "Nothing to run at the cursor."))
		return
	}
	w.withBinds([]ora.Stmt{st}, func(vals map[string]string) {
		w.running = true
		w.drawInfo("")
		binds := bindArgs(st, vals)
		w.a.Go(func() func() {
			var r ora.Result
			if trace && st.Kind == ora.KindQuery {
				r = w.sess.Autotrace(st, binds)
			} else {
				r = w.sess.Run(st, binds)
			}
			w.hist(st.Text, r)
			return func() {
				w.running = false
				page := ""
				if trace && st.Kind == ora.KindQuery {
					page = "trace"
				}
				w.showResult(r, page)
			}
		})
	})
}

func (w *Worksheet) hist(sql string, r ora.Result) {
	store.AddHist(store.Hist{When: time.Now(), Conn: w.c.Info.Name, SQL: strings.TrimSpace(sql), OK: r.Err == nil, Ms: r.Elapsed.Milliseconds()})
}

// showResult distribui o resultado nas abas (como no SQL Developer):
// consulta → Resultado; DML/DDL/erros → Saída do script; DBMS_OUTPUT → DBMS Output.
func (w *Worksheet) showResult(r ora.Result, page string) {
	for _, l := range r.Output {
		w.appendOut("dbms", l+"\n")
	}
	if r.Err != nil {
		w.appendOut("script", scriptLine(r))
		w.show("script")
		w.drawInfo("[red]" + tview.Escape(firstLine(r.Err.Error())) + "[-]")
		return
	}
	switch {
	case page == "plan" || page == "trace":
		w.setOut(page, r.Text)
		if len(r.Cols) > 0 {
			w.grid.SetResult(r)
		}
		w.show(page)
		w.drawInfo(fmt.Sprintf("%s %s", outTitle(page), r.Elapsed.Round(time.Millisecond)))
	case len(r.Cols) > 0:
		w.grid.SetResult(r)
		w.grid.Source = sourceTable(r.Stmt.Text)
		more := ""
		if r.More {
			more = fmt.Sprintf(L(" (primeiras %d; há mais)", " (first %d; more available)"), len(r.Rows))
		}
		w.show("grid")
		w.drawInfo(fmt.Sprintf(L("%d linha(s)%s em %s", "%d row(s)%s in %s"), len(r.Rows), more, r.Elapsed.Round(time.Millisecond)))
	default:
		w.appendOut("script", scriptLine(r))
		if len(r.Output) > 0 && r.Text == "" {
			w.show("dbms")
		} else {
			w.show("script")
		}
		w.drawInfo(fmt.Sprintf("%s %s", tview.Escape(firstLine(r.Msg)), r.Elapsed.Round(time.Millisecond)))
	}
}

func errText(r ora.Result) string {
	s := r.Err.Error()
	if r.Stmt.Line > 0 {
		s = fmt.Sprintf(L("Erro no comando da linha %d:\n%s", "Error in statement at line %d:\n%s"), r.Stmt.Line, s)
	}
	if r.Text != "" {
		s += "\n" + r.Text
	}
	return s
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// sourceTable pega a tabela de um "select ... from X" simples (para INSERT exportado).
func sourceTable(q string) string {
	f := strings.Fields(strings.ToUpper(q))
	for i, x := range f {
		if x == "FROM" && i+1 < len(f) {
			t := strings.TrimRight(f[i+1], ";,)")
			if !strings.ContainsAny(t, "(") {
				return t
			}
		}
	}
	return ""
}

// runScript executa tudo (ou a seleção) como no SQL*Plus.
func (w *Worksheet) runScript() {
	if w.busy() {
		return
	}
	text, _ := w.selection()
	if strings.TrimSpace(text) == "" {
		text = w.ed.GetText()
	}
	sts := ora.Split(text)
	if len(sts) == 0 {
		return
	}
	w.withBinds(sts, func(vals map[string]string) {
		w.running = true
		w.clearOut("script")
		w.show("script")
		w.drawInfo("")
		w.a.Go(func() func() {
			t0 := time.Now()
			var last ora.Result
			ok, fail := 0, 0
			for _, st := range sts {
				r := w.sess.Run(st, bindArgs(st, vals))
				w.hist(st.Text, r)
				out := scriptLine(r)
				dbms := r.Output
				w.a.tv.QueueUpdateDraw(func() {
					w.appendOut("script", out)
					for _, l := range dbms {
						w.appendOut("dbms", l+"\n")
					}
				})
				if r.Err != nil {
					fail++
				} else {
					ok++
				}
				if len(r.Cols) > 0 {
					last = r
				}
				if w.sess == nil {
					break
				}
			}
			el := time.Since(t0).Round(time.Millisecond)
			return func() {
				w.running = false
				if len(last.Cols) > 0 {
					w.grid.SetResult(last)
				}
				w.show("script")
				w.drawInfo(fmt.Sprintf(L("script: %d ok, %d erro(s) em %s  ·  clique nas abas do resultado", "script: %d ok, %d error(s) in %s  ·  click the result tabs"), ok, fail, el))
			}
		})
	})
}

// scriptLine é a saída de um comando no modo script.
func scriptLine(r ora.Result) string {
	var b strings.Builder
	if r.Stmt.Kind != ora.KindClient {
		fmt.Fprintf(&b, "SQL> %s\n", strings.ReplaceAll(strings.TrimSpace(r.Stmt.Text), "\n", "\n  2  "))
	}
	if r.Err != nil {
		b.WriteString(errText(r) + "\n\n")
		return b.String()
	}
	if len(r.Cols) > 0 {
		b.WriteString(asText(r))
		fmt.Fprintf(&b, L("\n%d linha(s) selecionada(s).\n\n", "\n%d row(s) selected.\n\n"), len(r.Rows))
	}
	if r.Text != "" {
		b.WriteString(r.Text + "\n")
	}
	for _, l := range r.Output {
		b.WriteString(l + "\n")
	}
	if r.Msg != "" {
		b.WriteString(r.Msg + "\n\n")
	}
	return b.String()
}

// asText formata a grade em colunas (saída de script).
func asText(r ora.Result) string {
	w := make([]int, len(r.Cols))
	for i, c := range r.Cols {
		w[i] = len([]rune(c))
	}
	cell := func(c ora.Cell) string {
		if c.Null {
			return ""
		}
		s := strings.ReplaceAll(c.V, "\n", " ")
		if len([]rune(s)) > 60 {
			s = string([]rune(s)[:59]) + "…"
		}
		return s
	}
	for _, row := range r.Rows {
		for i, c := range row {
			if n := len([]rune(cell(c))); n > w[i] {
				w[i] = n
			}
		}
	}
	var b strings.Builder
	for i, c := range r.Cols {
		fmt.Fprintf(&b, "%-*s ", w[i], c)
	}
	b.WriteString("\n")
	for i := range r.Cols {
		b.WriteString(strings.Repeat("-", w[i]) + " ")
	}
	b.WriteString("\n")
	for _, row := range r.Rows {
		for i, c := range row {
			fmt.Fprintf(&b, "%-*s ", w[i], cell(c))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (w *Worksheet) explain() {
	if w.busy() {
		return
	}
	st, ok := w.current()
	if !ok {
		return
	}
	w.running = true
	w.drawInfo("")
	w.a.Go(func() func() {
		r := w.sess.Explain(st)
		return func() {
			w.running = false
			w.showResult(r, "plan")
		}
	})
}

func (w *Worksheet) commit(ok bool) {
	if w.busy() {
		return
	}
	var err error
	if ok {
		err = w.sess.Commit()
	} else {
		err = w.sess.Rollback()
	}
	if err != nil {
		w.a.Errf("%v", err)
		return
	}
	w.appendOut("script", map[bool]string{true: L("Commit concluído.", "Commit complete."), false: L("Rollback concluído.", "Rollback complete.")}[ok]+"\n")
	w.drawInfo(map[bool]string{true: "[green]" + L("Commit concluído.", "Commit complete.") + "[-]", false: "[yellow]" + L("Rollback concluído.", "Rollback complete.") + "[-]"}[ok])
	w.a.tabs.Redraw()
}

// ---- histórico, arquivos, completar ----

func (w *Worksheet) history() {
	hs := store.LoadHist()
	l := tview.NewList().ShowSecondaryText(true).SetHighlightFullLine(true)
	l.SetBorder(true).SetTitle(" " + L("Histórico (Enter insere · Esc fecha)", "History (Enter inserts · Esc closes)") + " ")
	n := 0
	for i := len(hs) - 1; i >= 0 && n < 300; i-- {
		h := hs[i]
		if h.Conn != w.c.Info.Name && h.Conn != "" {
			continue
		}
		n++
		sql := h.SQL
		first := strings.ReplaceAll(sql, "\n", " ")
		if len([]rune(first)) > 110 {
			first = string([]rune(first)[:109]) + "…"
		}
		st := "[green]ok[-]"
		if !h.OK {
			st = "[red]erro[-]"
		}
		l.AddItem(tview.Escape(first), fmt.Sprintf("%s  %s  %d ms", h.When.Format("02/01 15:04:05"), st, h.Ms), 0, func() {
			w.a.CloseModal("hist")
			w.insert(sql + "\n")
		})
	}
	if n == 0 {
		l.AddItem(L("(sem histórico nesta conexão)", "(no history for this connection)"), "", 0, nil)
	}
	l.SetDoneFunc(func() { w.a.CloseModal("hist") })
	w.a.Modal("hist", l, 120, 30)
}

func (w *Worksheet) insert(s string) {
	_, start, end := w.ed.GetSelection()
	w.ed.Replace(start, end, s)
}

func (w *Worksheet) openFile() {
	home, _ := os.UserHomeDir()
	f := tview.NewForm()
	f.SetBorder(true).SetTitle(" " + L("Abrir script", "Open script") + " ")
	f.AddInputField(L("Arquivo", "File"), home+"/", 70, nil, nil)
	f.AddButton(L("Abrir", "Open"), func() {
		p := strings.TrimSpace(f.GetFormItem(0).(*tview.InputField).GetText())
		if strings.HasPrefix(p, "~/") {
			p = filepath.Join(home, p[2:])
		}
		b, err := os.ReadFile(p)
		if err != nil {
			w.a.Errf("%v", err)
			return
		}
		w.a.CloseModal("open")
		w.ed.SetText(string(b), false)
		w.file = p
		w.drawInfo("")
	})
	f.AddButton(L("Cancelar", "Cancel"), func() { w.a.CloseModal("open") })
	f.SetCancelFunc(func() { w.a.CloseModal("open") })
	w.a.Modal("open", f, 90, 7)
}

func (w *Worksheet) saveFile() {
	if w.file != "" {
		if err := os.WriteFile(w.file, []byte(w.ed.GetText()), 0o644); err != nil {
			w.a.Errf("%v", err)
			return
		}
		w.drawInfo("[green]" + L("gravado", "saved") + "[-]")
		return
	}
	w.a.SaveFile(sanitize(w.title)+".sql", w.ed.GetText())
}

var keywords = strings.Fields(`SELECT FROM WHERE AND OR NOT NULL IS IN EXISTS BETWEEN LIKE GROUP BY ORDER HAVING
 INSERT INTO VALUES UPDATE SET DELETE MERGE USING MATCHED THEN WHEN CASE ELSE END JOIN LEFT RIGHT FULL OUTER INNER CROSS ON
 UNION ALL INTERSECT MINUS DISTINCT AS WITH FETCH FIRST ROWS ONLY OFFSET CREATE ALTER DROP TRUNCATE TABLE VIEW INDEX SEQUENCE
 SYNONYM PROCEDURE FUNCTION PACKAGE BODY TRIGGER TYPE BEGIN DECLARE EXCEPTION RETURN COMMIT ROLLBACK GRANT REVOKE
 COUNT SUM AVG MIN MAX NVL NVL2 DECODE COALESCE TO_CHAR TO_DATE TO_NUMBER SYSDATE SYSTIMESTAMP TRUNC ROUND SUBSTR INSTR
 LENGTH UPPER LOWER TRIM LISTAGG ROW_NUMBER RANK DENSE_RANK OVER PARTITION ROWNUM ROWID DUAL DBMS_OUTPUT.PUT_LINE`)

func wordBefore(text string, pos int) (string, int) {
	r := []rune(text[:pos])
	i := len(r)
	for i > 0 && (unicode.IsLetter(r[i-1]) || unicode.IsDigit(r[i-1]) || strings.ContainsRune("_$#.", r[i-1])) {
		i--
	}
	return string(r[i:]), len(string(r[:i]))
}

func (w *Worksheet) complete() {
	_, pos, _ := w.ed.GetSelection()
	text := w.ed.GetText()
	word, start := wordBefore(text, pos)
	w.a.Go(func() func() {
		var cands []string
		if i := strings.LastIndex(word, "."); i >= 0 {
			obj := strings.ToUpper(word[:i])
			owner, name := ora.SplitName(obj)
			if owner == "" {
				owner = w.c.User
			}
			q := `select column_name from all_tab_columns where owner = :1 and table_name = :2 order by column_id`
			r := Query(w.c, q, []any{owner, name}, 2000)
			if len(r.Rows) == 0 {
				// alias: procura "TABELA alias" no texto
				if t := aliasTable(text, word[:i]); t != "" {
					o, n := ora.SplitName(strings.ToUpper(t))
					if o == "" {
						o = w.c.User
					}
					r = Query(w.c, q, []any{o, n}, 2000)
				}
			}
			for _, row := range r.Rows {
				cands = append(cands, word[:i+1]+row[0].V)
			}
		} else {
			w.c.mu.Lock()
			names := w.c.names
			w.c.mu.Unlock()
			if names == nil {
				r := Query(w.c, `select object_name from user_objects where object_type in ('TABLE','VIEW','SYNONYM','PACKAGE','PROCEDURE','FUNCTION','SEQUENCE','MATERIALIZED VIEW')
				   union select synonym_name from all_synonyms where owner = 'PUBLIC' and rownum <= 3000`, nil, 8000)
				for _, row := range r.Rows {
					names = append(names, row[0].V)
				}
				w.c.mu.Lock()
				w.c.names = names
				w.c.mu.Unlock()
			}
			up := strings.ToUpper(word)
			for _, k := range append(append([]string{}, keywords...), names...) {
				if strings.HasPrefix(k, up) && k != up {
					cands = append(cands, k)
				}
			}
			sort.Strings(cands)
		}
		return func() {
			if len(cands) == 0 {
				w.a.Msg(L("Nada para completar.", "Nothing to complete."))
				return
			}
			lower := word != "" && word == strings.ToLower(word)
			apply := func(s string) {
				if lower {
					s = strings.ToLower(s)
				}
				w.ed.Replace(start, pos, s)
			}
			if len(cands) == 1 {
				apply(cands[0])
				return
			}
			l := tview.NewList().ShowSecondaryText(false)
			l.SetBorder(true).SetTitle(" " + L("Completar", "Complete") + " ")
			for i, c := range cands {
				if i >= 200 {
					break
				}
				c := c
				l.AddItem(tview.Escape(c), "", 0, func() { w.a.CloseModal("complete"); apply(c) })
			}
			l.SetDoneFunc(func() { w.a.CloseModal("complete") })
			w.a.Modal("complete", l, 50, 16)
		}
	})
}

// aliasTable acha a tabela de um alias ("from emp e" → emp).
func aliasTable(text, alias string) string {
	f := strings.Fields(strings.NewReplacer(",", " , ", "\n", " ").Replace(text))
	for i := 1; i < len(f); i++ {
		if strings.EqualFold(f[i], alias) && i > 0 {
			p := f[i-1]
			if strings.EqualFold(p, "AS") && i > 1 {
				p = f[i-2]
			}
			up := strings.ToUpper(p)
			if up != "FROM" && up != "JOIN" && up != "," && up != "SELECT" {
				return p
			}
		}
	}
	return ""
}
