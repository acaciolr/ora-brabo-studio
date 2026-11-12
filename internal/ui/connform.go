// Formulário de conexão (básica, TNS ou descritor; papel SYSDBA etc.; wallet;
// comando do RMAN), pedido de senha e importação do SQL Developer. by • DBA BRABO
package ui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/acaciolr/ora-brabo-studio/internal/ora"
	"github.com/acaciolr/ora-brabo-studio/internal/store"
)

// listas montadas na hora: o idioma pode mudar por --lang depois do init
func rolesList() []string {
	return []string{L("(padrão)", "(default)"), "SYSDBA", "SYSOPER", "SYSBACKUP", "SYSDG", "SYSKM"}
}
func colorsList() []string {
	return []string{L("(nenhuma)", "(none)"), "red", "orange", "yellow", "green", "blue", "purple"}
}

func idx(list []string, v string) int {
	for i, x := range list {
		if strings.EqualFold(x, v) {
			return i
		}
	}
	return 0
}

// connForm abre o diálogo de conexão já preenchido (cópia de conexão).
func (a *App) connForm(info *ora.ConnInfo) { a.connDialog("", info) }

// ConnectDialog é o "Conectar" (F9): conexões salvas à esquerda, formulário à
// direita, status embaixo, como no goSSMS / SQL Developer. sel pré-seleciona.
func (a *App) ConnectDialog(sel string) { a.connDialog(sel, nil) }

func (a *App) connDialog(sel string, pre *ora.ConnInfo) {
	roles, colors := rolesList(), colorsList()
	const page = "connform"
	if a.root.HasPage(page) {
		return
	}
	list := tview.NewList().ShowSecondaryText(true).SetHighlightFullLine(true)
	list.SetBorder(true).SetTitle(fmt.Sprintf(" %s (%d) ", L("Conexões salvas", "Saved connections"), len(a.conns))).SetTitleAlign(tview.AlignLeft)
	list.SetBackgroundColor(colModal)
	list.SetMainTextStyle(tcell.StyleDefault.Background(colModal).Foreground(tcell.ColorWhite))
	list.SetSecondaryTextStyle(tcell.StyleDefault.Background(colModal).Foreground(tcell.ColorGray))
	list.SetSelectedStyle(tcell.StyleDefault.Background(tcell.NewRGBColor(40, 80, 95)).Foreground(tcell.ColorWhite))
	status := tview.NewTextView().SetDynamicColors(true)
	status.SetBackgroundColor(colModal)
	holder := tview.NewFlex()
	holder.SetBackgroundColor(colModal)

	var f *tview.Form
	var oldName, hint string
	setStatus := func(s string) { status.SetText(" " + L("Status: ", "Status: ") + s) }
	get := func(l string) string { return strings.TrimSpace(f.GetFormItemByLabel(l).(*tview.InputField).GetText()) }

	read := func() (ora.ConnInfo, error) {
		n := ora.ConnInfo{
			Name: get(L("Nome", "Name")), User: get(L("Usuário", "User")),
			Password: f.GetFormItemByLabel(L("Senha", "Password")).(*tview.InputField).GetText(),
			SavePwd:  f.GetFormItemByLabel(L("Salvar senha (cifrada)", "Save password (encrypted)")).(*tview.Checkbox).IsChecked(),
			Wallet:   get(L("Wallet (pasta)", "Wallet (folder)")), SSL: f.GetFormItemByLabel("SSL/TCPS").(*tview.Checkbox).IsChecked(),
			RmanCmd: get(L("Comando RMAN", "RMAN command")),
		}
		_, role := f.GetFormItemByLabel(L("Papel", "Role")).(*tview.DropDown).GetCurrentOption()
		if role != roles[0] {
			n.Role = role
		}
		_, cor := f.GetFormItemByLabel(L("Cor", "Color")).(*tview.DropDown).GetCurrentOption()
		if cor != colors[0] {
			n.Color = cor
		}
		t, _ := f.GetFormItemByLabel(L("Tipo", "Type")).(*tview.DropDown).GetCurrentOption()
		if t == 1 {
			n.TNS = get("TNS" + hint)
		} else {
			n.Host = get("Host")
			n.Port, _ = strconv.Atoi(get(L("Porta", "Port")))
			n.Service = get("Service name")
			n.SID = get("SID")
		}
		if n.Name == "" {
			n.Name = n.Display()
		}
		if n.User == "" {
			return n, errors.New(L("informe o usuário", "fill the user"))
		}
		if n.TNS == "" && n.Host == "" {
			return n, errors.New(L("informe host/serviço ou TNS", "fill host/service or TNS"))
		}
		return n, nil
	}
	save := func() (ora.ConnInfo, bool) {
		n, err := read()
		if err != nil {
			setStatus("[red]" + tview.Escape(err.Error()) + "[-]")
			return n, false
		}
		if n.Name != oldName && a.connExists(n.Name) {
			setStatus("[red]" + tview.Escape(fmt.Sprintf(L("já existe conexão com o nome %q", "a connection named %q already exists"), n.Name)) + "[-]")
			return n, false
		}
		if oldName != "" && a.connExists(oldName) {
			if a.Connected(oldName) != nil && oldName != n.Name {
				a.disconnectTree(oldName)
			}
			a.removeConn(oldName)
		}
		a.conns = append(a.conns, n)
		if err := store.SaveConns(a.conns); err != nil {
			setStatus("[red]" + tview.Escape(err.Error()) + "[-]")
			return n, false
		}
		oldName = n.Name
		a.buildConnTree()
		a.buildDBATree()
		return n, true
	}
	var fill func(c ora.ConnInfo, name string)
	closeDlg := func() { a.CloseModal(page) }
	fill = func(c ora.ConnInfo, name string) {
		oldName = name
		if c.Port == 0 {
			c.Port = 1521
		}
		f = tview.NewForm()
		f.SetItemPadding(0).SetBorder(true).SetBorderPadding(0, 0, 1, 1)
		f.SetTitle(" " + map[bool]string{true: L("Propriedades da conexão", "Connection properties"), false: L("Nova conexão", "New connection")}[name != ""] + " ").SetTitleAlign(tview.AlignLeft)
		tipo := 0
		if c.TNS != "" {
			tipo = 1
		}
		f.AddInputField(L("Nome", "Name"), c.Name, 36, nil, nil)
		f.AddInputField(L("Usuário", "User"), c.User, 30, nil, nil)
		f.AddPasswordField(L("Senha", "Password"), c.Password, 30, '*', nil)
		f.AddCheckbox(L("Salvar senha (cifrada)", "Save password (encrypted)"), c.SavePwd, nil)
		f.AddDropDown(L("Papel", "Role"), roles, idx(roles, c.Role), nil)
		f.AddDropDown(L("Tipo", "Type"), []string{L("Básico", "Basic"), "TNS"}, tipo, nil)
		f.AddInputField("Host", c.Host, 36, nil, nil)
		f.AddInputField(L("Porta", "Port"), strconv.Itoa(c.Port), 6, tview.InputFieldInteger, nil)
		f.AddInputField("Service name", c.Service, 30, nil, nil)
		f.AddInputField("SID", c.SID, 16, nil, nil)
		hint = ""
		if al := ora.TNSAliases(); len(al) > 0 {
			hint = fmt.Sprintf(" (%d alias)", len(al))
		}
		f.AddInputField("TNS"+hint, c.TNS, 44, nil, nil)
		f.AddInputField(L("Wallet (pasta)", "Wallet (folder)"), c.Wallet, 36, nil, nil)
		f.AddCheckbox("SSL/TCPS", c.SSL, nil)
		f.AddInputField(L("Comando RMAN", "RMAN command"), c.RmanCmd, 44, nil, nil)
		f.AddDropDown(L("Cor", "Color"), colors, idx(colors, c.Color), nil)
		// tipo Básico × TNS: deixa cinza o que não se aplica
		gray := func(t int) {
			for _, l := range []string{"Host", L("Porta", "Port"), "Service name", "SID"} {
				f.GetFormItemByLabel(l).(*tview.InputField).SetDisabled(t == 1)
			}
			f.GetFormItemByLabel("TNS" + hint).(*tview.InputField).SetDisabled(t == 0)
		}
		gray(tipo)
		f.GetFormItemByLabel(L("Tipo", "Type")).(*tview.DropDown).SetSelectedFunc(func(_ string, i int) { gray(i) })

		f.AddButton(L("Testar", "Test"), func() {
			n, err := read()
			if err != nil {
				setStatus("[red]" + tview.Escape(err.Error()) + "[-]")
				return
			}
			setStatus("[yellow]" + L("testando…", "testing…") + "[-]")
			a.Go(func() func() {
				db, err := ora.Open(n)
				if err != nil {
					return func() { setStatus("[red]" + tview.Escape(L("Falhou: ", "Failed: ")+err.Error()) + "[-]") }
				}
				var banner string
				_ = db.QueryRow("select banner from v$version where rownum = 1").Scan(&banner)
				db.Close()
				return func() { setStatus("[green]" + L("Sucesso", "Success") + "[-]  " + tview.Escape(banner)) }
			})
		})
		f.AddButton(L("Salvar", "Save"), func() {
			if n, ok := save(); ok {
				setStatus("[green]" + L("Salva: ", "Saved: ") + tview.Escape(n.Name) + "[-]")
				refill(list, a, sel, n.Name)
				a.tv.SetFocus(f) // refill recria o formulário
			}
		})
		f.AddButton(L("Conectar", "Connect"), func() {
			n, ok := save()
			if !ok {
				return
			}
			closeDlg()
			a.connectAndOpen(n.Name)
		})
		f.AddButton(L("Limpar", "Clear"), func() { fill(ora.ConnInfo{}, ""); a.tv.SetFocus(f) })
		f.AddButton(L("Remover", "Delete"), func() {
			if oldName == "" || !a.connExists(oldName) {
				return
			}
			nm := oldName
			a.Confirm(fmt.Sprintf(L("Remover a conexão %q (e a senha salva)?", "Delete connection %q (and its saved password)?"), nm), func() {
				a.deleteConn(nm)
				refill(list, a, "", "")
				fill(ora.ConnInfo{}, "")
				setStatus(L("removida: ", "deleted: ") + tview.Escape(nm))
				a.tv.SetFocus(f)
			})
		})
		f.AddButton(L("Cancelar", "Cancel"), closeDlg)
		f.SetCancelFunc(closeDlg)
		solid(f)
		holder.Clear().AddItem(f, 0, 1, true)
	}
	list.SetChangedFunc(func(i int, main, _ string, _ rune) {
		if i < len(a.conns) {
			fill(a.conns[i], a.conns[i].Name)
		}
	})
	list.SetSelectedFunc(func(i int, _, _ string, _ rune) {
		if i < len(a.conns) {
			nm := a.conns[i].Name
			closeDlg()
			a.connectAndOpen(nm)
		}
	})
	refill(list, a, sel, sel)
	switch {
	case pre != nil:
		fill(*pre, "")
	case sel != "":
		if c, ok := a.findConn(sel); ok {
			fill(c, sel)
		}
	case len(a.conns) > 0:
		fill(a.conns[0], a.conns[0].Name)
	default:
		fill(ora.ConnInfo{}, "")
	}
	body := tview.NewFlex().AddItem(list, 32, 0, len(a.conns) > 0 && pre == nil).AddItem(holder, 0, 1, len(a.conns) == 0 || pre != nil)
	box := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(body, 0, 1, true).AddItem(status, 1, 0, false)
	box.SetBorder(true).SetTitle(" " + Icon() + " " + L("Conectar ao banco Oracle", "Connect to Oracle Database") + " ")
	box.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch {
		case ev.Key() == tcell.KeyEscape && list.HasFocus():
			closeDlg()
			return nil
		case ev.Key() == tcell.KeyTab && list.HasFocus(), ev.Key() == tcell.KeyRight && list.HasFocus():
			a.tv.SetFocus(f)
			return nil
		case ev.Key() == tcell.KeyF2:
			if list.HasFocus() {
				a.tv.SetFocus(f)
			} else if len(a.conns) > 0 {
				a.tv.SetFocus(list)
			}
			return nil
		}
		return ev
	})
	setStatus(L("[gray]Enter na lista conecta · Tab vai ao formulário · F2 alterna lista/formulário · Esc fecha[-]",
		"[gray]Enter in the list connects · Tab goes to the form · F2 toggles list/form · Esc closes[-]"))
	box.SetDrawFunc(func(_ tcell.Screen, x, y, w, h int) (int, int, int, int) {
		lw := 32
		if w < 90 {
			lw = 0 // tela estreita: esconde a lista (como o goSSMS)
		}
		body.ResizeItem(list, lw, 0)
		return x + 1, y + 1, w - 2, h - 2
	})
	a.Modal(page, box, 104, 26)
	if len(a.conns) > 0 && pre == nil {
		a.tv.SetFocus(list)
	} else {
		a.tv.SetFocus(f)
	}
}

// refill recarrega a lista de conexões salvas, selecionando sel.
func refill(list *tview.List, a *App, _ string, sel string) {
	list.Clear()
	cur := 0
	for i, c := range a.conns {
		st := "[gray]○[-]"
		if a.Connected(c.Name) != nil {
			st = "[#2EBD59]●[-]"
		}
		list.AddItem(st+" "+tview.Escape(c.Name), "   "+tview.Escape(c.Display()), 0, nil)
		if c.Name == sel {
			cur = i
		}
	}
	list.SetTitle(fmt.Sprintf(" %s (%d) ", L("Conexões salvas", "Saved connections"), len(a.conns)))
	if len(a.conns) > 0 {
		list.SetCurrentItem(cur)
	}
}

// connectAndOpen conecta, abre a conexão nas árvores e uma worksheet.
func (a *App) connectAndOpen(name string) {
	a.Connect(name, func(c *Conn) {
		a.markConnected(name)
		for _, n := range a.connT.GetRoot().GetChildren() {
			if r, ok := n.GetReference().(*nodeRef); ok && r.conn == name && r.kind == nkConn {
				a.fillConnNode(n, c)
				r.loaded = true
				n.SetExpanded(true)
				a.connT.SetCurrentNode(n)
			}
		}
		w, err := NewWorksheet(a, c, "")
		if err != nil {
			a.Errf("%v", err)
			return
		}
		a.tabs.Add(w)
	})
}

func (a *App) connExists(name string) bool {
	_, ok := a.findConn(name)
	return ok
}

func (a *App) removeConn(name string) {
	out := a.conns[:0]
	for _, c := range a.conns {
		if c.Name != name {
			out = append(out, c)
		}
	}
	a.conns = out
}

func (a *App) deleteConn(name string) {
	a.Disconnect(name)
	a.removeConn(name)
	if err := store.SaveConns(a.conns); err != nil {
		a.Errf("%v", err)
	}
	a.buildConnTree()
	a.buildDBATree()
}

func (a *App) askPassword(info ora.ConnInfo, done func(string)) {
	f := tview.NewForm()
	f.SetBorder(true).SetTitle(" " + L("Senha de ", "Password for ") + tview.Escape(info.Display()) + " ")
	f.AddPasswordField(L("Senha", "Password"), "", 30, '*', nil)
	ok := func() {
		pw := f.GetFormItem(0).(*tview.InputField).GetText()
		a.CloseModal("pwd")
		done(pw)
	}
	f.GetFormItem(0).(*tview.InputField).SetDoneFunc(func(k tcell.Key) {
		if k == tcell.KeyEnter {
			ok()
		}
	})
	f.AddButton("OK", ok)
	f.AddButton(L("Cancelar", "Cancel"), func() { a.CloseModal("pwd") })
	f.SetCancelFunc(func() { a.CloseModal("pwd") })
	a.Modal("pwd", f, 70, 7)
}

func (a *App) importSQLDev() {
	files := store.SQLDevConnFiles()
	f := tview.NewForm()
	f.SetBorder(true).SetTitle(" " + L("Importar conexões do SQL Developer", "Import SQL Developer connections") + " ")
	def := ""
	if len(files) > 0 {
		def = files[0]
	}
	f.AddInputField("connections.json", def, 70, nil, nil)
	f.AddTextView("", L("No SQL Developer: clique direito em Conexões > Exportar Conexões (ou use o connections.json da pasta system). As senhas não vêm: você digita no primeiro acesso.",
		"In SQL Developer: right-click Connections > Export Connections (or use connections.json from the system folder). Passwords are not imported: you type them on first use."), 70, 3, false, false)
	f.AddButton(L("Importar", "Import"), func() {
		p := strings.TrimSpace(f.GetFormItem(0).(*tview.InputField).GetText())
		cs, err := store.ImportSQLDev(p)
		if err != nil {
			a.Errf("%v", err)
			return
		}
		n := 0
		for _, c := range cs {
			if a.connExists(c.Name) {
				continue
			}
			a.conns = append(a.conns, c)
			n++
		}
		if err := store.SaveConns(a.conns); err != nil {
			a.Errf("%v", err)
			return
		}
		a.CloseModal("import")
		a.buildConnTree()
		a.buildDBATree()
		a.Msg(fmt.Sprintf(L("[green]%d conexão(ões) importada(s).[-]", "[green]%d connection(s) imported.[-]"), n))
	})
	f.AddButton(L("Cancelar", "Cancel"), func() { a.CloseModal("import") })
	f.SetCancelFunc(func() { a.CloseModal("import") })
	a.Modal("import", f, 90, 12)
}
