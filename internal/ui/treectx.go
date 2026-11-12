// Menus de contexto das árvores (o clique direito do SQL Developer), atualizar
// nó, ir para objeto e a verificação de todas as telas DBA. by • DBA BRABO
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/acaciolr/ora-brabo-studio/internal/catalog"
)

// treeNav: + abre, - / Backspace fecha (como no goSSMS).
func treeNav(n *tview.TreeNode, ev *tcell.EventKey) *tcell.EventKey {
	switch {
	case ev.Key() == tcell.KeyRune && ev.Rune() == '+':
		n.SetExpanded(true)
		return nil
	case ev.Key() == tcell.KeyRune && ev.Rune() == '-', ev.Key() == tcell.KeyBackspace, ev.Key() == tcell.KeyBackspace2:
		n.SetExpanded(false)
		return nil
	}
	return ev
}

func (a *App) connCtx() []menuItem {
	n := a.connT.GetCurrentNode()
	if n == nil {
		return nil
	}
	r, _ := n.GetReference().(*nodeRef)
	if r == nil {
		return nil
	}
	open := func() { a.onConnSelect(n) }
	switch r.kind {
	case nkNewConn, nkImport:
		return []menuItem{
			item(L("Nova conexão…", "New connection…"), "", func() { a.ConnectDialog("") }),
			item(L("Importar do SQL Developer…", "Import from SQL Developer…"), "", a.importSQLDev),
		}
	case nkConn:
		on := a.Connected(r.conn) != nil
		return []menuItem{
			itemIf(!on, L("Conectar", "Connect"), "Enter", open),
			itemIf(on, L("Desconectar", "Disconnect"), "d", func() { a.disconnectTree(r.conn) }),
			itemIf(on, L("Reconectar", "Reconnect"), "", func() { a.disconnectTree(r.conn); open() }),
			sep(),
			item(L("Nova worksheet", "New worksheet"), "w", func() { a.newWorksheet(r.conn) }),
			item(L("Abrir na aba DBA", "Open in the DBA tab"), "", func() { a.openInDBA(r.conn) }),
			itemIf(on, L("Ir para objeto…", "Go to object…"), "Alt+O", func() { a.findObject(a.Connected(r.conn)) }),
			sep(),
			item(L("Propriedades…", "Properties…"), "e", func() { a.ConnectDialog(r.conn) }),
			item(L("Copiar conexão…", "Copy connection…"), "c", func() {
				info, _ := a.findConn(r.conn)
				info.Name += L(" (cópia)", " (copy)")
				a.connForm(&info)
			}),
			item(L("Remover conexão", "Delete connection"), "x", func() {
				a.Confirm(fmt.Sprintf(L("Remover a conexão %q?", "Delete connection %q?"), r.conn), func() { a.deleteConn(r.conn) })
			}),
			sep(),
			itemIf(on && r.loaded, L("Atualizar", "Refresh"), "F5", func() { a.refreshNode(n, r) }),
		}
	case nkNewWorksheet:
		return []menuItem{item(L("Nova worksheet", "New worksheet"), "Enter", open)}
	case nkCategory, nkUser, nkOtherUsers:
		return []menuItem{
			item(map[bool]string{true: L("Recolher", "Collapse"), false: L("Expandir", "Expand")}[n.IsExpanded()], "Enter", func() {
				if n.IsExpanded() {
					n.SetExpanded(false)
				} else {
					a.onConnSelect(n)
				}
			}),
			item(L("Atualizar", "Refresh"), "F5", func() { a.refreshNode(n, r) }),
		}
	case nkObject:
		c := a.Connected(r.conn)
		if c == nil {
			return nil
		}
		row := catalog.ObjRow(r.owner, r.name, r.typ)
		q := catalog.Q2(r.owner, r.name)
		items := []menuItem{
			item(L("Abrir", "Open"), "Enter", open),
			item(L("Mostrar DDL", "Show DDL"), "D", func() {
				a.openText(c, "DDL "+r.name, catalog.DDLSetup()+"\n/\n"+catalog.DDLQuery(r.typ), []any{r.owner, r.name})
			}),
		}
		if r.typ == "TABLE" || r.typ == "VIEW" || r.typ == "MATERIALIZED VIEW" || r.typ == "SYNONYM" {
			items = append(items, item(L("SELECT numa worksheet", "SELECT in a worksheet"), "", func() {
				if w, err := NewWorksheet(a, c, "SELECT *\n  FROM "+q+";\n"); err == nil {
					a.tabs.Add(w)
				}
			}))
		}
		items = append(items,
			item(L("Copiar nome", "Copy name"), "", func() { Copy(q); a.Msg(L("Copiado: ", "Copied: ") + q) }),
			sep())
		for _, x := range catalog.ObjectActions(r.typ) {
			x := x
			lbl := x.Name
			if x.Danger {
				lbl = "⚠ " + lbl
			}
			items = append(items, item(lbl, string(x.Key), func() { a.RunAction(c, x, row, func() {}) }))
		}
		return items
	}
	return nil
}

func (a *App) dbaCtx() []menuItem {
	n := a.dbaT.GetCurrentNode()
	if n == nil {
		return nil
	}
	r, _ := n.GetReference().(*nodeRef)
	if r == nil {
		return nil
	}
	switch r.kind {
	case nkDBAConn:
		on := a.Connected(r.conn) != nil
		return []menuItem{
			itemIf(!on, L("Conectar", "Connect"), "Enter", func() { a.onDBASelect(n) }),
			itemIf(on, L("Desconectar", "Disconnect"), "", func() { a.disconnectTree(r.conn) }),
			sep(),
			item(L("Nova worksheet", "New worksheet"), "Ctrl+N", func() { a.newWorksheet(r.conn) }),
			itemIf(on, L("Painel da instância", "Instance viewer"), "", func() { a.openViewID(a.Connected(r.conn), "db.panel") }),
			itemIf(on, "Top Activity", "", func() { a.openViewID(a.Connected(r.conn), "db.topactivity") }),
		}
	case nkDBAGroup:
		return []menuItem{item(map[bool]string{true: L("Recolher", "Collapse"), false: L("Expandir", "Expand")}[n.IsExpanded()], "Enter", func() { n.SetExpanded(!n.IsExpanded()) })}
	case nkDBAView:
		v := r.view
		return []menuItem{
			item(L("Abrir", "Open"), "Enter", func() { a.onDBASelect(n) }),
			item(L("Abrir com atualização automática", "Open with auto refresh"), "", func() {
				a.Connect(r.conn, func(c *Conn) {
					a.openView(c, v)
					if l, ok := a.tabs.Current().(*ListView); ok && !l.auto {
						l.toggleAuto()
					}
				})
			}),
			item(L("Copiar a consulta da tela", "Copy the screen query"), "", func() { Copy(strings.TrimSpace(v.SQL)); a.Msg(L("Consulta copiada.", "Query copied.")) }),
		}
	}
	return nil
}

func (a *App) dbaTreeKeys(ev *tcell.EventKey) *tcell.EventKey {
	n := a.dbaT.GetCurrentNode()
	if n == nil {
		return ev
	}
	return treeNav(n, ev)
}

// disconnectTree desconecta e fecha os nós das duas árvores.
func (a *App) disconnectTree(name string) {
	a.Disconnect(name)
	for _, t := range []*tview.TreeView{a.connT, a.dbaT} {
		for _, n := range t.GetRoot().GetChildren() {
			if r, ok := n.GetReference().(*nodeRef); ok && r.conn == name && (r.kind == nkConn || r.kind == nkDBAConn) {
				n.SetText(connLabel(name, false)).ClearChildren().SetExpanded(false)
				r.loaded = false
			}
		}
	}
}

// markConnected acende o ícone da conexão nas duas árvores.
func (a *App) markConnected(name string) {
	for _, t := range []*tview.TreeView{a.connT, a.dbaT} {
		for _, n := range t.GetRoot().GetChildren() {
			if r, ok := n.GetReference().(*nodeRef); ok && r.conn == name && (r.kind == nkConn || r.kind == nkDBAConn) {
				n.SetText(connLabel(name, true))
			}
		}
	}
}

func (a *App) refreshNode(n *tview.TreeNode, r *nodeRef) {
	switch r.kind {
	case nkConn:
		if c := a.Connected(r.conn); c != nil {
			c.mu.Lock()
			c.names = nil
			c.mu.Unlock()
			a.fillConnNode(n, c)
			n.SetExpanded(true)
		}
	case nkCategory, nkUser, nkOtherUsers:
		a.loadChildren(n, r)
	}
}

// openInDBA abre a conexão na árvore DBA.
func (a *App) openInDBA(name string) {
	for _, n := range a.dbaT.GetRoot().GetChildren() {
		if r, ok := n.GetReference().(*nodeRef); ok && r.conn == name {
			a.dbaT.SetCurrentNode(n)
			a.focusTree("dba")
			if !n.IsExpanded() {
				a.onDBASelect(n)
			}
			return
		}
	}
}

// openViewID abre uma tela do catálogo pelo ID.
func (a *App) openViewID(c *Conn, id string) {
	if c == nil {
		return
	}
	var v *catalog.View
	catalog.Walk(catalog.DBA(), func(_ []string, n *catalog.Node) {
		if n.View != nil && n.View.ID == id && v == nil {
			v = n.View
		}
	})
	if v == nil {
		a.Errf("tela %s não existe", id)
		return
	}
	a.openView(c, v)
}

// findObject: "Ir para objeto" (Find DB Object do SQL Developer).
func (a *App) findObject(c *Conn) {
	if c == nil {
		return
	}
	in := tview.NewInputField().SetLabel(L("Nome (início, % curinga): ", "Name (prefix, % wildcard): ")).SetFieldWidth(40)
	in.SetBorder(true).SetTitle(" " + L("Ir para objeto", "Go to object") + " · " + tview.Escape(c.Info.Name) + " ")
	in.SetDoneFunc(func(k tcell.Key) {
		txt := strings.ToUpper(strings.TrimSpace(in.GetText()))
		a.CloseModal("find")
		if k != tcell.KeyEnter || txt == "" {
			return
		}
		a.Msg(L("Procurando…", "Searching…"))
		a.Go(func() func() {
			r := Query(c, `select owner, object_name, object_type from all_objects
			  where object_name like :1 escape '\' and object_type in ('TABLE','VIEW','INDEX','PACKAGE','PROCEDURE','FUNCTION','TRIGGER','TYPE','SEQUENCE','SYNONYM','MATERIALIZED VIEW','JOB')
			  order by decode(owner, user, 0, 1), object_name, owner fetch first 200 rows only`, []any{strings.TrimSuffix(txt, "%") + "%"}, 200)
			return func() {
				if r.Err != nil {
					a.Errf("%v", r.Err)
					return
				}
				if len(r.Rows) == 0 {
					a.Msg(L("Nada encontrado.", "Nothing found."))
					return
				}
				var items []menuItem
				for _, row := range r.Rows {
					o, n, t := row[0].V, row[1].V, row[2].V
					items = append(items, item(o+"."+n, strings.ToLower(t), func() { a.openObject(c, o, n, t) }))
				}
				_, _, w, h := a.root.GetRect()
				a.Popup(w/2-30, h/4, items, nil, nil)
			}
		})
	})
	a.Modal("find", in, 70, 3)
}

// checkAll roda a consulta de cada tela DBA e mostra o resultado numa aba.
func (a *App) checkAll(c *Conn) {
	t := NewTextTab(a, c, L("Verificação das telas DBA", "DBA screens check"), "")
	a.tabs.Add(t)
	var views []*catalog.View
	seen := map[string]bool{}
	catalog.Walk(catalog.DBA(), func(_ []string, n *catalog.Node) {
		if n.View != nil && n.View.SQL != "" && !seen[n.View.ID] {
			seen[n.View.ID] = true
			views = append(views, n.View)
		}
	})
	go func() {
		ok, fail := 0, 0
		for _, v := range views {
			args := make([]any, len(v.Args))
			r := Query(c, v.SQL, args, 20)
			line := fmt.Sprintf("ok    %-24s %s (%s)\n", v.ID, v.Title, r.Elapsed.Round(time.Millisecond))
			if r.Err != nil {
				fail++
				line = fmt.Sprintf("ERRO  %-24s %s\n      %s\n", v.ID, v.Title, firstLine(r.Err.Error()))
			} else {
				ok++
			}
			a.tv.QueueUpdateDraw(func() { t.Append(line) })
		}
		end := fmt.Sprintf(L("\n%d telas: %d ok, %d com erro\n", "\n%d screens: %d ok, %d with errors\n"), ok+fail, ok, fail)
		a.tv.QueueUpdateDraw(func() { t.Append(end) })
	}()
}
