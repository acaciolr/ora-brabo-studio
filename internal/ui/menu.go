// Barra de menus (F10 ou clique), menu de contexto (clique direito, Shift+F10
// ou Ctrl+Space nas árvores e grades) e o clique / duplo clique de um app
// gráfico: clique seleciona, duplo clique abre. by • DBA BRABO
package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// menuItem é uma linha de menu.
type menuItem struct {
	label string
	key   string // atalho mostrado à direita
	fn    func()
	off   bool // desabilitado (cinza)
	sep   bool // separador
}

func sep() menuItem { return menuItem{sep: true} }

func item(label, key string, fn func()) menuItem { return menuItem{label: label, key: key, fn: fn} }

func itemIf(ok bool, label, key string, fn func()) menuItem {
	return menuItem{label: label, key: key, fn: fn, off: !ok}
}

// popup é a lista flutuante de um menu.
type popup struct {
	list  *tview.List
	items []menuItem
	x, y  int
	w, h  int
}

const popupPage = "popup"

// Popup abre um menu na posição (x, y). left/right (opcionais) trocam de menu
// na barra.
func (a *App) Popup(x, y int, items []menuItem, left, right func()) {
	a.closePopup()
	if len(items) == 0 {
		return
	}
	wl, wk := 0, 0
	for _, it := range items {
		wl = max(wl, utf8.RuneCountInString(it.label))
		wk = max(wk, utf8.RuneCountInString(it.key))
	}
	w := wl + wk + 6
	if wk == 0 {
		w = wl + 4
	}
	l := tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true).SetWrapAround(true)
	l.SetBorder(true).SetBackgroundColor(colModal)
	l.SetMainTextStyle(tcell.StyleDefault.Background(colModal).Foreground(tcell.ColorWhite))
	l.SetSelectedStyle(tcell.StyleDefault.Background(tcell.NewRGBColor(46, 189, 89)).Foreground(tcell.ColorBlack))
	for _, it := range items {
		if it.sep {
			l.AddItem("[gray]"+strings.Repeat("─", w-2)+"[-]", "", 0, nil)
			continue
		}
		txt := tview.Escape(it.label)
		pad := w - 4 - utf8.RuneCountInString(it.label) - utf8.RuneCountInString(it.key)
		if it.key != "" {
			txt += strings.Repeat(" ", max(pad, 1)) + "[gray]" + tview.Escape(it.key) + "[-]"
		}
		if it.off {
			txt = "[gray]" + tview.Escape(it.label) + "[-]"
		}
		l.AddItem(" "+txt, "", 0, nil)
	}
	run := func(i int) {
		if i < 0 || i >= len(items) || items[i].sep || items[i].off || items[i].fn == nil {
			return
		}
		a.closePopup()
		items[i].fn()
	}
	l.SetSelectedFunc(func(i int, _, _ string, _ rune) { run(i) })
	// pula separadores e itens desabilitados ao andar
	skip := func(dir int) {
		i := l.GetCurrentItem()
		for n := 0; n < len(items); n++ {
			i = (i + dir + len(items)) % len(items)
			if !items[i].sep && !items[i].off {
				l.SetCurrentItem(i)
				return
			}
		}
	}
	l.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Key() {
		case tcell.KeyEscape, tcell.KeyF10:
			a.closePopup()
			return nil
		case tcell.KeyUp:
			skip(-1)
			return nil
		case tcell.KeyDown, tcell.KeyTab:
			skip(1)
			return nil
		case tcell.KeyLeft:
			if left != nil {
				left()
			}
			return nil
		case tcell.KeyRight:
			if right != nil {
				right()
			}
			return nil
		}
		return ev
	})
	if items[0].sep || items[0].off {
		skip(1)
	}
	_, _, sw, sh := a.root.GetRect()
	h := len(items) + 2
	if h > sh-2 {
		h = sh - 2
	}
	if x+w > sw {
		x = max(sw-w, 0)
	}
	if y+h > sh-1 {
		y = max(sh-1-h, 0)
	}
	a.pop = &popup{list: l, items: items, x: x, y: y, w: w, h: h}
	a.popFocus = a.tv.GetFocus()
	a.root.AddPage(popupPage, l, false, true)
	l.SetRect(x, y, w, h)
	a.modals++
	a.tv.SetFocus(l)
}

func (a *App) closePopup() {
	if a.pop == nil {
		return
	}
	a.pop = nil
	a.root.RemovePage(popupPage)
	a.modals--
	a.menuOpen = -1
	a.drawMenuBar()
	if a.popFocus != nil && a.modals == 0 {
		a.tv.SetFocus(a.popFocus)
	}
}

// ---- barra de menus ----

type topMenu struct {
	title string
	items func() []menuItem
	x     int
}

func (a *App) buildMenus() {
	a.menus = []*topMenu{
		{title: L("Arquivo", "File"), items: a.menuFile},
		{title: L("Editar", "Edit"), items: a.menuEdit},
		{title: L("Exibir", "View"), items: a.menuView},
		{title: L("Navegar", "Navigate"), items: a.menuNavigate},
		{title: L("Executar", "Run"), items: a.menuRun},
		{title: L("Ferramentas", "Tools"), items: a.menuTools},
		{title: L("Janela", "Window"), items: a.menuWindow},
		{title: L("Ajuda", "Help"), items: a.menuHelp},
	}
	a.menuOpen = -1
	a.menuBar.SetHighlightedFunc(func(added, _, _ []string) {
		if len(added) == 0 {
			return
		}
		a.menuBar.Highlight()
		id := added[0]
		if strings.HasPrefix(id, "m") {
			var i int
			fmt.Sscanf(id, "m%d", &i)
			a.openMenu(i)
			return
		}
		a.toolbar(id)
	})
	a.drawMenuBar()
}

func (a *App) drawMenuBar() {
	if a.menuBar == nil {
		return
	}
	var b strings.Builder
	x := 1
	b.WriteString(" ")
	for i, m := range a.menus {
		m.x = x
		if i == a.menuOpen {
			fmt.Fprintf(&b, `["m%d"][black:#2EBD59] %s [-:-:-][""] `, i, m.title)
		} else {
			fmt.Fprintf(&b, `["m%d"] %s [""] `, i, m.title)
		}
		x += utf8.RuneCountInString(m.title) + 3
	}
	// barra de ferramentas (como a do SQL Developer / goSSMS), clicável
	tools := []struct{ id, ic, tip string }{
		{"tnew", "✚", L("worksheet", "worksheet")},
		{"trun", "▶", ""}, {"tscript", "▷", ""}, {"tstop", "■", ""},
		{"tplan", "⇲", L("plano", "plan")},
		{"tcommit", "✓", ""}, {"troll", "✗", ""},
		{"tconn", "⛁", L("conectar", "connect")},
	}
	b.WriteString("   [gray]│[-] ")
	for _, t := range tools {
		lbl := t.ic
		if t.tip != "" {
			lbl += " " + t.tip
		}
		fmt.Fprintf(&b, `["%s"][#9fd3c7]%s[-][""]  `, t.id, lbl)
	}
	a.menuBar.SetText(b.String())
}

func (a *App) openMenu(i int) {
	n := len(a.menus)
	i = (i + n) % n
	a.menuOpen = i
	a.drawMenuBar()
	m := a.menus[i]
	a.Popup(m.x, 1, m.items(), func() { a.openMenu(i - 1) }, func() { a.openMenu(i + 1) })
	a.menuOpen = i
	a.drawMenuBar()
}

func (a *App) toolbar(id string) {
	w := a.curWorksheet()
	switch id {
	case "tnew":
		a.newWorksheet(a.currentConnName())
	case "tconn":
		a.ConnectDialog("")
	case "trun", "tscript", "tstop", "tplan", "tcommit", "troll":
		if w == nil {
			a.Msg(L("Abra uma worksheet (Ctrl+N).", "Open a worksheet (Ctrl+N)."))
			return
		}
		w.toolbar(id)
	}
}

func (a *App) curWorksheet() *Worksheet {
	if w, ok := a.tabs.Current().(*Worksheet); ok {
		return w
	}
	return nil
}

// ---- menus ----

func (a *App) menuFile() []menuItem {
	w := a.curWorksheet()
	conn := a.currentConnName()
	return []menuItem{
		item(L("Nova conexão…", "New connection…"), "", func() { a.ConnectDialog("") }),
		item(L("Conectar…", "Connect…"), "F9", func() { a.ConnectDialog(conn) }),
		itemIf(conn != "" && a.Connected(conn) != nil, L("Desconectar", "Disconnect"), "", func() { a.disconnectTree(conn) }),
		sep(),
		item(L("Nova worksheet", "New worksheet"), "Ctrl+N", func() { a.newWorksheet(conn) }),
		itemIf(w != nil, L("Abrir script…", "Open script…"), "Ctrl+O", func() { w.openFile() }),
		itemIf(w != nil, L("Salvar", "Save"), "Ctrl+S", func() { w.saveFile() }),
		itemIf(w != nil, L("Salvar como…", "Save as…"), "", func() { w.file = ""; w.saveFile() }),
		itemIf(a.tabs.Current() != nil, L("Fechar aba", "Close tab"), "Ctrl+W", func() { a.tabs.CloseCurrent() }),
		sep(),
		item(L("Importar conexões do SQL Developer…", "Import SQL Developer connections…"), "", a.importSQLDev),
		sep(),
		item(L("Sair", "Exit"), "Ctrl+Q", a.quit),
	}
}

func (a *App) menuEdit() []menuItem {
	w := a.curWorksheet()
	ed := func(k tcell.Key) func() {
		return func() {
			if w == nil {
				return
			}
			a.tv.SetFocus(w.ed)
			w.ed.InputHandler()(tcell.NewEventKey(k, 0, tcell.ModNone), func(p tview.Primitive) { a.tv.SetFocus(p) })
		}
	}
	return []menuItem{
		itemIf(w != nil, L("Desfazer", "Undo"), "Ctrl+Z", ed(tcell.KeyCtrlZ)),
		itemIf(w != nil, L("Refazer", "Redo"), "Ctrl+Y", ed(tcell.KeyCtrlY)),
		sep(),
		itemIf(w != nil, L("Recortar", "Cut"), "Ctrl+X", ed(tcell.KeyCtrlX)),
		itemIf(w != nil, L("Copiar", "Copy"), "Ctrl+C", ed(tcell.KeyCtrlQ)),
		itemIf(w != nil, L("Colar", "Paste"), "Ctrl+V", ed(tcell.KeyCtrlV)),
		itemIf(w != nil, L("Selecionar tudo", "Select all"), "Ctrl+L", ed(tcell.KeyCtrlL)),
		sep(),
		itemIf(w != nil, L("Completar", "Complete"), "Ctrl+Space", func() { w.complete() }),
		itemIf(w != nil, L("Limpar worksheet", "Clear worksheet"), "", func() { w.ed.SetText("", false) }),
	}
}

func (a *App) menuView() []menuItem {
	conn := a.currentConnName()
	c := a.Connected(conn)
	return []menuItem{
		item(L("Conexões", "Connections"), "F2", func() { a.focusTree("conn") }),
		item("DBA", "F2", func() { a.focusTree("dba") }),
		item(L("Mostrar/ocultar painel esquerdo", "Show/hide left panel"), "", a.toggleLeft),
		sep(),
		itemIf(c != nil, L("Painel da instância", "Instance viewer"), "", func() { a.openViewID(c, "db.panel") }),
		itemIf(c != nil, "Top Activity", "", func() { a.openViewID(c, "db.topactivity") }),
		itemIf(c != nil, L("Sessões", "Sessions"), "", func() { a.openViewID(c, "mo.sessions") }),
		sep(),
		itemIf(a.curWorksheet() != nil, L("Histórico de SQL", "SQL history"), "Ctrl+R", func() { a.curWorksheet().history() }),
		itemIf(a.msgs.Len() > 0, L("Mensagens", "Messages"), "", a.showMessages),
	}
}

func (a *App) menuNavigate() []menuItem {
	conn := a.currentConnName()
	return []menuItem{
		itemIf(a.Connected(conn) != nil, L("Ir para objeto…", "Go to object…"), "Alt+O", func() { a.findObject(a.Connected(conn)) }),
		sep(),
		item(L("Próxima aba", "Next tab"), "Ctrl+PgDn", func() { a.tabs.Next(1) }),
		item(L("Aba anterior", "Previous tab"), "Ctrl+PgUp", func() { a.tabs.Next(-1) }),
		item(L("Alternar foco", "Cycle focus"), "F3", a.cycleFocus),
	}
}

func (a *App) menuRun() []menuItem {
	w := a.curWorksheet()
	ok := w != nil
	ac := L("Ligar autocommit", "Autocommit on")
	if ok && w.sess.AutoCommit {
		ac = L("Desligar autocommit", "Autocommit off")
	}
	return []menuItem{
		itemIf(ok, L("Executar comando", "Run statement"), "F9", func() { w.runCurrent(false) }),
		itemIf(ok, L("Executar script", "Run script"), "F5", w.runScriptSafe),
		itemIf(ok, L("Plano de execução", "Explain plan"), "F10*", func() { w.explain() }),
		itemIf(ok, "Autotrace", "F6", func() { w.runCurrent(true) }),
		itemIf(ok && w.running, L("Cancelar", "Cancel"), "Esc", func() { w.sess.Cancel() }),
		sep(),
		itemIf(ok, "Commit", "F11", func() { w.commit(true) }),
		itemIf(ok, "Rollback", "F12", func() { w.commit(false) }),
		itemIf(ok, ac, "Ctrl+T", func() { w.toggleAutocommit() }),
	}
}

func (a *App) menuTools() []menuItem {
	c := a.Connected(a.currentConnName())
	ok := c != nil
	return []menuItem{
		itemIf(ok, L("Monitorar sessões", "Monitor sessions"), "", func() { a.openViewID(c, "mo.sessions") }),
		itemIf(ok, L("Bloqueios", "Blocking"), "", func() { a.openViewID(c, "mo.blockers") }),
		itemIf(ok, "Top SQL", "", func() { a.openViewID(c, "mo.topsql") }),
		itemIf(ok, "Alert log", "", func() { a.openViewID(c, "mo.alert") }),
		sep(),
		itemIf(ok, "Tablespaces", "", func() { a.openViewID(c, "st.tablespaces") }),
		itemIf(ok, "RMAN: backups", "", func() { a.openViewID(c, "bk.jobs") }),
		itemIf(ok, "Data Pump", "", func() { a.openViewID(c, "dp.jobs") }),
		itemIf(ok, "AWR", "", func() { a.openViewID(c, "pf.snaps") }),
		sep(),
		itemIf(ok, L("Verificar todas as telas DBA", "Check every DBA screen"), "", func() { a.checkAll(c) }),
	}
}

func (a *App) menuWindow() []menuItem {
	var out []menuItem
	for i, t := range a.tabs.All() {
		i := i
		k := ""
		if i < 9 {
			k = fmt.Sprintf("Alt+%d", i+1)
		}
		mark := "  "
		if t == a.tabs.Current() {
			mark = "• "
		}
		out = append(out, item(mark+t.Title(), k, func() { a.tabs.Select(i) }))
	}
	if len(out) == 0 {
		out = append(out, menuItem{label: L("(nenhuma aba aberta)", "(no open tabs)"), off: true})
	}
	out = append(out, sep(), itemIf(len(a.tabs.All()) > 0, L("Fechar todas", "Close all"), "", a.closeAllTabs))
	return out
}

func (a *App) menuHelp() []menuItem {
	return []menuItem{
		item(L("Atalhos e ajuda", "Shortcuts and help"), "F1", a.openHelp),
		item(L("Sobre o ORA BRABO Studio", "About ORA BRABO Studio"), "", func() {
			a.Alert(fmt.Sprintf("ORA BRABO Studio %s\n\n%s\n\nhttps://github.com/acaciolr/ora-brabo-studio\nMIT · by • DBA BRABO",
				Version, L("Cliente Oracle leve no terminal.", "Light Oracle client for the terminal.")))
		}),
	}
}

// ---- menu de contexto ----

// ctxProvider devolve os itens de contexto do primitivo com foco.
type ctxProvider func() []menuItem

// SetContext registra o menu de contexto de um primitivo.
func (a *App) SetContext(p tview.Primitive, f ctxProvider) { a.ctx[p] = f }

// ContextMenu abre o menu de contexto do primitivo (em x, y; -1 = perto do foco).
func (a *App) ContextMenu(p tview.Primitive, x, y int) bool {
	f, ok := a.ctx[p]
	if !ok {
		return false
	}
	items := f()
	if len(items) == 0 {
		return false
	}
	if x < 0 {
		px, py, _, _ := p.GetRect()
		x, y = px+4, py+2
		if t, ok := p.(*tview.TreeView); ok {
			y = py + 1 + rowOf(t)
		}
		if t, ok := p.(*tview.Table); ok {
			r, _ := t.GetSelection()
			ro, _ := t.GetOffset()
			y = py + 1 + r - ro
		}
	}
	a.Popup(x, y, items, nil, nil)
	return true
}

// rowOf acha a linha visível do nó atual da árvore.
func rowOf(t *tview.TreeView) int {
	cur := t.GetCurrentNode()
	n := 0
	found := -1
	var walk func(node *tview.TreeNode, depth int)
	top := 0
	walk = func(node *tview.TreeNode, depth int) {
		if found >= 0 {
			return
		}
		if depth >= top {
			if node == cur {
				found = n
				return
			}
			n++
		}
		if node.IsExpanded() || depth < top {
			for _, c := range node.GetChildren() {
				walk(c, depth+1)
			}
		}
	}
	top = 1
	walk(t.GetRoot(), 0)
	if found < 0 {
		return 0
	}
	return found - t.GetScrollOffset()
}

// mouseTree dá à árvore o comportamento de app gráfico: clique seleciona,
// duplo clique abre, clique direito seleciona e abre o menu de contexto.
func (a *App) mouseTree(t *tview.TreeView) {
	t.SetMouseCapture(func(action tview.MouseAction, ev *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		switch action {
		case tview.MouseLeftClick:
			a.noSelect = true
			a.tv.QueueUpdate(func() { a.noSelect = false })
		case tview.MouseLeftDoubleClick:
			return tview.MouseLeftClick, ev
		case tview.MouseRightClick:
			a.noSelect = true
			x, y := ev.Position()
			a.tv.QueueUpdateDraw(func() {
				a.noSelect = false
				a.tv.SetFocus(t)
				a.ContextMenu(t, x, y)
			})
			return tview.MouseLeftClick, ev
		}
		return action, ev
	})
}

// mouseTable: duplo clique = Enter; clique direito seleciona e abre o contexto.
func (a *App) mouseTable(t *tview.Table, enter func()) {
	t.SetMouseCapture(func(action tview.MouseAction, ev *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		switch action {
		case tview.MouseLeftDoubleClick:
			a.tv.QueueUpdateDraw(enter)
			return tview.MouseLeftClick, ev
		case tview.MouseRightClick:
			x, y := ev.Position()
			a.tv.QueueUpdateDraw(func() {
				a.tv.SetFocus(t)
				a.ContextMenu(t, x, y)
			})
			return tview.MouseLeftClick, ev
		}
		return action, ev
	})
}

// mouseText: clique direito abre o contexto (editor, textos).
func (a *App) mouseText(p interface {
	tview.Primitive
	SetMouseCapture(func(tview.MouseAction, *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse)) *tview.Box
}) {
	p.SetMouseCapture(func(action tview.MouseAction, ev *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseRightClick {
			x, y := ev.Position()
			a.tv.QueueUpdateDraw(func() {
				a.tv.SetFocus(p)
				a.ContextMenu(p, x, y)
			})
			return tview.MouseConsumed, nil
		}
		return action, ev
	})
}
