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
	ed      *tview.TextArea
	info    *tview.TextView
	out     *tview.Pages
	grid    *Grid
	text    *tview.TextView
	textBuf strings.Builder
	running bool
	file    string
	showing string // "grid" ou "text"
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
	w := &Worksheet{Flex: tview.NewFlex().SetDirection(tview.FlexRow), sess: s}
	w.ed = tview.NewTextArea().SetPlaceholder(L("Digite o SQL. F9 executa o comando do cursor, F5 o script inteiro. F1 ajuda.",
		"Type SQL. F9 runs the statement at the cursor, F5 the whole script. F1 help."))
	w.ed.SetText(text, false)
	w.ed.SetBorder(true).SetBorderColor(tcell.NewRGBColor(70, 90, 100))
	w.info = tview.NewTextView().SetDynamicColors(true)
	w.grid = NewGrid(a)
	w.text = tview.NewTextView().SetScrollable(true).SetDynamicColors(false).SetWrap(false)
	w.out = tview.NewPages().AddPage("grid", w.grid, true, true).AddPage("text", w.text, true, false)
	w.showing = "grid"
	w.AddItem(w.ed, 0, 1, true).AddItem(w.info, 1, 0, false).AddItem(w.out, 0, 1, false)
	w.baseTab = baseTab{a: a, c: c, title: title, main: w.ed}
	w.ed.SetInputCapture(w.edKeys)
	w.text.SetInputCapture(w.outKeys)
	w.grid.Keys = w.outKeys
	w.drawInfo("")
	return w, nil
}

// Close devolve a sessão (rollback do que estiver pendente).
func (w *Worksheet) Close() {
	if w.sess != nil {
		w.sess.Cancel()
		go w.sess.Close()
	}
}

// FocusPart: 0 = editor, 1 = alterna editor/resultado.
func (w *Worksheet) FocusPart(step int) bool {
	if step == 0 {
		w.a.tv.SetFocus(w.ed)
		return true
	}
	if w.ed.HasFocus() {
		if w.showing == "grid" {
			w.a.tv.SetFocus(w.grid)
		} else {
			w.a.tv.SetFocus(w.text)
		}
		return true
	}
	return false
}

func (w *Worksheet) Hint() string {
	if w.grid.HasFocus() {
		return GridHint() + L("  [yellow]t[-] saída/grade  [yellow]F3[-] editor", "  [yellow]t[-] output/grid  [yellow]F3[-] editor")
	}
	return L("[yellow]F9[-] executa  [yellow]F5[-] script  [yellow]F10[-] plano  [yellow]F6[-] autotrace  [yellow]F11[-] commit  [yellow]F12[-] rollback  [yellow]Esc[-] cancela  [yellow]Ctrl+Space[-] completar  [yellow]Ctrl+R[-] histórico  [yellow]Ctrl+O/S[-] abrir/salvar  [yellow]Ctrl+T[-] autocommit  [yellow]F3[-] resultado",
		"[yellow]F9[-] run  [yellow]F5[-] script  [yellow]F10[-] plan  [yellow]F6[-] autotrace  [yellow]F11[-] commit  [yellow]F12[-] rollback  [yellow]Esc[-] cancel  [yellow]Ctrl+Space[-] complete  [yellow]Ctrl+R[-] history  [yellow]Ctrl+O/S[-] open/save  [yellow]Ctrl+T[-] autocommit  [yellow]F3[-] result")
}

func (w *Worksheet) drawInfo(msg string) {
	flags := []string{}
	if w.sess.AutoCommit {
		flags = append(flags, "[yellow]AUTOCOMMIT[-]")
	}
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
}

func (w *Worksheet) edKeys(ev *tcell.EventKey) *tcell.EventKey {
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
		w.sess.AutoCommit = !w.sess.AutoCommit
		w.drawInfo("")
		return nil
	}
	return ev
}

func (w *Worksheet) outKeys(ev *tcell.EventKey) *tcell.EventKey {
	if ev.Key() == tcell.KeyEscape {
		w.a.tv.SetFocus(w.ed)
		return nil
	}
	if ev.Key() == tcell.KeyRune && ev.Rune() == 't' {
		if w.showing == "grid" {
			w.show("text")
			w.a.tv.SetFocus(w.text)
		} else {
			w.show("grid")
			w.a.tv.SetFocus(w.grid)
		}
		return nil
	}
	if ev.Key() == tcell.KeyRune && ev.Rune() == 'y' && w.text.HasFocus() {
		Copy(w.textBuf.String())
		w.a.Msg(L("Saída copiada.", "Output copied."))
		return nil
	}
	return ev
}

func (w *Worksheet) show(p string) {
	w.showing = p
	w.out.SwitchToPage(p)
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

func (w *Worksheet) appendText(s string) {
	w.textBuf.WriteString(s)
	fmt.Fprint(w.text, s)
	w.text.ScrollToEnd()
}

func (w *Worksheet) clearText() {
	w.textBuf.Reset()
	w.text.Clear()
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
				w.showResult(r, true)
			}
		})
	})
}

func (w *Worksheet) hist(sql string, r ora.Result) {
	store.AddHist(store.Hist{When: time.Now(), Conn: w.c.Info.Name, SQL: strings.TrimSpace(sql), OK: r.Err == nil, Ms: r.Elapsed.Milliseconds()})
}

// showResult põe o resultado na grade ou no texto.
func (w *Worksheet) showResult(r ora.Result, clear bool) {
	if clear {
		w.clearText()
	}
	if r.Err != nil {
		w.appendText(errText(r) + "\n")
		w.show("text")
		w.drawInfo("[red]" + tview.Escape(firstLine(r.Err.Error())) + "[-]")
		return
	}
	for _, l := range r.Output {
		w.appendText(l + "\n")
	}
	switch {
	case len(r.Cols) > 0:
		w.grid.SetResult(r)
		w.grid.Source = sourceTable(r.Stmt.Text)
		more := ""
		if r.More {
			more = fmt.Sprintf(L(" (primeiras %d; há mais)", " (first %d; more available)"), len(r.Rows))
		}
		if r.Text != "" {
			w.appendText(r.Text)
		}
		if r.Text != "" || len(r.Output) > 0 {
			w.show("text")
		} else {
			w.show("grid")
		}
		w.drawInfo(fmt.Sprintf(L("%d linha(s)%s em %s", "%d row(s)%s in %s"), len(r.Rows), more, r.Elapsed.Round(time.Millisecond)))
	default:
		if r.Text != "" {
			w.appendText(r.Text)
		}
		if r.Msg != "" {
			w.appendText(r.Msg + "\n")
		}
		w.show("text")
		w.drawInfo(fmt.Sprintf("%s %s", tview.Escape(r.Msg), r.Elapsed.Round(time.Millisecond)))
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
		w.clearText()
		w.drawInfo("")
		w.a.Go(func() func() {
			t0 := time.Now()
			var last ora.Result
			ok, fail := 0, 0
			for _, st := range sts {
				r := w.sess.Run(st, bindArgs(st, vals))
				w.hist(st.Text, r)
				out := scriptLine(r)
				w.a.tv.QueueUpdateDraw(func() { w.appendText(out) })
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
				w.show("text")
				w.drawInfo(fmt.Sprintf(L("script: %d ok, %d erro(s) em %s  ·  t alterna saída/grade", "script: %d ok, %d error(s) in %s  ·  t toggles output/grid"), ok, fail, el))
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
			w.showResult(r, true)
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
