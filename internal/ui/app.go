// Aplicação: título com o ícone verde de banco, árvore à esquerda (Conexões e
// DBA), abas à direita (worksheets, telas DBA, objetos) e a linha de atalhos.
// by • DBA BRABO
package ui

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/acaciolr/ora-brabo-studio/internal/i18n"
	"github.com/acaciolr/ora-brabo-studio/internal/ora"
	"github.com/acaciolr/ora-brabo-studio/internal/store"
)

// Version é a versão do binário (o main troca no build).
var Version = "dev"

// L escolhe o texto do idioma atual.
func L(pt, en string) string { return i18n.L(pt, en) }

// Conn é uma conexão aberta.
type Conn struct {
	Info    ora.ConnInfo
	DB      *sql.DB
	User    string
	Version string
	CDB     bool
	ConName string
	Packs   string // control_management_pack_access
	packOK  map[string]bool
	mu      sync.Mutex
	names   []string // cache para completar (tabelas/views do usuário)
}

// App é a interface inteira.
type App struct {
	tv      *tview.Application
	root    *tview.Pages
	header  *tview.TextView
	status  *tview.TextView
	left    *tview.Pages
	leftBar *tview.TextView
	connT   *tview.TreeView
	dbaT    *tview.TreeView
	tabs    *Tabs
	body    *tview.Flex
	leftOn  string // "conn" ou "dba"

	conns   []ora.ConnInfo
	open    map[string]*Conn
	openMu  sync.Mutex
	msgTime time.Time
	modals  int
}

// New monta a aplicação (screen nil = terminal de verdade).
func New(screen tcell.Screen) *App {
	a := &App{tv: tview.NewApplication(), open: map[string]*Conn{}}
	if screen != nil {
		a.tv.SetScreen(screen)
	}
	a.tv.EnableMouse(true)
	a.tv.EnablePaste(true)
	tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
	tview.Styles.ContrastBackgroundColor = tcell.NewRGBColor(40, 60, 70)
	tview.Styles.MoreContrastBackgroundColor = tcell.NewRGBColor(30, 45, 55)
	tview.Styles.BorderColor = tcell.NewRGBColor(90, 110, 120)
	tview.Styles.TitleColor = tcell.ColorWhite
	tview.Styles.GraphicsColor = tcell.NewRGBColor(90, 110, 120)

	a.header = tview.NewTextView().SetDynamicColors(true)
	a.status = tview.NewTextView().SetDynamicColors(true)
	a.leftBar = tview.NewTextView().SetDynamicColors(true).SetRegions(true)
	a.connT = tview.NewTreeView()
	a.dbaT = tview.NewTreeView()
	a.left = tview.NewPages()
	a.left.AddPage("conn", a.connT, true, true)
	a.left.AddPage("dba", a.dbaT, true, false)
	a.leftOn = "conn"
	a.leftBar.SetHighlightedFunc(func(added, _, _ []string) {
		if len(added) > 0 {
			a.showLeft(added[0])
		}
	})
	leftBox := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.leftBar, 1, 0, false).
		AddItem(a.left, 0, 1, true)
	leftBox.SetBorder(true).SetBorderPadding(0, 0, 1, 0)
	a.tabs = NewTabs(a)

	a.body = tview.NewFlex().
		AddItem(leftBox, 38, 0, true).
		AddItem(a.tabs, 0, 1, false)
	main := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.header, 1, 0, false).
		AddItem(a.body, 0, 1, true).
		AddItem(a.status, 1, 0, false)
	a.root = tview.NewPages().AddPage("main", main, true, true)
	a.tv.SetRoot(a.root, true).SetFocus(a.connT)
	a.tv.SetInputCapture(a.keys)

	cs, err := store.LoadConns()
	if err != nil {
		a.Msg("[red]" + L("Erro lendo conexões: ", "Error reading connections: ") + err.Error())
	}
	a.conns = cs
	a.buildConnTree()
	a.buildDBATree()
	a.drawLeftBar()
	a.drawHeader()
	a.hint()
	if len(cs) == 0 {
		a.tabs.Add(NewWelcome(a))
	}
	return a
}

// Run roda a interface até sair.
func (a *App) Run() error {
	go func() {
		t := time.NewTicker(time.Second)
		for range t.C {
			a.tv.QueueUpdateDraw(func() {
				a.drawHeader()
				if !a.msgTime.IsZero() && time.Since(a.msgTime) > 8*time.Second {
					a.msgTime = time.Time{}
					a.hint()
				}
			})
		}
	}()
	return a.tv.Run()
}

// Stop encerra.
func (a *App) Stop() { a.tv.Stop() }

// Icon é o ícone de banco de dados do canto superior esquerdo (verde, como no
// SQL Developer). ORA_BRABO_ASCII=1 troca por ASCII em terminais sem o glifo.
func Icon() string {
	if os.Getenv("ORA_BRABO_ASCII") == "1" {
		return "[#2EBD59::b](=)[-::-]"
	}
	return "[#2EBD59::b]⛁[-::-]"
}

func (a *App) drawHeader() {
	cur := ""
	if t := a.tabs.Current(); t != nil {
		if c := t.Conn(); c != nil {
			cor := "[#9fd3c7]"
			if c.Info.Color != "" {
				cor = "[" + c.Info.Color + "]"
			}
			cur = "  " + cor + tview.Escape(c.Info.Name) + "[-] [gray]" + tview.Escape(c.User+" · "+c.ConName+" · "+c.Version) + "[-]"
		}
	}
	a.header.SetText(fmt.Sprintf(" %s [::b]ORA BRABO[::-] [#9fd3c7]Studio[-]%s[gray]%s %s[-]",
		Icon(), cur, strings.Repeat(" ", 2), time.Now().Format("15:04:05")))
}

func (a *App) drawLeftBar() {
	c, d := "", ""
	if a.leftOn == "conn" {
		c = "[black:#9fd3c7]"
	} else {
		d = "[black:#9fd3c7]"
	}
	a.leftBar.SetText(fmt.Sprintf(`["conn"]%s %s [-:-:-][""] ["dba"]%s DBA [-:-:-][""]`, c, L("Conexões", "Connections"), d))
}

func (a *App) showLeft(name string) {
	a.leftOn = name
	a.left.SwitchToPage(name)
	a.drawLeftBar()
	if name == "dba" {
		a.tv.SetFocus(a.dbaT)
	} else {
		a.tv.SetFocus(a.connT)
	}
}

// Msg mostra uma mensagem na linha de baixo por alguns segundos.
func (a *App) Msg(s string) {
	a.status.SetText(" " + s)
	a.msgTime = time.Now()
}

// Errf mostra um erro.
func (a *App) Errf(format string, args ...any) {
	a.Msg("[red::b]" + tview.Escape(fmt.Sprintf(format, args...)) + "[-::-]")
}

func (a *App) hint() {
	h := L("[yellow]F1[-] ajuda  [yellow]F2[-] Conexões/DBA  [yellow]F3[-] foco  [yellow]Ctrl+N[-] worksheet  [yellow]F5[-] script  [yellow]F9[-] executar  [yellow]F10[-] plano  [yellow]F6[-] autotrace  [yellow]F11[-] commit  [yellow]F12[-] rollback  [yellow]Ctrl+W[-] fecha aba  [yellow]Ctrl+Q[-] sair",
		"[yellow]F1[-] help  [yellow]F2[-] Connections/DBA  [yellow]F3[-] focus  [yellow]Ctrl+N[-] worksheet  [yellow]F5[-] script  [yellow]F9[-] run  [yellow]F10[-] plan  [yellow]F6[-] autotrace  [yellow]F11[-] commit  [yellow]F12[-] rollback  [yellow]Ctrl+W[-] close tab  [yellow]Ctrl+Q[-] quit")
	if t := a.tabs.Current(); t != nil {
		if s := t.Hint(); s != "" {
			h = s
		}
	}
	a.status.SetText(" " + h)
}

// keys trata os atalhos globais.
func (a *App) keys(ev *tcell.EventKey) *tcell.EventKey {
	if a.modals > 0 {
		return ev
	}
	switch ev.Key() {
	case tcell.KeyF1:
		if i := a.tabs.Find(L("Ajuda", "Help"), nil); i >= 0 {
			a.tabs.Select(i)
		} else {
			a.tabs.Add(NewHelp(a))
		}
		return nil
	case tcell.KeyF2:
		if a.leftOn == "conn" {
			a.showLeft("dba")
		} else {
			a.showLeft("conn")
		}
		return nil
	case tcell.KeyF3:
		a.cycleFocus()
		return nil
	case tcell.KeyCtrlN:
		a.newWorksheet(a.currentConnName())
		return nil
	case tcell.KeyCtrlW:
		a.tabs.CloseCurrent()
		return nil
	case tcell.KeyCtrlQ:
		a.quit()
		return nil
	case tcell.KeyPgDn:
		if ev.Modifiers()&tcell.ModCtrl != 0 {
			a.tabs.Next(1)
			return nil
		}
	case tcell.KeyPgUp:
		if ev.Modifiers()&tcell.ModCtrl != 0 {
			a.tabs.Next(-1)
			return nil
		}
	case tcell.KeyRight:
		if ev.Modifiers()&tcell.ModAlt != 0 {
			a.tabs.Next(1)
			return nil
		}
	case tcell.KeyLeft:
		if ev.Modifiers()&tcell.ModAlt != 0 {
			a.tabs.Next(-1)
			return nil
		}
	case tcell.KeyRune:
		if ev.Modifiers()&tcell.ModAlt != 0 && ev.Rune() >= '1' && ev.Rune() <= '9' {
			a.tabs.Select(int(ev.Rune() - '1'))
			return nil
		}
	}
	return ev
}

func (a *App) cycleFocus() {
	f := a.tv.GetFocus()
	if f == a.connT || f == a.dbaT {
		if t := a.tabs.Current(); t != nil {
			t.FocusPart(0)
			return
		}
	}
	if t := a.tabs.Current(); t != nil && t.FocusPart(1) {
		return
	}
	if a.leftOn == "dba" {
		a.tv.SetFocus(a.dbaT)
	} else {
		a.tv.SetFocus(a.connT)
	}
}

func (a *App) quit() {
	pend := []string{}
	for _, t := range a.tabs.All() {
		if w, ok := t.(*Worksheet); ok && w.sess != nil && w.sess.Pending {
			pend = append(pend, w.Title())
		}
	}
	if len(pend) == 0 {
		a.Confirm(L("Sair do ORA BRABO Studio?", "Quit ORA BRABO Studio?"), func() { a.closeAll(); a.Stop() })
		return
	}
	a.Confirm(fmt.Sprintf(L("Há alterações sem commit em: %s.\nSair faz ROLLBACK delas. Sair mesmo?", "Uncommitted changes in: %s.\nQuitting rolls them back. Quit anyway?"), strings.Join(pend, ", ")),
		func() { a.closeAll(); a.Stop() })
}

func (a *App) closeAll() {
	for _, t := range a.tabs.All() {
		t.Close()
	}
	a.openMu.Lock()
	for _, c := range a.open {
		c.DB.Close()
	}
	a.openMu.Unlock()
}

// Go roda f em segundo plano e devolve o resultado na thread da interface.
func (a *App) Go(f func() func()) {
	go func() {
		done := f()
		if done != nil {
			a.tv.QueueUpdateDraw(done)
		}
	}()
}

// ---- diálogos ----

// Modal mostra um primitivo por cima de tudo.
func (a *App) Modal(name string, p tview.Primitive, w, h int) {
	grid := tview.NewGrid().SetColumns(0, w, 0).SetRows(0, h, 0).AddItem(p, 1, 1, 1, 1, 0, 0, true)
	a.root.AddPage(name, grid, true, true)
	a.modals++
	a.tv.SetFocus(p)
}

// CloseModal fecha o diálogo e volta o foco.
func (a *App) CloseModal(name string) {
	if a.root.HasPage(name) {
		a.root.RemovePage(name)
		a.modals--
	}
	if a.modals == 0 {
		if t := a.tabs.Current(); t != nil {
			t.FocusPart(0)
		} else {
			a.showLeft(a.leftOn)
		}
	}
}

// Confirm pergunta sim/não.
func (a *App) Confirm(text string, yes func()) {
	m := tview.NewModal().SetText(text).AddButtons([]string{L("Sim", "Yes"), L("Não", "No")}).
		SetDoneFunc(func(i int, _ string) {
			a.CloseModal("confirm")
			if i == 0 {
				yes()
			}
		})
	a.root.AddPage("confirm", m, true, true)
	a.modals++
	a.tv.SetFocus(m)
}

// Alert mostra um aviso.
func (a *App) Alert(text string) {
	m := tview.NewModal().SetText(text).AddButtons([]string{"OK"}).
		SetDoneFunc(func(int, string) { a.CloseModal("alert") })
	a.root.AddPage("alert", m, true, true)
	a.modals++
	a.tv.SetFocus(m)
}

// TextBox mostra um texto longo rolável (valor de célula, DDL, erro).
func (a *App) TextBox(title, text string) {
	tv := tview.NewTextView().SetText(text).SetScrollable(true).SetWrap(true)
	tv.SetBorder(true).SetTitle(" " + title + L(" · Esc fecha · y copia ", " · Esc closes · y copies "))
	tv.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyEscape || ev.Key() == tcell.KeyEnter {
			a.CloseModal("textbox")
			return nil
		}
		if ev.Rune() == 'y' {
			Copy(text)
			a.Msg(L("Copiado para a área de transferência.", "Copied to clipboard."))
			return nil
		}
		return ev
	})
	a.Modal("textbox", tv, 100, 30)
}

// Copy manda o texto para a área de transferência do terminal (OSC 52).
func Copy(s string) {
	enc := b64(s)
	fmt.Fprintf(os.Stdout, "\x1b]52;c;%s\x07", enc)
}

// ---- conexões ----

func (a *App) findConn(name string) (ora.ConnInfo, bool) {
	for _, c := range a.conns {
		if c.Name == name {
			return c, true
		}
	}
	return ora.ConnInfo{}, false
}

// Connected devolve a conexão aberta.
func (a *App) Connected(name string) *Conn {
	a.openMu.Lock()
	defer a.openMu.Unlock()
	return a.open[name]
}

// Connect abre a conexão (pede a senha se não estiver salva) e chama done.
func (a *App) Connect(name string, done func(*Conn)) {
	if c := a.Connected(name); c != nil {
		done(c)
		return
	}
	info, ok := a.findConn(name)
	if !ok {
		a.Errf(L("conexão %s não existe", "connection %s does not exist"), name)
		return
	}
	if info.Password == "" {
		a.askPassword(info, func(pw string) {
			info.Password = pw
			a.doConnect(info, done)
		})
		return
	}
	a.doConnect(info, done)
}

func (a *App) doConnect(info ora.ConnInfo, done func(*Conn)) {
	a.Msg("[yellow]" + L("Conectando em ", "Connecting to ") + tview.Escape(info.Display()) + "…[-]")
	a.Go(func() func() {
		db, err := ora.Open(info)
		if err != nil {
			return func() { a.Errf("%s: %v", info.Name, err) }
		}
		c := &Conn{Info: info, DB: db, packOK: map[string]bool{}}
		row := db.QueryRow(`select user, (select version from v$instance),
		        sys_context('userenv', 'con_name'),
		        (select cdb from v$database),
		        (select value from v$parameter where name = 'control_management_pack_access') from dual`)
		var ver, cdb, packs sql.NullString
		if err := row.Scan(&c.User, &ver, &c.ConName, &cdb, &packs); err != nil {
			// sem acesso às V$: segue com o básico
			_ = db.QueryRow("select user, sys_context('userenv','con_name') from dual").Scan(&c.User, &c.ConName)
		}
		c.Version, c.CDB, c.Packs = ver.String, cdb.String == "YES", packs.String
		if c.Version == "" {
			c.Version = "?"
		}
		return func() {
			a.openMu.Lock()
			a.open[info.Name] = c
			a.openMu.Unlock()
			a.Msg("[green]" + L("Conectado: ", "Connected: ") + tview.Escape(info.Display()) + "[-]")
			a.drawHeader()
			done(c)
		}
	})
}

// Disconnect fecha a conexão e as abas dela.
func (a *App) Disconnect(name string) {
	c := a.Connected(name)
	if c == nil {
		return
	}
	for _, t := range a.tabs.All() {
		if t.Conn() == c {
			a.tabs.Remove(t)
		}
	}
	a.openMu.Lock()
	delete(a.open, name)
	a.openMu.Unlock()
	c.DB.Close()
	a.Msg(L("Desconectado de ", "Disconnected from ") + name)
}

func (a *App) currentConnName() string {
	if t := a.tabs.Current(); t != nil {
		if c := t.Conn(); c != nil {
			return c.Info.Name
		}
	}
	tree := a.connT
	if a.leftOn == "dba" {
		tree = a.dbaT
	}
	for n := tree.GetCurrentNode(); n != nil; {
		if r, ok := n.GetReference().(*nodeRef); ok && r.conn != "" {
			return r.conn
		}
		break
	}
	if len(a.open) == 1 {
		for k := range a.open {
			return k
		}
	}
	if len(a.conns) == 1 {
		return a.conns[0].Name
	}
	return ""
}

func (a *App) newWorksheet(conn string) {
	if conn == "" {
		a.Msg(L("Escolha uma conexão na árvore e tecle Ctrl+N (ou Enter em \"Nova worksheet\").", "Pick a connection in the tree and press Ctrl+N (or Enter on \"New worksheet\")."))
		a.showLeft("conn")
		return
	}
	a.Connect(conn, func(c *Conn) {
		w, err := NewWorksheet(a, c, "")
		if err != nil {
			a.Errf("%v", err)
			return
		}
		a.tabs.Add(w)
	})
}

// checkPack avisa sobre licença antes de usar AWR/ASH/ADDM/SQL Tuning.
func (a *App) checkPack(c *Conn, pack string, then func()) {
	if pack == "" || c.packOK[pack] {
		then()
		return
	}
	nome := map[string]string{"diag": "Diagnostics Pack", "tuning": "Tuning Pack"}[pack]
	p := strings.ToUpper(c.Packs)
	ok := (pack == "diag" && strings.Contains(p, "DIAGNOSTIC")) || (pack == "tuning" && strings.Contains(p, "TUNING"))
	msg := fmt.Sprintf(L("Isto usa o %s (licença à parte do Enterprise Edition).\ncontrol_management_pack_access = %s\n\nVocê tem a licença?",
		"This uses the %s (licensed separately from Enterprise Edition).\ncontrol_management_pack_access = %s\n\nDo you have the license?"), nome, c.Packs)
	if !ok {
		msg += L("\n\nAtenção: o banco está com o pack desligado; as views podem vir vazias.", "\n\nNote: the pack is disabled in this database; views may come back empty.")
	}
	a.Confirm(msg, func() {
		c.packOK[pack] = true
		then()
	})
}
