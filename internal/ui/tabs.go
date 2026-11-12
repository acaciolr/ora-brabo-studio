// Abas da área de trabalho: barra de títulos clicável + páginas. by • DBA BRABO
package ui

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/rivo/tview"
)

// Tab é o que vive numa aba.
type Tab interface {
	tview.Primitive
	Title() string
	Conn() *Conn
	// Focus põe o foco: step 0 = principal, 1 = próximo (devolve false se não tem)
	FocusPart(step int) bool
	Hint() string
	Close()
}

// Tabs gerencia as abas.
type Tabs struct {
	*tview.Flex
	a     *App
	bar   *tview.TextView
	pages *tview.Pages
	list  []Tab
	cur   int
	seq   int
	ids   map[Tab]string
}

// NewTabs cria a área de abas.
func NewTabs(a *App) *Tabs {
	t := &Tabs{Flex: tview.NewFlex().SetDirection(tview.FlexRow), a: a,
		bar:   tview.NewTextView().SetDynamicColors(true).SetRegions(true).SetWrap(false),
		pages: tview.NewPages(), cur: -1, ids: map[Tab]string{}}
	t.bar.SetHighlightedFunc(func(added, _, _ []string) {
		if len(added) > 0 {
			if i, err := strconv.Atoi(added[0]); err == nil {
				t.Select(i)
			}
		}
	})
	t.AddItem(t.bar, 1, 0, false).AddItem(t.pages, 0, 1, true)
	t.SetBorder(true)
	return t
}

func (t *Tabs) draw() {
	var b strings.Builder
	for i, tab := range t.list {
		title := tab.Title()
		if len([]rune(title)) > 28 {
			title = string([]rune(title)[:27]) + "…"
		}
		cor := ""
		if c := tab.Conn(); c != nil && c.Info.Color != "" {
			cor = "[" + c.Info.Color + "]▌[-]"
		}
		if i == t.cur {
			fmt.Fprintf(&b, `["%d"][black:#9fd3c7] %s%s [-:-:-][""] `, i, cor, tview.Escape(title))
		} else {
			fmt.Fprintf(&b, `["%d"][#9fd3c7:#1e2d37] %s%s [-:-:-][""] `, i, cor, tview.Escape(title))
		}
	}
	t.bar.SetText(b.String())
	if t.cur >= 0 {
		t.SetTitle(" " + tview.Escape(t.list[t.cur].Title()) + " ")
	} else {
		t.SetTitle("")
	}
}

// Add abre uma aba e vai para ela.
func (t *Tabs) Add(tab Tab) {
	t.seq++
	id := "tab" + strconv.Itoa(t.seq)
	t.ids[tab] = id
	t.list = append(t.list, tab)
	t.pages.AddPage(id, tab, true, false)
	t.Select(len(t.list) - 1)
}

// Select vai para a aba i.
func (t *Tabs) Select(i int) {
	if i < 0 || i >= len(t.list) {
		return
	}
	t.cur = i
	t.pages.SwitchToPage(t.ids[t.list[i]])
	t.draw()
	t.list[i].FocusPart(0)
	t.a.drawHeader()
	t.a.hint()
}

// Next anda entre as abas.
func (t *Tabs) Next(d int) {
	if len(t.list) == 0 {
		return
	}
	t.Select((t.cur + d + len(t.list)) % len(t.list))
}

// Current é a aba visível.
func (t *Tabs) Current() Tab {
	if t.cur < 0 || t.cur >= len(t.list) {
		return nil
	}
	return t.list[t.cur]
}

// All lista as abas.
func (t *Tabs) All() []Tab { return append([]Tab(nil), t.list...) }

// Find acha uma aba pelo título (para não abrir a mesma tela duas vezes).
func (t *Tabs) Find(title string, c *Conn) int {
	for i, tab := range t.list {
		if tab.Title() == title && tab.Conn() == c {
			return i
		}
	}
	return -1
}

// CloseCurrent fecha a aba atual (worksheet com pendência pergunta antes).
func (t *Tabs) CloseCurrent() {
	tab := t.Current()
	if tab == nil {
		return
	}
	if w, ok := tab.(*Worksheet); ok && w.sess != nil && w.sess.Pending {
		t.a.Confirm(L("Esta worksheet tem alterações sem commit.\nFechar faz ROLLBACK. Fechar mesmo?", "This worksheet has uncommitted changes.\nClosing rolls them back. Close anyway?"),
			func() { t.Remove(tab) })
		return
	}
	t.Remove(tab)
}

// Remove fecha uma aba.
func (t *Tabs) Remove(tab Tab) {
	for i, x := range t.list {
		if x == tab {
			tab.Close()
			t.pages.RemovePage(t.ids[tab])
			delete(t.ids, tab)
			t.list = append(t.list[:i], t.list[i+1:]...)
			if t.cur >= len(t.list) {
				t.cur = len(t.list) - 1
			}
			if t.cur >= 0 {
				t.Select(t.cur)
			} else {
				t.draw()
				t.a.showLeft(t.a.leftOn)
				t.a.drawHeader()
				t.a.hint()
			}
			return
		}
	}
}

// Redraw atualiza os títulos.
func (t *Tabs) Redraw() { t.draw() }

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
