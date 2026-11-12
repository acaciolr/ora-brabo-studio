// Formulário de conexão (básica, TNS ou descritor; papel SYSDBA etc.; wallet;
// comando do RMAN), pedido de senha e importação do SQL Developer. by • DBA BRABO
package ui

import (
	"errors"
	"fmt"
	"sort"
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

// connForm cria (info nil) ou edita uma conexão.
func (a *App) connForm(info *ora.ConnInfo) {
	editing := info != nil && a.connExists(info.Name)
	c := ora.ConnInfo{Port: 1521, SavePwd: false}
	oldName := ""
	if info != nil {
		c = *info
		oldName = info.Name
	}
	roles, colors := rolesList(), colorsList()
	f := tview.NewForm()
	f.SetBorder(true).SetTitle(" " + map[bool]string{true: L("Editar conexão", "Edit connection"), false: L("Nova conexão", "New connection")}[editing] + " ")
	f.SetItemPadding(0)
	tipo := 0
	if c.TNS != "" {
		tipo = 1
	}
	f.AddInputField(L("Nome", "Name"), c.Name, 40, nil, nil)
	f.AddInputField(L("Usuário", "User"), c.User, 30, nil, nil)
	f.AddPasswordField(L("Senha", "Password"), c.Password, 30, '*', nil)
	f.AddCheckbox(L("Salvar senha (cifrada)", "Save password (encrypted)"), c.SavePwd, nil)
	f.AddDropDown(L("Papel", "Role"), roles, idx(roles, c.Role), nil)
	f.AddDropDown(L("Tipo", "Type"), []string{L("Básico (host/porta/serviço)", "Basic (host/port/service)"), L("TNS (alias ou descritor)", "TNS (alias or descriptor)")}, tipo, nil)
	f.AddInputField("Host", c.Host, 40, nil, nil)
	f.AddInputField(L("Porta", "Port"), strconv.Itoa(c.Port), 6, tview.InputFieldInteger, nil)
	f.AddInputField("Service name", c.Service, 30, nil, nil)
	f.AddInputField(L("SID (se não tiver service)", "SID (if no service)"), c.SID, 20, nil, nil)
	aliases := ora.TNSAliases()
	hint := ""
	if len(aliases) > 0 {
		var ks []string
		for k := range aliases {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		if len(ks) > 6 {
			ks = append(ks[:6], "…")
		}
		hint = " (" + strings.Join(ks, ", ") + ")"
	}
	f.AddInputField("TNS"+hint, c.TNS, 60, nil, nil)
	f.AddInputField(L("Wallet (pasta)", "Wallet (folder)"), c.Wallet, 50, nil, nil)
	f.AddCheckbox("SSL/TCPS", c.SSL, nil)
	f.AddInputField(L("Comando RMAN", "RMAN command"), c.RmanCmd, 60, nil, nil)
	f.AddDropDown(L("Cor da aba", "Tab color"), colors, idx(colors, c.Color), nil)

	read := func() (ora.ConnInfo, error) {
		get := func(l string) string { return strings.TrimSpace(f.GetFormItemByLabel(l).(*tview.InputField).GetText()) }
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
		_, cor := f.GetFormItemByLabel(L("Cor da aba", "Tab color")).(*tview.DropDown).GetCurrentOption()
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
			n.SID = get(L("SID (se não tiver service)", "SID (if no service)"))
		}
		if n.Name == "" {
			n.Name = n.User + "@" + strings.TrimPrefix(n.Display(), n.User+"@")
		}
		if n.User == "" {
			return n, errors.New(L("informe o usuário", "fill the user"))
		}
		if n.TNS == "" && n.Host == "" {
			return n, errors.New(L("informe host/serviço ou TNS", "fill host/service or TNS"))
		}
		if n.Name != oldName && a.connExists(n.Name) {
			return n, fmt.Errorf(L("já existe conexão com o nome %q", "a connection named %q already exists"), n.Name)
		}
		return n, nil
	}
	save := func(connect bool) {
		n, err := read()
		if err != nil {
			a.Errf("%v", err)
			return
		}
		if oldName != "" && a.connExists(oldName) {
			a.Disconnect(oldName)
			a.removeConn(oldName)
		}
		a.conns = append(a.conns, n)
		if err := store.SaveConns(a.conns); err != nil {
			a.Errf(L("erro salvando: %v", "error saving: %v"), err)
		}
		a.CloseModal("connform")
		a.buildConnTree()
		a.buildDBATree()
		a.Msg("[green]" + L("Conexão salva: ", "Connection saved: ") + tview.Escape(n.Name) + "[-]")
		if connect {
			a.newWorksheet(n.Name)
		}
	}
	f.AddButton(L("Testar", "Test"), func() {
		n, err := read()
		if err != nil {
			a.Errf("%v", err)
			return
		}
		a.Msg("[yellow]" + L("Testando…", "Testing…") + "[-]")
		a.Go(func() func() {
			db, err := ora.Open(n)
			if err != nil {
				return func() { a.Errf(L("Falhou: %v", "Failed: %v"), err) }
			}
			var banner string
			_ = db.QueryRow("select banner from v$version where rownum = 1").Scan(&banner)
			db.Close()
			return func() { a.Msg("[green]OK[-] " + tview.Escape(banner)) }
		})
	})
	f.AddButton(L("Salvar", "Save"), func() { save(false) })
	f.AddButton(L("Salvar e conectar", "Save and connect"), func() { save(true) })
	f.AddButton(L("Cancelar", "Cancel"), func() { a.CloseModal("connform") })
	f.SetCancelFunc(func() { a.CloseModal("connform") })
	a.Modal("connform", f, 92, 21)
	a.Msg(L("Comando RMAN: recebe o script pela entrada. Ex.: ssh oracle@db01 '. ~/.bash_profile; rman target /'  ou  docker exec -i meu-oracle rman target /",
		"RMAN command: receives the script on stdin. E.g. ssh oracle@db01 '. ~/.bash_profile; rman target /'  or  docker exec -i my-oracle rman target /"))
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
