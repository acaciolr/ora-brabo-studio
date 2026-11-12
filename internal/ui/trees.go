// Árvores da esquerda: Conexões (com o navegador de objetos) e DBA (o catálogo
// por conexão). Enter abre/expande; letras fazem as ações do nó. by • DBA BRABO
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/acaciolr/ora-brabo-studio/internal/catalog"
)

type nodeKind int

const (
	nkNewConn nodeKind = iota
	nkImport
	nkConn
	nkNewWorksheet
	nkCategory
	nkObject
	nkOtherUsers
	nkUser
	nkDBAConn
	nkDBAGroup
	nkDBAView
)

type nodeRef struct {
	kind   nodeKind
	conn   string
	owner  string
	cat    catalog.Category
	name   string
	typ    string
	view   *catalog.View
	group  *catalog.Node
	loaded bool
}

var (
	colConn   = tcell.NewRGBColor(159, 211, 199)
	colFolder = tcell.NewRGBColor(230, 200, 120)
	colObj    = tcell.ColorWhite
	colDim    = tcell.ColorGray
	colAction = tcell.NewRGBColor(46, 189, 89)
)

func (a *App) buildConnTree() {
	root := tview.NewTreeNode(L("Conexões", "Connections")).SetColor(colConn).SetSelectable(false)
	root.AddChild(tview.NewTreeNode("+ " + L("Nova conexão", "New connection")).SetColor(colAction).SetReference(&nodeRef{kind: nkNewConn}))
	root.AddChild(tview.NewTreeNode("⇪ " + L("Importar do SQL Developer", "Import from SQL Developer")).SetColor(colAction).SetReference(&nodeRef{kind: nkImport}))
	for _, c := range a.conns {
		root.AddChild(a.connNode(c.Name))
	}
	a.connT.SetRoot(root).SetCurrentNode(root.GetChildren()[0]).SetTopLevel(1)
	a.connT.SetGraphicsColor(tcell.NewRGBColor(70, 90, 100))
	a.connT.SetSelectedFunc(a.onConnSelect)
	a.connT.SetInputCapture(a.connTreeKeys)
}

func connLabel(name string, open bool) string {
	ic := "[gray]○[-]"
	if open {
		ic = "[#2EBD59]●[-]"
	}
	return ic + " " + tview.Escape(name)
}

func (a *App) connNode(name string) *tview.TreeNode {
	n := tview.NewTreeNode(connLabel(name, a.Connected(name) != nil)).SetColor(colConn).
		SetReference(&nodeRef{kind: nkConn, conn: name}).SetExpanded(false)
	return n
}

func (a *App) onConnSelect(n *tview.TreeNode) {
	r, _ := n.GetReference().(*nodeRef)
	if r == nil {
		return
	}
	switch r.kind {
	case nkNewConn:
		a.connForm(nil)
	case nkImport:
		a.importSQLDev()
	case nkConn:
		if n.IsExpanded() && r.loaded {
			n.SetExpanded(false)
			return
		}
		a.Connect(r.conn, func(c *Conn) {
			n.SetText(connLabel(r.conn, true))
			a.fillConnNode(n, c)
			r.loaded = true
			n.SetExpanded(true)
		})
	case nkNewWorksheet:
		a.newWorksheet(r.conn)
	case nkCategory, nkUser, nkOtherUsers:
		if n.IsExpanded() && r.loaded {
			n.SetExpanded(false)
			return
		}
		a.loadChildren(n, r)
	case nkObject:
		a.Connect(r.conn, func(c *Conn) { a.openObject(c, r.owner, r.name, r.typ) })
	}
}

func (a *App) fillConnNode(n *tview.TreeNode, c *Conn) {
	n.ClearChildren()
	n.AddChild(tview.NewTreeNode("▶ " + L("Nova worksheet", "New worksheet")).SetColor(colAction).SetReference(&nodeRef{kind: nkNewWorksheet, conn: c.Info.Name}))
	for _, cat := range catalog.Categories() {
		n.AddChild(tview.NewTreeNode("▸ " + cat.Title).SetColor(colFolder).
			SetReference(&nodeRef{kind: nkCategory, conn: c.Info.Name, owner: c.User, cat: cat}))
	}
	n.AddChild(tview.NewTreeNode("▸ " + L("Outros usuários", "Other users")).SetColor(colFolder).
		SetReference(&nodeRef{kind: nkOtherUsers, conn: c.Info.Name}))
}

// loadChildren lê os objetos de uma pasta (ou os usuários) em segundo plano.
func (a *App) loadChildren(n *tview.TreeNode, r *nodeRef) {
	c := a.Connected(r.conn)
	if c == nil {
		return
	}
	n.ClearChildren()
	n.AddChild(tview.NewTreeNode(L("carregando…", "loading…")).SetColor(colDim).SetSelectable(false))
	n.SetExpanded(true)
	a.Go(func() func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var q string
		var args []any
		switch r.kind {
		case nkOtherUsers:
			q = `select username, case when oracle_maintained = 'Y' then 'oracle' end from all_users order by oracle_maintained, username`
		case nkUser:
			return func() {
				n.ClearChildren()
				for _, cat := range catalog.Categories() {
					n.AddChild(tview.NewTreeNode("▸ " + cat.Title).SetColor(colFolder).
						SetReference(&nodeRef{kind: nkCategory, conn: r.conn, owner: r.owner, cat: cat}))
				}
				r.loaded = true
			}
		default:
			q, args = r.cat.CategorySQL(), []any{r.owner}
			if strings.Count(q, ":1") > 1 {
				// a mesma bind :1 repetida: o go-ora aceita por nome; passamos uma vez
			}
		}
		rows, err := c.DB.QueryContext(ctx, q, args...)
		if err != nil {
			return func() {
				n.ClearChildren()
				n.AddChild(tview.NewTreeNode("[red]" + tview.Escape(err.Error()) + "[-]").SetSelectable(false))
			}
		}
		defer rows.Close()
		type it struct{ a, b, c string }
		var items []it
		cols, _ := rows.Columns()
		for rows.Next() && len(items) < 5000 {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if rows.Scan(ptrs...) != nil {
				continue
			}
			s := make([]string, 3)
			for i := 0; i < len(cols) && i < 3; i++ {
				if vals[i] != nil {
					s[i] = fmt.Sprint(vals[i])
				}
			}
			items = append(items, it{s[0], s[1], s[2]})
		}
		return func() {
			n.ClearChildren()
			if len(items) == 0 {
				n.AddChild(tview.NewTreeNode(L("(vazio)", "(empty)")).SetColor(colDim).SetSelectable(false))
			}
			for _, x := range items {
				if r.kind == nkOtherUsers {
					lbl := "▸ " + tview.Escape(x.a)
					col := colFolder
					if x.b != "" {
						col = colDim
					}
					n.AddChild(tview.NewTreeNode(lbl).SetColor(col).SetReference(&nodeRef{kind: nkUser, conn: r.conn, owner: x.a}))
					continue
				}
				lbl := tview.Escape(x.a)
				if x.c != "" {
					lbl += " [gray]" + tview.Escape(x.c) + "[-]"
				}
				col := colObj
				if x.c == "INVALID" {
					col = tcell.ColorRed
				}
				n.AddChild(tview.NewTreeNode(lbl).SetColor(col).
					SetReference(&nodeRef{kind: nkObject, conn: r.conn, owner: r.owner, name: x.a, typ: x.b}))
			}
			r.loaded = true
		}
	})
}

func (a *App) connTreeKeys(ev *tcell.EventKey) *tcell.EventKey {
	n := a.connT.GetCurrentNode()
	if n == nil {
		return ev
	}
	r, _ := n.GetReference().(*nodeRef)
	if r == nil || ev.Key() != tcell.KeyRune {
		return ev
	}
	switch r.kind {
	case nkConn:
		switch ev.Rune() {
		case 'e':
			info, _ := a.findConn(r.conn)
			a.connForm(&info)
			return nil
		case 'c':
			info, _ := a.findConn(r.conn)
			info.Name += L(" (cópia)", " (copy)")
			a.connForm(&info)
			return nil
		case 'x':
			a.Confirm(fmt.Sprintf(L("Remover a conexão %q?", "Delete connection %q?"), r.conn), func() { a.deleteConn(r.conn) })
			return nil
		case 'd':
			a.Disconnect(r.conn)
			n.SetText(connLabel(r.conn, false)).ClearChildren()
			r.loaded = false
			return nil
		case 'w':
			a.newWorksheet(r.conn)
			return nil
		}
	case nkCategory, nkUser, nkOtherUsers:
		if ev.Rune() == 'r' {
			a.loadChildren(n, r)
			return nil
		}
	case nkObject:
		c := a.Connected(r.conn)
		if c == nil {
			return ev
		}
		row := catalog.ObjRow(r.owner, r.name, r.typ)
		if ev.Rune() == 'D' {
			a.openText(c, "DDL "+r.name, catalog.DDLSetup()+"\n/\n"+catalog.DDLQuery(r.typ), []any{r.owner, r.name})
			return nil
		}
		if ev.Rune() == 'w' {
			a.newWorksheet(r.conn)
			return nil
		}
		for _, act := range catalog.ObjectActions(r.typ) {
			if act.Key == ev.Rune() {
				a.RunAction(c, act, row, func() {})
				return nil
			}
		}
	}
	return ev
}

// ---- árvore DBA ----

func (a *App) buildDBATree() {
	root := tview.NewTreeNode("DBA").SetColor(colConn).SetSelectable(false)
	for _, c := range a.conns {
		root.AddChild(tview.NewTreeNode(connLabel(c.Name, a.Connected(c.Name) != nil)).SetColor(colConn).
			SetReference(&nodeRef{kind: nkDBAConn, conn: c.Name}).SetExpanded(false))
	}
	if len(a.conns) == 0 {
		root.AddChild(tview.NewTreeNode(L("(crie uma conexão na aba Conexões)", "(create a connection in Connections)")).SetColor(colDim).SetSelectable(false))
	}
	a.dbaT.SetRoot(root).SetTopLevel(1)
	if ch := root.GetChildren(); len(ch) > 0 {
		a.dbaT.SetCurrentNode(ch[0])
	}
	a.dbaT.SetGraphicsColor(tcell.NewRGBColor(70, 90, 100))
	a.dbaT.SetSelectedFunc(a.onDBASelect)
}

func (a *App) onDBASelect(n *tview.TreeNode) {
	r, _ := n.GetReference().(*nodeRef)
	if r == nil {
		return
	}
	switch r.kind {
	case nkDBAConn:
		if n.IsExpanded() && r.loaded {
			n.SetExpanded(false)
			return
		}
		a.Connect(r.conn, func(c *Conn) {
			n.SetText(connLabel(r.conn, true))
			n.ClearChildren()
			for _, g := range catalog.DBA().Children {
				a.addDBAGroup(n, r.conn, g)
			}
			r.loaded = true
			n.SetExpanded(true)
		})
	case nkDBAGroup:
		n.SetExpanded(!n.IsExpanded())
	case nkDBAView:
		a.Connect(r.conn, func(c *Conn) { a.openView(c, r.view) })
	}
}

func (a *App) addDBAGroup(parent *tview.TreeNode, conn string, g *catalog.Node) {
	if g.View != nil && len(g.Children) == 0 {
		parent.AddChild(tview.NewTreeNode(tview.Escape(g.Title)).SetColor(colObj).
			SetReference(&nodeRef{kind: nkDBAView, conn: conn, view: g.View}))
		return
	}
	n := tview.NewTreeNode("▸ " + tview.Escape(g.Title)).SetColor(colFolder).
		SetReference(&nodeRef{kind: nkDBAGroup, conn: conn, group: g}).SetExpanded(false)
	for _, ch := range g.Children {
		a.addDBAGroup(n, conn, ch)
	}
	parent.AddChild(n)
}

// openView abre (ou volta para) a aba de uma tela do catálogo.
func (a *App) openView(c *Conn, v *catalog.View) {
	open := func() {
		if v.Needs == "cdb" && !c.CDB {
			a.Alert(L("Este banco não é CDB (multitenant): a tela não se aplica.", "This database is not a CDB (multitenant): the screen does not apply."))
			return
		}
		title := v.Title
		if i := a.tabs.Find(title, c); i >= 0 {
			a.tabs.Select(i)
			return
		}
		switch v.Special {
		case "instance":
			a.tabs.Add(NewInstancePanel(a, c))
		case "topactivity":
			a.tabs.Add(NewTopActivity(a, c))
		default:
			a.tabs.Add(NewListView(a, c, v))
		}
	}
	a.checkPack(c, v.Pack, open)
}
