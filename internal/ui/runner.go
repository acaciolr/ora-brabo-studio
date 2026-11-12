// Executor das ações: formulário dos campos, pré-visualização editável do
// comando, confirmação digitada nas destrutivas e execução (SQL, relatório,
// grade ou RMAN pelo comando da conexão). by • DBA BRABO
package ui

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/acaciolr/ora-brabo-studio/internal/catalog"
	"github.com/acaciolr/ora-brabo-studio/internal/ora"
)

// RunAction executa uma ação do catálogo sobre a linha.
func (a *App) RunAction(c *Conn, x *catalog.Action, row catalog.Row, after func()) {
	if x.NeedRow && row == nil {
		a.Msg(L("Selecione uma linha primeiro.", "Select a row first."))
		return
	}
	if row == nil {
		row = catalog.Row{}
	}
	a.checkPack(c, x.Pack, func() {
		if x.Drill != nil {
			args := []any{}
			if x.DrillArgs != nil {
				args = x.DrillArgs(row)
			}
			title := x.Drill.Title
			if x.DrillName != nil {
				if n := x.DrillName(row); n != "" {
					title += " · " + n
				}
			}
			a.openView(c, x.Drill.Clone(title, args))
			return
		}
		a.askFields(x, row, func(vals catalog.Vals) {
			sc, err := x.Build(row, vals)
			if err != nil {
				a.Errf("%v", err)
				return
			}
			if x.Auto {
				a.execScript(c, x, row, sc, after)
				return
			}
			a.preview(c, x, row, sc, after)
		})
	})
}

// askFields mostra o formulário (pula se a ação não tem campos).
func (a *App) askFields(x *catalog.Action, row catalog.Row, done func(catalog.Vals)) {
	if len(x.Fields) == 0 {
		done(catalog.Vals{})
		return
	}
	f := tview.NewForm()
	f.SetBorder(true).SetTitle(" " + tview.Escape(x.Name) + " ")
	f.SetItemPadding(0)
	h := 4
	for _, fd := range x.Fields {
		label := fd.Label
		if label == "" {
			label = fd.Name
		}
		if fd.Required {
			label += " *"
		}
		def := catalog.Fill(fd.Default, row)
		switch fd.Kind {
		case catalog.Bool:
			f.AddCheckbox(label, catalog.Yes(def), nil)
		case catalog.Choice:
			i := 0
			for k, ch := range fd.Choices {
				if ch == def {
					i = k
				}
			}
			f.AddDropDown(label, fd.Choices, i, nil)
		case catalog.Password:
			f.AddPasswordField(label, def, 40, '*', nil)
		case catalog.Number:
			f.AddInputField(label, def, 20, nil, nil)
		case catalog.Multi:
			f.AddTextArea(label, def, 70, 5, 0, nil)
			h += 4
		default:
			f.AddInputField(label, def, 60, nil, nil)
		}
		h++
		if fd.Hint != "" {
			f.AddTextView("", "[gray]"+tview.Escape(fd.Hint)+"[-]", 70, 1, true, false)
			h++
		}
	}
	read := func() (catalog.Vals, error) {
		v := catalog.Vals{}
		i := 0
		for _, fd := range x.Fields {
			it := f.GetFormItem(i)
			i++
			if fd.Hint != "" {
				i++
			}
			var s string
			switch t := it.(type) {
			case *tview.Checkbox:
				if t.IsChecked() {
					s = "1"
				}
			case *tview.DropDown:
				_, s = t.GetCurrentOption()
			case *tview.InputField:
				s = t.GetText()
			case *tview.TextArea:
				s = t.GetText()
			}
			if fd.Required && strings.TrimSpace(s) == "" {
				lb := fd.Label
				if lb == "" {
					lb = fd.Name
				}
				return nil, fmt.Errorf(L("preencha: %s", "fill in: %s"), lb)
			}
			v[fd.Name] = s
		}
		return v, nil
	}
	f.AddButton(L("Continuar", "Continue"), func() {
		v, err := read()
		if err != nil {
			a.Errf("%v", err)
			return
		}
		a.CloseModal("fields")
		done(v)
	})
	f.AddButton(L("Cancelar", "Cancel"), func() { a.CloseModal("fields") })
	f.SetCancelFunc(func() { a.CloseModal("fields") })
	if h > 34 {
		h = 34
	}
	a.Modal("fields", f, 96, h+2)
}

func kindName(k catalog.ScriptKind) string {
	switch k {
	case catalog.RMAN:
		return "RMAN"
	case catalog.Report:
		return L("relatório", "report")
	case catalog.Grid:
		return L("consulta", "query")
	}
	return "SQL"
}

// preview mostra o comando (editável) antes de executar.
func (a *App) preview(c *Conn, x *catalog.Action, row catalog.Row, sc catalog.Script, after func()) {
	ed := tview.NewTextArea().SetText(sc.Text, false)
	ed.SetBorder(true).SetTitle(fmt.Sprintf(" %s · %s · %s ", tview.Escape(x.Name), kindName(sc.Kind), L("edite se quiser", "edit if you want")))
	if x.Danger {
		ed.SetBorderColor(tcell.ColorRed)
	}
	btn := tview.NewForm().SetHorizontal(true)
	run := func() {
		sc.Text = ed.GetText()
		a.CloseModal("preview")
		if x.Danger {
			a.confirmTyped(x, row, func() { a.execScript(c, x, row, sc, after) })
			return
		}
		a.execScript(c, x, row, sc, after)
	}
	btn.AddButton(L("Executar (Ctrl+G)", "Run (Ctrl+G)"), run)
	btn.AddButton(L("Copiar", "Copy"), func() { Copy(ed.GetText()); a.Msg(L("Comando copiado.", "Command copied.")) })
	btn.AddButton(L("Abrir na worksheet", "Open in worksheet"), func() {
		t := ed.GetText()
		a.CloseModal("preview")
		if w, err := NewWorksheet(a, c, t); err == nil {
			a.tabs.Add(w)
		} else {
			a.Errf("%v", err)
		}
	})
	btn.AddButton(L("Cancelar", "Cancel"), func() { a.CloseModal("preview") })
	btn.SetCancelFunc(func() { a.CloseModal("preview") })
	box := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(ed, 0, 1, true).AddItem(btn, 3, 0, false)
	keys := func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Key() {
		case tcell.KeyCtrlG:
			run()
			return nil
		case tcell.KeyEscape:
			a.CloseModal("preview")
			return nil
		case tcell.KeyTab:
			if ed.HasFocus() {
				a.tv.SetFocus(btn)
				return nil
			}
		case tcell.KeyBacktab:
			if btn.HasFocus() {
				a.tv.SetFocus(ed)
				return nil
			}
		}
		return ev
	}
	box.SetInputCapture(keys)
	lines := strings.Count(sc.Text, "\n") + 1
	h := lines + 6
	if h < 10 {
		h = 10
	}
	if h > 36 {
		h = 36
	}
	a.Modal("preview", box, 110, h)
	a.tv.SetFocus(ed)
	a.Msg(L("Revise o comando. [yellow]Ctrl+G[-] executa, [yellow]Tab[-] vai aos botões, [yellow]Esc[-] cancela.", "Review the command. [yellow]Ctrl+G[-] runs, [yellow]Tab[-] goes to buttons, [yellow]Esc[-] cancels."))
}

// confirmTyped pede para digitar o nome do alvo nas ações destrutivas.
func (a *App) confirmTyped(x *catalog.Action, row catalog.Row, yes func()) {
	want := row[x.Confirm]
	if want == "" {
		a.Confirm(fmt.Sprintf(L("%s\n\nAção destrutiva. Continuar?", "%s\n\nDestructive action. Continue?"), x.Name), yes)
		return
	}
	f := tview.NewForm()
	f.SetBorder(true).SetBorderColor(tcell.ColorRed).SetTitle(" " + L("Confirmação", "Confirmation") + " ")
	f.AddTextView("", fmt.Sprintf(L("%s é destrutiva.\nDigite [::b]%s[::-] para confirmar:", "%s is destructive.\nType [::b]%s[::-] to confirm:"), tview.Escape(x.Name), tview.Escape(want)), 70, 2, true, false)
	f.AddInputField("", "", 50, nil, nil)
	ok := func() {
		if strings.TrimSpace(f.GetFormItem(1).(*tview.InputField).GetText()) != want {
			a.Errf("%s", L("Não confere. Nada foi executado.", "Does not match. Nothing was run."))
			a.CloseModal("typed")
			return
		}
		a.CloseModal("typed")
		yes()
	}
	f.GetFormItem(1).(*tview.InputField).SetDoneFunc(func(k tcell.Key) {
		if k == tcell.KeyEnter {
			ok()
		}
	})
	f.AddButton(L("Executar", "Run"), ok)
	f.AddButton(L("Cancelar", "Cancel"), func() { a.CloseModal("typed") })
	f.SetCancelFunc(func() { a.CloseModal("typed") })
	a.Modal("typed", f, 80, 9)
	a.tv.SetFocus(f.GetFormItem(1))
}

// execScript roda o comando conforme o tipo.
func (a *App) execScript(c *Conn, x *catalog.Action, row catalog.Row, sc catalog.Script, after func()) {
	switch sc.Kind {
	case catalog.RMAN:
		a.runRMAN(c, x.Name, sc.Text)
	case catalog.Report:
		a.openText(c, x.Name, sc.Text, nil)
	case catalog.Grid:
		t := NewGridTab(a, c, x.Name, sc.Text)
		a.tabs.Add(t)
	default:
		a.Msg("[yellow]" + L("Executando: ", "Running: ") + tview.Escape(x.Name) + "…[-]")
		a.Go(func() func() {
			out, fails, outputs := runSQL(c, sc.Text)
			return func() {
				if fails == 0 && !outputs {
					a.Msg("[green]" + tview.Escape(x.Name) + ": OK[-]")
				} else {
					a.tabs.Add(NewTextTab(a, c, x.Name, out))
					if fails > 0 {
						a.Errf(L("%s: %d erro(s) (veja a aba)", "%s: %d error(s) (see the tab)"), x.Name, fails)
					}
				}
				if after != nil {
					after()
				}
			}
		})
	}
}

// runSQL roda o script em autocommit e devolve a saída (tipo SQL*Plus).
func runSQL(c *Conn, script string) (string, int, bool) {
	s, err := ora.NewSession(c.DB)
	if err != nil {
		return err.Error(), 1, true
	}
	defer s.Close()
	s.AutoCommit = true
	var b strings.Builder
	fails, outputs := 0, false
	for _, st := range ora.Split(script) {
		r := s.Run(st, nil)
		b.WriteString(scriptLine(r))
		if r.Err != nil {
			fails++
			break
		}
		if len(r.Cols) > 0 || len(r.Output) > 0 || r.Text != "" {
			outputs = true
		}
	}
	return b.String(), fails, outputs
}

// runRMAN manda o script ao comando RMAN da conexão (stdin) e mostra a saída.
func (a *App) runRMAN(c *Conn, title, script string) {
	if strings.TrimSpace(c.Info.RmanCmd) == "" {
		a.Alert(L("Esta conexão não tem o \"Comando RMAN\".\n\nEdite a conexão (tecla e na árvore) e informe, por exemplo:\n  ssh oracle@servidor '. ~/.bash_profile; rman target /'\n  docker exec -i meu-oracle rman target /\n\nO script foi copiado para a área de transferência.",
			"This connection has no \"RMAN command\".\n\nEdit the connection (key e in the tree) and set, for example:\n  ssh oracle@server '. ~/.bash_profile; rman target /'\n  docker exec -i my-oracle rman target /\n\nThe script was copied to the clipboard."))
		Copy(script)
		return
	}
	t := NewTextTab(a, c, "RMAN · "+title, "")
	t.Append("$ " + c.Info.RmanCmd + "\n" + script + "\n\n")
	a.tabs.Add(t)
	go func() {
		cmd := exec.Command("sh", "-c", c.Info.RmanCmd)
		cmd.Stdin = strings.NewReader(strings.TrimSpace(script) + "\nexit\n")
		pr, pw := io.Pipe()
		cmd.Stdout, cmd.Stderr = pw, pw
		t0 := time.Now()
		if err := cmd.Start(); err != nil {
			a.tv.QueueUpdateDraw(func() { t.Append(err.Error() + "\n") })
			return
		}
		done := make(chan struct{})
		go func() {
			sc := bufio.NewScanner(pr)
			sc.Buffer(make([]byte, 64*1024), 1<<20)
			for sc.Scan() {
				line := sc.Text() + "\n"
				a.tv.QueueUpdateDraw(func() { t.Append(line) })
			}
			close(done)
		}()
		err := cmd.Wait()
		pw.Close()
		<-done
		end := fmt.Sprintf(L("\n--- fim (%s)", "\n--- end (%s)"), time.Since(t0).Round(time.Second))
		if err != nil {
			end += ": " + err.Error()
		}
		a.tv.QueueUpdateDraw(func() { t.Append(end + "\n") })
	}()
}

// GridTab é uma consulta aberta numa grade.
type GridTab struct {
	*tview.Flex
	baseTab
	grid *Grid
	info *tview.TextView
	q    string
}

// NewGridTab cria a aba e roda a consulta.
func NewGridTab(a *App, c *Conn, title, q string) *GridTab {
	g := &GridTab{Flex: tview.NewFlex().SetDirection(tview.FlexRow), grid: NewGrid(a), info: tview.NewTextView().SetDynamicColors(true), q: q}
	g.AddItem(g.info, 1, 0, false).AddItem(g.grid, 0, 1, true)
	g.baseTab = baseTab{a: a, c: c, title: title, main: g.grid}
	a.SetContext(g.grid.Table, g.grid.ctxItems)
	a.mouseTable(g.grid.Table, func() {
		if i := g.grid.Selected(); i >= 0 {
			_, col := g.grid.Table.GetSelection()
			g.grid.showCell(i, col)
		}
	})
	g.grid.Keys = func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyF5 {
			g.load()
			return nil
		}
		return ev
	}
	g.load()
	return g
}

func (g *GridTab) Hint() string {
	return GridHint() + L("  [yellow]F5[-] atualiza", "  [yellow]F5[-] refresh")
}

func (g *GridTab) load() {
	g.info.SetText(" " + L("carregando…", "loading…"))
	g.a.Go(func() func() {
		r := Query(g.c, g.q, nil, 5000)
		return func() {
			if r.Err != nil {
				g.info.SetText(" [red]" + tview.Escape(r.Err.Error()) + "[-]")
				return
			}
			g.grid.SetResult(r)
			g.info.SetText(fmt.Sprintf(L(" %d linha(s) · %s", " %d row(s) · %s"), len(r.Rows), r.Elapsed.Round(time.Millisecond)))
		}
	})
}
