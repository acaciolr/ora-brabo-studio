// Gera os prints do README (SVG) a partir da tela simulada: rode com
// SHOTS_DIR=docs/img go test -run TestShots ./internal/ui/  by • DBA BRABO
package ui

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"

	"github.com/acaciolr/ora-brabo-studio/internal/catalog"
	"github.com/acaciolr/ora-brabo-studio/internal/i18n"
)

const (
	shotFG = "#d4d4d4"
	shotBG = "#0f1a20"
)

func hexColor(c tcell.Color, def string) string {
	if c == tcell.ColorDefault || !c.Valid() {
		return def
	}
	r, g, b := c.RGB()
	if r < 0 {
		return def
	}
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// SVG da tela simulada.
func screenSVG(s tcell.SimulationScreen, title string) string {
	cells, w, h := s.GetContents()
	const cw, ch = 8.4, 17.0
	var b strings.Builder
	W, H := float64(w)*cw+24, float64(h)*ch+52
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`, W, H, W, H)
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" rx="10" fill="%s"/>`, "#0a1216")
	b.WriteString(`<circle cx="20" cy="16" r="6" fill="#ff5f57"/><circle cx="40" cy="16" r="6" fill="#febc2e"/><circle cx="60" cy="16" r="6" fill="#28c840"/>`)
	fmt.Fprintf(&b, `<text x="%.0f" y="21" fill="#7f8c8d" font-family="monospace" font-size="13" text-anchor="middle">%s</text>`, W/2, html.EscapeString(title))
	fmt.Fprintf(&b, `<g transform="translate(12,36)"><rect width="%.1f" height="%.1f" fill="%s"/>`, float64(w)*cw, float64(h)*ch, shotBG)
	b.WriteString(`<g font-family="DejaVu Sans Mono,Menlo,Consolas,monospace" font-size="14">`)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			fg, bg, attr := c.Style.Decompose()
			if attr&tcell.AttrReverse != 0 {
				fg, bg = bg, fg
			}
			if bgc := hexColor(bg, ""); bgc != "" {
				fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`, float64(x)*cw, float64(y)*ch, cw+0.4, ch+0.4, bgc)
			}
			if len(c.Runes) == 0 || c.Runes[0] == ' ' {
				continue
			}
			wt := ""
			if attr&tcell.AttrBold != 0 {
				wt = ` font-weight="bold"`
			}
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s"%s>%s</text>`, float64(x)*cw, float64(y)*ch+13, hexColor(fg, shotFG), wt, html.EscapeString(string(c.Runes)))
		}
	}
	b.WriteString(`</g></g></svg>`)
	return b.String()
}

func TestShots(t *testing.T) {
	dir := os.Getenv("SHOTS_DIR")
	if dir == "" {
		t.Skip("SHOTS_DIR não definido")
	}
	_ = os.MkdirAll(dir, 0o755)
	for _, lang := range []string{"pt", "en"} {
		h := startLang(t, lang == "en")
		shot := func(name, title string) {
			h.sync()
			p := filepath.Join(dir, name+"-"+lang+".svg")
			if err := os.WriteFile(p, []byte(screenSVG(h.s, title)), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		shot("welcome", "ORA BRABO Studio")

		// conexões salvas + uma "aberta" (sem banco) para mostrar as árvores
		h.a.tv.QueueUpdateDraw(func() {
			h.a.conns = append(h.a.conns, oraInfo("PROD · db01"), oraInfo("HML · db02"), oraInfo("lab-26ai"))
			h.a.conns[0].Color, h.a.conns[0].User = "red", "system"
			h.a.buildConnTree()
			h.a.buildDBATree()
			c := &Conn{Info: h.a.conns[0], User: "SYSTEM", Version: "23.26.0.0.0", ConName: "FREEPDB1", packOK: map[string]bool{}}
			h.a.open[c.Info.Name] = c
			ch := h.a.dbaT.GetRoot().GetChildren()[0]
			ch.SetText(connLabel(c.Info.Name, true)).ClearChildren()
			for _, g := range catalog.DBA().Children {
				h.a.addDBAGroup(ch, c.Info.Name, g)
			}
			ch.SetExpanded(true)
			for _, g := range ch.GetChildren()[:3] {
				g.SetExpanded(true)
			}
			n := h.a.connT.GetRoot().GetChildren()[2]
			n.SetText(connLabel(c.Info.Name, true))
			h.a.fillConnNode(n, c)
			n.SetExpanded(true)
			h.a.tabs.Remove(h.a.tabs.Current())
			h.a.showLeft("dba")
		})
		shot("dba-tree", "ORA BRABO Studio · DBA")

		h.a.tv.QueueUpdateDraw(func() { h.a.showLeft("conn"); h.a.tabs.Add(NewHelp(h.a)) })
		shot("help", "ORA BRABO Studio · F1")

		h.a.tv.QueueUpdateDraw(func() {
			info := oraInfo("PROD · db01")
			info.Role, info.RmanCmd, info.Color = "SYSDBA", "ssh oracle@db01 '. ~/.bash_profile; rman target /'", "red"
			h.a.connForm(&info)
		})
		shot("connection", "ORA BRABO Studio · conexão")
		h.a.tv.QueueUpdateDraw(func() { h.a.CloseModal("connform") })

		c := h.a.open["PROD · db01"]
		var x *catalog.Action
		catalog.Walk(catalog.DBA(), func(_ []string, n *catalog.Node) {
			if n.View != nil && n.View.ID == "st.tablespaces" {
				x = n.View.FindAction('a')
			}
		})
		row := catalog.Row{"TABLESPACE_NAME": "USERS", "BIGFILE": "NO"}
		h.a.tv.QueueUpdateDraw(func() { h.a.askFields(x, row, func(catalog.Vals) {}) })
		shot("action-form", "ORA BRABO Studio · DBA")
		h.a.tv.QueueUpdateDraw(func() {
			h.a.CloseModal("fields")
			vals := catalog.Vals{}
			for _, f := range x.Fields {
				vals[f.Name] = catalog.Fill(f.Default, row)
			}
			sc, err := x.Build(row, vals)
			if err != nil {
				t.Error(err)
				return
			}
			h.a.preview(c, x, row, sc, nil)
		})
		shot("action-preview", "ORA BRABO Studio · DBA")
		h.stop()
	}
	i18n.EN = false
}

func startLang(t *testing.T, en bool) *harness {
	t.Setenv("ORA_BRABO_STUDIO_HOME", t.TempDir())
	i18n.EN = en
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, s: s, a: New(s), err: make(chan error, 1)}
	go func() { h.err <- h.a.tv.Run() }()
	h.wait("ORA BRABO")
	s.SetSize(150, 40)
	h.sync()
	return h
}
