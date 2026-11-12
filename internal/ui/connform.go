// Diálogo Conectar: tipos do SQL Developer (básica, TNS, Cloud Wallet, LDAP,
// URL personalizada), autenticação (senha, SO, Kerberos), proxy, wallet,
// SSL, túnel SSH, comando do RMAN; pedido de senha e importação do SQL
// Developer. by • DBA BRABO
package ui

import (
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
	return []string{L("(padrão)", "(default)"), "SYSDBA", "SYSOPER", "SYSBACKUP", "SYSDG", "SYSKM", "SYSRAC", "SYSASM"}
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

	d := &connDlg{a: a, list: list, holder: holder, page: page}
	d.setStatus = func(s string) { status.SetText(" " + L("Status: ", "Status: ") + s) }
	closeDlg := func() { a.CloseModal(page) }
	d.closeDlg = closeDlg

	list.SetChangedFunc(func(i int, _, _ string, _ rune) {
		if i < len(a.conns) {
			d.load(a.conns[i], a.conns[i].Name)
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
		d.load(*pre, "")
	case sel != "":
		if c, ok := a.findConn(sel); ok {
			d.load(c, sel)
		}
	case len(a.conns) > 0:
		d.load(a.conns[0], a.conns[0].Name)
	default:
		d.load(ora.ConnInfo{}, "")
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
			a.tv.SetFocus(d.f)
			return nil
		case ev.Key() == tcell.KeyF2:
			if list.HasFocus() {
				a.tv.SetFocus(d.f)
			} else if len(a.conns) > 0 {
				a.tv.SetFocus(list)
			}
			return nil
		case ev.Key() == tcell.KeyPgDn && ev.Modifiers()&tcell.ModCtrl != 0, ev.Key() == tcell.KeyPgUp && ev.Modifiers()&tcell.ModCtrl != 0:
			step := 1
			if ev.Key() == tcell.KeyPgUp {
				step = len(dlgTabs) - 1
			}
			d.tab = (d.tab + step) % len(dlgTabs)
			d.build()
			a.tv.SetFocus(d.f)
			return nil
		}
		return ev
	})
	d.setStatus(L("[gray]Enter na lista conecta · Tab vai ao formulário · F2 lista/formulário · Ctrl+PgUp/PgDn abas · Esc fecha[-]",
		"[gray]Enter in the list connects · Tab goes to the form · F2 list/form · Ctrl+PgUp/PgDn tabs · Esc closes[-]"))
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
		a.tv.SetFocus(d.f)
	}
}

// abas do formulário de conexão (como as abas de baixo do SQL Developer)
var dlgTabs = []string{"main", "adv", "ssh"}

func dlgTabTitle(t string) string {
	switch t {
	case "adv":
		return L("Avançado", "Advanced")
	case "ssh":
		return "SSH"
	}
	return L("Principal", "Main")
}

func typesList() []string {
	return []string{L("Básico", "Basic"), "TNS", "Cloud Wallet", "LDAP", L("URL personalizada", "Custom URL")}
}

var typeKeys = []string{ora.TypeBasic, ora.TypeTNS, ora.TypeCloud, ora.TypeLDAP, ora.TypeCustom}

func authList() []string {
	return []string{L("Usuário e senha", "User and password"), L("Sistema operacional (SO)", "Operating system (OS)"), "Kerberos"}
}

var authKeys = []string{ora.AuthPassword, ora.AuthOS, ora.AuthKerberos}

// connDlg guarda o estado do diálogo: cur é a conexão sendo editada; os
// campos escrevem direto nela, e o formulário é refeito ao trocar de aba ou
// de tipo (cada tipo mostra só os campos dele).
type connDlg struct {
	a         *App
	list      *tview.List
	holder    *tview.Flex
	f         *tview.Form
	page      string
	cur       ora.ConnInfo
	oldName   string
	tab       int
	svcCache  map[string][]string
	setStatus func(string)
	closeDlg  func()
}

func (d *connDlg) load(c ora.ConnInfo, name string) {
	if c.Port == 0 {
		c.Port = 1521
	}
	if c.Type == "" {
		c.Type = c.Kind()
	}
	d.cur, d.oldName = c, name
	d.build()
}

func (d *connDlg) input(label string, v *string, w int) *tview.InputField {
	d.f.AddInputField(label, *v, w, nil, func(t string) { *v = strings.TrimSpace(t) })
	return d.f.GetFormItem(d.f.GetFormItemCount() - 1).(*tview.InputField)
}

func (d *connDlg) secret(label string, v *string, w int) {
	d.f.AddPasswordField(label, *v, w, '*', func(t string) { *v = t })
}

func (d *connDlg) check(label string, v *bool) {
	d.f.AddCheckbox(label, *v, func(b bool) { *v = b })
}

func (d *connDlg) number(label string, v *int, def int) {
	s := ""
	if *v != 0 {
		s = strconv.Itoa(*v)
	} else if def != 0 {
		s = strconv.Itoa(def)
	}
	d.f.AddInputField(label, s, 7, tview.InputFieldInteger, func(t string) { *v, _ = strconv.Atoi(t) })
}

// choose: dropdown que grava a chave escolhida; rebuild refaz o formulário.
func (d *connDlg) choose(label string, opts, keys []string, v *string, rebuild bool) {
	i := idx(keys, *v)
	d.f.AddDropDown(label, opts, i, nil)
	dd := d.f.GetFormItem(d.f.GetFormItemCount() - 1).(*tview.DropDown)
	pos := d.f.GetFormItemCount() - 1
	dd.SetSelectedFunc(func(_ string, j int) {
		if keys[j] == *v {
			return
		}
		*v = keys[j]
		if rebuild {
			d.a.tv.QueueUpdateDraw(func() {
				d.build()
				d.f.SetFocus(pos)
				d.a.tv.SetFocus(d.f)
			})
		}
	})
}

// complete liga a lista de sugestões (aliases, serviços) a um campo.
func complete(in *tview.InputField, opts func() []string) {
	in.SetAutocompleteFunc(func(t string) []string {
		var out []string
		up := strings.ToUpper(t)
		for _, o := range opts() {
			if strings.Contains(strings.ToUpper(o), up) {
				out = append(out, o)
			}
		}
		if len(out) > 30 {
			out = out[:30]
		}
		return out
	})
}

func (d *connDlg) build() {
	c := &d.cur
	f := tview.NewForm()
	d.f = f
	f.SetItemPadding(0).SetBorder(true).SetBorderPadding(0, 0, 1, 1)
	var tt []string
	for i, t := range dlgTabs {
		lbl := dlgTabTitle(t)
		if i == d.tab {
			lbl = "[#0f1a20:#2EBD59:b] " + lbl + " [-:-:-]"
		} else {
			lbl = "[gray] " + lbl + " [-]"
		}
		tt = append(tt, lbl)
	}
	head := L("Nova conexão", "New connection")
	if d.oldName != "" {
		head = L("Propriedades da conexão", "Connection properties")
	}
	f.SetTitle(" " + head + "  " + strings.Join(tt, "") + " ").SetTitleAlign(tview.AlignLeft)
	f.SetTitleColor(tcell.ColorWhite)
	// clique nas abas do título
	f.SetMouseCapture(func(act tview.MouseAction, ev *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		x0, y0, _, _ := f.GetRect()
		mx, my := ev.Position()
		if act != tview.MouseLeftClick || my != y0 {
			return act, ev
		}
		pos := x0 + 1 + len([]rune(" "+head+"  "))
		for i, t := range dlgTabs {
			w := len([]rune(dlgTabTitle(t))) + 2
			if mx >= pos && mx < pos+w {
				d.tab = i
				d.a.tv.QueueUpdateDraw(func() { d.build(); d.a.tv.SetFocus(d.f) })
				return tview.MouseConsumed, nil
			}
			pos += w
		}
		return act, ev
	})
	switch dlgTabs[d.tab] {
	case "adv":
		d.input(L("Usuário proxy (cliente)", "Proxy user (client)"), &c.Proxy, 30)
		d.input(L("Wallet (pasta)", "Wallet (folder)"), &c.Wallet, 0)
		d.secret(L("Senha do wallet", "Wallet password"), &c.WalletPwd, 30)
		d.check("SSL/TCPS", &c.SSL)
		d.check(L("Verificar certificado do servidor", "Verify server certificate"), &c.SSLVerify)
		d.input(L("Pasta do tnsnames.ora", "tnsnames.ora folder"), &c.TNSAdmin, 0)
		d.input(L("Comando RMAN", "RMAN command"), &c.RmanCmd, 0)
		cols := colorsList()
		keys := append([]string{""}, cols[1:]...)
		d.choose(L("Cor", "Color"), cols, keys, &c.Color, false)
	case "ssh":
		d.input(L("Host SSH (vazio = sem túnel)", "SSH host (empty = no tunnel)"), &c.SSHHost, 40)
		d.number(L("Porta SSH", "SSH port"), &c.SSHPort, 22)
		d.input(L("Usuário SSH", "SSH user"), &c.SSHUser, 24)
		d.input(L("Chave privada (arquivo)", "Private key (file)"), &c.SSHKey, 0)
		d.secret(L("Senha / frase da chave", "Password / key passphrase"), &c.SSHPwd, 30)
		f.AddTextView("", L("[gray]Sem chave: usa o agente SSH e as chaves de ~/.ssh. A chave do host é gravada no primeiro acesso e conferida depois.[-]",
			"[gray]No key: uses the SSH agent and ~/.ssh keys. The host key is saved on first use and checked afterwards.[-]"), 0, 4, true, false)
	default:
		d.input(L("Nome", "Name"), &c.Name, 36)
		d.choose(L("Autenticação", "Authentication"), authList(), authKeys, &c.Auth, true)
		if c.Auth == ora.AuthPassword {
			d.input(L("Usuário", "User"), &c.User, 30)
			d.secret(L("Senha", "Password"), &c.Password, 30)
			d.check(L("Salvar senhas (cifradas)", "Save passwords (encrypted)"), &c.SavePwd)
		}
		roles := rolesList()
		rk := append([]string{""}, roles[1:]...)
		d.choose(L("Papel", "Role"), roles, rk, &c.Role, false)
		d.choose(L("Tipo de conexão", "Connection type"), typesList(), typeKeys, &c.Type, true)
		switch c.Type {
		case ora.TypeTNS:
			in := d.input(L("Alias ou descritor", "Alias or descriptor"), &c.TNS, 0)
			complete(in, func() []string { return ora.AliasNames(ora.TNSAliasesIn(c.TNSAdmin)) })
			n := len(ora.TNSAliasesIn(c.TNSAdmin))
			f.AddTextView("", fmt.Sprintf(L("[gray]%d alias em %s (pasta em Avançado)[-]", "[gray]%d alias(es) in %s (folder under Advanced)[-]"),
				n, tview.Escape(strings.Join(ora.TNSFilesIn(c.TNSAdmin), ", "))), 0, 4, true, false)
		case ora.TypeCloud:
			d.input(L("Arquivo do wallet (.zip)", "Wallet file (.zip)"), &c.CloudZip, 0)
			in := d.input(L("Serviço", "Service"), &c.TNS, 36)
			complete(in, func() []string {
				s, _ := ora.CloudServices(c.CloudZip)
				return s
			})
			d.secret(L("Senha do wallet", "Wallet password"), &c.WalletPwd, 30)
		case ora.TypeLDAP:
			d.input(L("Servidor LDAP (host:porta)", "LDAP server (host:port)"), &c.LDAPServer, 40)
			d.input(L("Contexto (ex.: dc=empresa,dc=com)", "Context (e.g. dc=corp,dc=com)"), &c.LDAPContext, 40)
			in := d.input(L("Serviço", "Service"), &c.TNS, 36)
			complete(in, func() []string { return d.ldapServices() })
		case ora.TypeCustom:
			d.input("URL", &c.CustomURL, 0)
			f.AddTextView("", L("[gray]jdbc:oracle:thin:@//host:porta/serviço · @host:porta:SID · @alias · @(DESCRIPTION=...) · oracle://usuário:senha@host:porta/serviço[-]",
				"[gray]jdbc:oracle:thin:@//host:port/service · @host:port:SID · @alias · @(DESCRIPTION=...) · oracle://user:pass@host:port/service[-]"), 0, 4, true, false)
		default:
			d.input("Host", &c.Host, 36)
			d.number(L("Porta", "Port"), &c.Port, 1521)
			d.input("Service name", &c.Service, 30)
			d.input("SID", &c.SID, 16)
		}
	}
	d.buttons()
	solid(f)
	d.holder.Clear().AddItem(f, 0, 1, true)
}

// ldapServices busca (uma vez por servidor/contexto) os serviços do LDAP.
func (d *connDlg) ldapServices() []string {
	k := d.cur.LDAPServer + "|" + d.cur.LDAPContext
	if d.svcCache == nil {
		d.svcCache = map[string][]string{}
	}
	if s, ok := d.svcCache[k]; ok {
		return s
	}
	d.svcCache[k] = nil
	srv, ctx := d.cur.LDAPServer, d.cur.LDAPContext
	if srv == "" || ctx == "" {
		return nil
	}
	d.a.Go(func() func() {
		s, err := ora.LDAPServices(srv, ctx)
		return func() {
			if err != nil {
				d.setStatus("[red]" + tview.Escape(err.Error()) + "[-]")
				return
			}
			d.svcCache[k] = s
			d.setStatus(fmt.Sprintf(L("%d serviço(s) no LDAP", "%d service(s) in LDAP"), len(s)))
		}
	})
	return nil
}

func (d *connDlg) read() (ora.ConnInfo, error) {
	n := d.cur
	if n.Type == ora.TypeBasic {
		n.Type = "" // conexões básicas ficam como antes no arquivo
	}
	// limpa o que é de outro tipo
	switch n.Kind() {
	case ora.TypeBasic:
		n.TNS, n.CloudZip, n.LDAPServer, n.LDAPContext, n.CustomURL = "", "", "", "", ""
	case ora.TypeTNS, ora.TypeCloud, ora.TypeLDAP, ora.TypeCustom:
		n.Host, n.Service, n.SID = "", "", ""
		if n.Kind() != ora.TypeCloud {
			n.CloudZip = ""
		}
		if n.Kind() != ora.TypeLDAP {
			n.LDAPServer, n.LDAPContext = "", ""
		}
		if n.Kind() != ora.TypeCustom {
			n.CustomURL = ""
		} else {
			n.TNS = ""
		}
	}
	if n.Auth != ora.AuthPassword {
		n.User, n.Password = "", ""
	}
	if n.SSHHost == "" {
		n.SSHPort, n.SSHUser, n.SSHKey, n.SSHPwd = 0, "", "", ""
	}
	if n.Name == "" {
		n.Name = n.Display()
	}
	return n, n.Check()
}

func (d *connDlg) save() (ora.ConnInfo, bool) {
	a := d.a
	n, err := d.read()
	if err != nil {
		d.setStatus("[red]" + tview.Escape(err.Error()) + "[-]")
		return n, false
	}
	if n.Name != d.oldName && a.connExists(n.Name) {
		d.setStatus("[red]" + tview.Escape(fmt.Sprintf(L("já existe conexão com o nome %q", "a connection named %q already exists"), n.Name)) + "[-]")
		return n, false
	}
	if d.oldName != "" && a.connExists(d.oldName) {
		if a.Connected(d.oldName) != nil && d.oldName != n.Name {
			a.disconnectTree(d.oldName)
		}
		a.removeConn(d.oldName)
	}
	a.conns = append(a.conns, n)
	if err := store.SaveConns(a.conns); err != nil {
		d.setStatus("[red]" + tview.Escape(err.Error()) + "[-]")
		return n, false
	}
	d.oldName = n.Name
	a.buildConnTree()
	a.buildDBATree()
	return n, true
}

func (d *connDlg) buttons() {
	a, f := d.a, d.f
	f.AddButton(L("Testar", "Test"), func() {
		n, err := d.read()
		if err != nil {
			d.setStatus("[red]" + tview.Escape(err.Error()) + "[-]")
			return
		}
		d.setStatus("[yellow]" + L("testando…", "testing…") + "[-]")
		a.Go(func() func() {
			db, err := ora.Open(n)
			if err != nil {
				return func() { d.setStatus("[red]" + tview.Escape(L("Falhou: ", "Failed: ")+err.Error()) + "[-]") }
			}
			var banner string
			_ = db.QueryRow("select banner from v$version where rownum = 1").Scan(&banner)
			ora.Close(db)
			return func() { d.setStatus("[green]" + L("Sucesso", "Success") + "[-]  " + tview.Escape(banner)) }
		})
	})
	f.AddButton(L("Salvar", "Save"), func() {
		if n, ok := d.save(); ok {
			d.setStatus("[green]" + L("Salva: ", "Saved: ") + tview.Escape(n.Name) + "[-]")
			refill(d.list, a, "", n.Name)
			a.tv.SetFocus(d.f) // refill recria o formulário
		}
	})
	f.AddButton(L("Conectar", "Connect"), func() {
		n, ok := d.save()
		if !ok {
			return
		}
		d.closeDlg()
		a.connectAndOpen(n.Name)
	})
	f.AddButton(L("Limpar", "Clear"), func() { d.load(ora.ConnInfo{}, ""); a.tv.SetFocus(d.f) })
	f.AddButton(L("Remover", "Delete"), func() {
		if d.oldName == "" || !a.connExists(d.oldName) {
			return
		}
		nm := d.oldName
		a.Confirm(fmt.Sprintf(L("Remover a conexão %q (e a senha salva)?", "Delete connection %q (and its saved password)?"), nm), func() {
			a.deleteConn(nm)
			refill(d.list, a, "", "")
			d.load(ora.ConnInfo{}, "")
			d.setStatus(L("removida: ", "deleted: ") + tview.Escape(nm))
			a.tv.SetFocus(d.f)
		})
	})
	f.AddButton(L("Cancelar", "Cancel"), d.closeDlg)
	f.SetCancelFunc(d.closeDlg)
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
