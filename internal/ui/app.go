// Aplicação: título com o ícone verde de banco, árvore à esquerda (Conexões e
// DBA), abas à direita (worksheets, telas DBA, objetos) e a linha de atalhos.
// by • DBA BRABO
package ui

import (
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/acaciolr/ora-brabo-studio/internal/i18n"
	"github.com/acaciolr/ora-brabo-studio/internal/ora"
	"github.com/acaciolr/ora-brabo-studio/internal/store"
)

var regexpTags = regexp.MustCompile(`\[[a-zA-Z#0-9:,\-]*\]`)

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
	menuBar *tview.TextView
	status  *tview.TextView
	left    *tview.Flex
	connT   *tview.TreeView
	dbaT    *tview.TreeView
	tabs    *Tabs
	body    *tview.Flex
	leftOn  string // árvore com foco por último: "conn" ou "dba"
	leftW   int    // largura do painel esquerdo (0 = escondido)
	connH   int    // peso da árvore Conexões no painel esquerdo (DBA = 10 - connH)

	menus    []*topMenu
	menuOpen int
	pop      *popup
	popFocus tview.Primitive
	ctx      map[tview.Primitive]ctxProvider
	noSelect bool // clique simples na árvore: só seleciona
	msgs     strings.Builder
	clip     string

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

	a.ctx = map[tview.Primitive]ctxProvider{}
	a.menuBar = tview.NewTextView().SetDynamicColors(true).SetRegions(true).SetWrap(false)
	a.menuBar.SetBackgroundColor(tcell.NewRGBColor(24, 38, 46))
	a.status = tview.NewTextView().SetDynamicColors(true).SetWrap(false)
	a.status.SetBackgroundColor(tcell.NewRGBColor(24, 38, 46))
	a.connT = tview.NewTreeView()
	a.dbaT = tview.NewTreeView()
	a.paneTitle(a.connT, Icon()+" "+L("Conexões", "Connections"))
	a.paneTitle(a.dbaT, Icon()+" DBA")
	a.leftOn, a.leftW, a.connH = "conn", 38, 5
	a.left = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.connT, 0, a.connH, true).
		AddItem(a.dbaT, 0, 10-a.connH, false)
	a.tabs = NewTabs(a)
	a.body = tview.NewFlex().
		AddItem(a.left, a.leftW, 0, true).
		AddItem(a.tabs, 0, 1, false)
	main := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(a.menuBar, 1, 0, false).
		AddItem(a.body, 0, 1, true).
		AddItem(a.status, 1, 0, false)
	a.root = tview.NewPages().AddPage("main", main, true, true)
	a.tv.SetRoot(a.root, true).SetFocus(a.connT)
	a.tv.SetInputCapture(a.keys)
	a.tv.SetMouseCapture(a.mouse)
	a.buildMenus()
	a.mouseTree(a.connT)
	a.mouseTree(a.dbaT)
	a.status.SetHighlightedFunc(func(added, _, _ []string) {
		if len(added) > 0 {
			a.status.Highlight()
			a.showMessages()
		}
	})

	cs, err := store.LoadConns()
	if err != nil {
		a.Msg("[red]" + L("Erro lendo conexões: ", "Error reading connections: ") + err.Error())
	}
	a.conns = cs
	a.buildConnTree()
	a.buildDBATree()
	a.drawHeader()
	a.hint()
	// como no goSSMS: o diálogo de conexão abre na entrada
	a.ConnectDialog("")
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
				}
				if a.msgTime.IsZero() {
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

// paneTitle dá título ao painel e o destaca quando tem foco (como no goSSMS).
func (a *App) paneTitle(t *tview.TreeView, title string) {
	t.SetBorder(true).SetTitle(" " + title + " ").SetTitleAlign(tview.AlignLeft)
	t.SetGraphicsColor(tcell.NewRGBColor(70, 90, 100))
	// seleção: a cor do nó vira o fundo e o texto fica escuro (legível em qualquer nó)
	t.SetDrawFunc(func(_ tcell.Screen, x, y, w, h int) (int, int, int, int) {
		if r := t.GetRoot(); r != nil {
			r.Walk(func(n, _ *tview.TreeNode) bool {
				n.SetSelectedTextStyle(tcell.StyleDefault.Background(n.GetColor()).Foreground(colModal).Bold(true))
				return true
			})
		}
		return x + 1, y + 1, w - 2, h - 2
	})
	t.SetFocusFunc(func() {
		t.SetBorderColor(tcell.NewRGBColor(46, 189, 89)).SetTitleColor(tcell.NewRGBColor(46, 189, 89))
		if t == a.connT {
			a.leftOn = "conn"
		} else {
			a.leftOn = "dba"
		}
		a.hint()
	})
	t.SetBlurFunc(func() {
		t.SetBorderColor(tview.Styles.BorderColor).SetTitleColor(tview.Styles.TitleColor)
	})
}

// drawHeader atualiza o lado direito da barra de status (conexão e hora).
func (a *App) drawHeader() { a.hint() }

func (a *App) right() string {
	cur := ""
	if t := a.tabs.Current(); t != nil {
		if c := t.Conn(); c != nil {
			cor := "[#9fd3c7]"
			if c.Info.Color != "" {
				cor = "[" + c.Info.Color + "]"
			}
			cur = cor + "⛁ " + tview.Escape(c.Info.Name) + "[-] [gray]" + tview.Escape(c.User+" · "+c.ConName+" · "+c.Version) + "[-]  "
		}
	}
	return cur + "[gray]" + time.Now().Format("15:04") + "[-] "
}

func (a *App) showLeft(name string) { a.focusTree(name) }

// focusTree põe o foco numa das árvores da esquerda.
func (a *App) focusTree(name string) {
	if a.leftW == 0 {
		a.toggleLeft()
	}
	if name == "dba" {
		a.tv.SetFocus(a.dbaT)
	} else {
		a.tv.SetFocus(a.connT)
	}
}

func (a *App) toggleLeft() {
	if a.leftW == 0 {
		a.leftW = 38
	} else {
		a.leftW = 0
	}
	a.body.ResizeItem(a.left, a.leftW, 0)
}

func (a *App) resizeLeft(dw, dh int) {
	if dw != 0 && a.leftW > 0 {
		a.leftW = min(max(a.leftW+dw, 20), 90)
		a.body.ResizeItem(a.left, a.leftW, 0)
	}
	if dh != 0 {
		a.connH = min(max(a.connH+dh, 1), 9)
		a.left.ResizeItem(a.connT, 0, a.connH).ResizeItem(a.dbaT, 0, 10-a.connH)
	}
}

// Msg mostra uma mensagem na linha de baixo por alguns segundos.
func (a *App) Msg(s string) {
	a.msgTime = time.Now()
	fmt.Fprintf(&a.msgs, "%s  %s\n", a.msgTime.Format("15:04:05"), s)
	a.setStatus(s)
}

func (a *App) setStatus(s string) {
	_, _, w, _ := a.status.GetRect()
	r := a.right()
	pad := w - tview.TaggedStringWidth(s) - tview.TaggedStringWidth(r) - 2
	if pad < 1 {
		pad = 1
	}
	a.status.SetText(`[""] ` + s + strings.Repeat(" ", pad) + r)
}

// showMessages mostra o histórico de mensagens (clique na barra de status).
func (a *App) showMessages() {
	t := a.msgs.String()
	if t == "" {
		t = L("(sem mensagens)", "(no messages)")
	}
	a.TextBox(L("Mensagens", "Messages"), stripTags(t))
}

func stripTags(s string) string { return regexpTags.ReplaceAllString(s, "") }

// Errf mostra um erro.
func (a *App) Errf(format string, args ...any) {
	a.Msg("[red::b]" + tview.Escape(fmt.Sprintf(format, args...)) + "[-::-]")
}

func (a *App) hint() {
	if !a.msgTime.IsZero() && time.Since(a.msgTime) < 8*time.Second {
		return
	}
	h := L("Pronto  [gray]│[-]  [yellow]F1[-] Ajuda  [gray]│[-]  [yellow]F10[-] Menu  [gray]│[-]  [yellow]Ctrl+N[-] Worksheet  [gray]│[-]  [yellow]F9[-] Conectar  [gray]│[-]  [yellow]Ctrl+Q[-] Sair",
		"Ready  [gray]│[-]  [yellow]F1[-] Help  [gray]│[-]  [yellow]F10[-] Menu  [gray]│[-]  [yellow]Ctrl+N[-] Worksheet  [gray]│[-]  [yellow]F9[-] Connect  [gray]│[-]  [yellow]Ctrl+Q[-] Quit")
	f := a.tv.GetFocus()
	if f != a.connT && f != a.dbaT {
		if t := a.tabs.Current(); t != nil {
			if s := t.Hint(); s != "" {
				h = s
			}
		}
	} else {
		h = L("[yellow]Enter[-]/duplo clique abre  [gray]│[-]  [yellow]clique direito[-]/Shift+F10 menu  [gray]│[-]  [yellow]F5[-] atualiza  [gray]│[-]  [yellow]Ctrl+←→↑↓[-] tamanho  [gray]│[-]  [yellow]F10[-] Menu",
			"[yellow]Enter[-]/double-click opens  [gray]│[-]  [yellow]right-click[-]/Shift+F10 menu  [gray]│[-]  [yellow]F5[-] refresh  [gray]│[-]  [yellow]Ctrl+←→↑↓[-] size  [gray]│[-]  [yellow]F10[-] Menu")
	}
	a.setStatus(h)
}

// keys trata os atalhos globais.
func (a *App) keys(ev *tcell.EventKey) *tcell.EventKey {
	// Ctrl+C copia (o padrão do tview seria fechar o programa)
	if ev.Key() == tcell.KeyCtrlC {
		a.copyFocused()
		return nil
	}
	if a.modals > 0 {
		return ev
	}
	f := a.tv.GetFocus()
	inTree := f == a.connT || f == a.dbaT
	ctrl := ev.Modifiers()&tcell.ModCtrl != 0
	switch ev.Key() {
	case tcell.KeyF1:
		a.openHelp()
		return nil
	case tcell.KeyF10:
		if ev.Modifiers()&tcell.ModShift != 0 {
			a.ContextMenu(f, -1, -1)
			return nil
		}
		if a.curWorksheet() != nil && !inTree && a.curWorksheet().ed.HasFocus() {
			return ev // F10 na worksheet = plano (como no SQL Developer)
		}
		a.openMenu(0)
		return nil
	case tcell.KeyF2:
		if a.leftOn == "conn" && inTree {
			a.focusTree("dba")
		} else {
			a.focusTree("conn")
		}
		return nil
	case tcell.KeyF3:
		a.cycleFocus()
		return nil
	case tcell.KeyF9:
		if a.curWorksheet() == nil || inTree {
			a.ConnectDialog(a.currentConnName())
			return nil
		}
	case tcell.KeyCtrlN:
		a.newWorksheet(a.currentConnName())
		return nil
	case tcell.KeyCtrlW:
		a.tabs.CloseCurrent()
		return nil
	case tcell.KeyCtrlQ:
		a.quit()
		return nil
	case tcell.KeyCtrlSpace:
		if inTree {
			a.ContextMenu(f, -1, -1)
			return nil
		}
	case tcell.KeyPgDn:
		if ctrl {
			a.tabs.Next(1)
			return nil
		}
	case tcell.KeyPgUp:
		if ctrl {
			a.tabs.Next(-1)
			return nil
		}
	case tcell.KeyRight, tcell.KeyLeft, tcell.KeyUp, tcell.KeyDown:
		d := map[tcell.Key]int{tcell.KeyRight: 1, tcell.KeyLeft: -1, tcell.KeyUp: -1, tcell.KeyDown: 1}[ev.Key()]
		if ev.Modifiers()&tcell.ModAlt != 0 && (ev.Key() == tcell.KeyRight || ev.Key() == tcell.KeyLeft) {
			a.tabs.Next(d)
			return nil
		}
		if ctrl && inTree {
			if ev.Key() == tcell.KeyRight || ev.Key() == tcell.KeyLeft {
				a.resizeLeft(2*d, 0)
			} else {
				a.resizeLeft(0, d)
			}
			return nil
		}
	case tcell.KeyRune:
		if ev.Modifiers()&tcell.ModAlt != 0 {
			switch r := ev.Rune(); {
			case r >= '1' && r <= '9':
				a.tabs.Select(int(r - '1'))
				return nil
			case r == 'o' || r == 'O':
				if c := a.Connected(a.currentConnName()); c != nil {
					a.findObject(c)
				}
				return nil
			}
		}
	}
	return ev
}

// copyFocused faz o Ctrl+C do primitivo com foco.
func (a *App) copyFocused() {
	f := a.tv.GetFocus()
	set := func(p tview.Primitive) { a.tv.SetFocus(p) }
	switch x := f.(type) {
	case *tview.TextArea:
		x.InputHandler()(tcell.NewEventKey(tcell.KeyCtrlQ, 0, tcell.ModNone), set)
	case *tview.InputField:
		Copy(x.GetText())
	default:
		if f != nil {
			f.InputHandler()(tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModNone), set)
		}
	}
}

// mouse: clique fora de um menu aberto fecha o menu.
func (a *App) mouse(ev *tcell.EventMouse, action tview.MouseAction) (*tcell.EventMouse, tview.MouseAction) {
	if a.pop != nil && (action == tview.MouseLeftDown || action == tview.MouseRightDown) {
		x, y := ev.Position()
		p := a.pop
		if x < p.x || x >= p.x+p.w || y < p.y || y >= p.y+p.h {
			_, my, _, _ := a.menuBar.GetRect()
			a.closePopup()
			if y == my && action == tview.MouseLeftDown {
				return ev, action // clicou em outro menu da barra
			}
			return nil, tview.MouseConsumed
		}
	}
	return ev, action
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
	a.focusTree(a.leftOn)
}

func (a *App) openHelp() {
	if i := a.tabs.Find(L("Ajuda", "Help"), nil); i >= 0 {
		a.tabs.Select(i)
		return
	}
	a.tabs.Add(NewHelp(a))
}

func (a *App) closeAllTabs() {
	for _, t := range a.tabs.All() {
		if w, ok := t.(*Worksheet); ok && w.sess != nil && w.sess.Pending {
			a.tabs.Select(a.tabs.Find(w.Title(), w.c))
			a.tabs.CloseCurrent()
			return
		}
		a.tabs.Remove(t)
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

// colModal é o fundo sólido dos diálogos (com o fundo "padrão" do terminal o
// conteúdo de baixo apareceria através deles).
var colModal = tcell.NewRGBColor(18, 30, 38)

func solid(p tview.Primitive) {
	switch x := p.(type) {
	case *tview.Modal:
		x.SetBackgroundColor(colModal)
	case interface {
		SetBackgroundColor(tcell.Color) *tview.Box
	}:
		x.SetBackgroundColor(colModal)
	}
	if f, ok := p.(*tview.Form); ok {
		f.SetFieldBackgroundColor(tcell.NewRGBColor(40, 60, 70)).SetButtonBackgroundColor(tcell.NewRGBColor(40, 60, 70))
	}
}

// Modal mostra um primitivo por cima de tudo.
func (a *App) Modal(name string, p tview.Primitive, w, h int) {
	solid(p)
	grid := tview.NewGrid().SetColumns(0, w, 0).SetRows(0, h, 0).AddItem(p, 1, 1, 1, 1, 0, 0, true)
	// o tamanho cabe na tela de agora (na entrada a tela ainda não tem tamanho)
	grid.SetDrawFunc(func(_ tcell.Screen, x, y, sw, sh int) (int, int, int, int) {
		grid.SetColumns(0, min(w, max(sw-2, 10)), 0).SetRows(0, min(h, max(sh-2, 5)), 0)
		return x, y, sw, sh
	})
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
			a.focusTree(a.leftOn)
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
	solid(m)
	a.root.AddPage("confirm", m, true, true)
	a.modals++
	a.tv.SetFocus(m)
}

// Alert mostra um aviso.
func (a *App) Alert(text string) {
	m := tview.NewModal().SetText(text).AddButtons([]string{"OK"}).
		SetDoneFunc(func(int, string) { a.CloseModal("alert") })
	solid(m)
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

var clipboard string

// Copy manda o texto para a área de transferência do terminal (OSC 52).
func Copy(s string) {
	clipboard = s
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
