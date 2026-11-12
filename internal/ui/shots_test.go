// Gera os prints do README a partir da tela simulada (SVG → PNG pelo
// rsvg-convert, e o GIF de demonstração): rode com
// SHOTS_DIR=docs/img go test -run TestShots ./internal/ui/  by • DBA BRABO
package ui

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/acaciolr/ora-brabo-studio/internal/catalog"
	"github.com/acaciolr/ora-brabo-studio/internal/i18n"
	"github.com/acaciolr/ora-brabo-studio/internal/ora"
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

// ---- banco de mentira para os prints (só responde o que as telas pedem) ----

type fakeDrv struct{}
type fakeConn struct{}
type fakeStmt struct{ q string }
type fakeRows struct {
	cols []string
	rows [][]driver.Value
	i    int
}

func (fakeDrv) Open(string) (driver.Conn, error)            { return fakeConn{}, nil }
func (fakeConn) Prepare(q string) (driver.Stmt, error)      { return fakeStmt{q}, nil }
func (fakeConn) Close() error                               { return nil }
func (fakeConn) Begin() (driver.Tx, error)                  { return fakeTx{}, nil }
func (fakeStmt) Close() error                               { return nil }
func (fakeStmt) NumInput() int                              { return -1 }
func (fakeStmt) Exec([]driver.Value) (driver.Result, error) { return driver.RowsAffected(0), nil }
func (s fakeStmt) Query([]driver.Value) (driver.Rows, error) {
	q := strings.ToLower(s.q)
	switch {
	case strings.Contains(q, "dba_tablespaces"):
		cols := []string{"TABLESPACE_NAME", "CONTENTS", "STATUS", "BIGFILE", "EXTENT_MANAGEMENT", "SSM", "ENCRYPTED", "ALLOC_MB", "USED_MB", "MAX_MB", "PCT_USED"}
		return &fakeRows{cols: cols, rows: [][]driver.Value{
			{"DATA_HR", "PERMANENT", "ONLINE", "NO", "LOCAL", "AUTO", "YES", "2048", "1532", "32768", "4.7"},
			{"SYSAUX", "PERMANENT", "ONLINE", "NO", "LOCAL", "AUTO", "NO", "1210", "1146", "32768", "3.5"},
			{"SYSTEM", "PERMANENT", "ONLINE", "NO", "LOCAL", "MANUAL", "NO", "1100", "1089", "32768", "3.3"},
			{"TEMP", "TEMPORARY", "ONLINE", "NO", "LOCAL", "MANUAL", "NO", "264", "", "", ""},
			{"UNDOTBS1", "UNDO", "ONLINE", "NO", "LOCAL", "MANUAL", "NO", "620", "38", "32768", "0.1"},
			{"USERS", "PERMANENT", "ONLINE", "NO", "LOCAL", "AUTO", "NO", "512", "301", "32768", "0.9"},
		}}, nil
	}
	return &fakeRows{cols: []string{"X"}, rows: [][]driver.Value{{"SYSTEM"}}}, nil
}

type fakeTx struct{}

func (fakeTx) Commit() error   { return nil }
func (fakeTx) Rollback() error { return nil }

func (r *fakeRows) Columns() []string { return r.cols }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(d []driver.Value) error {
	if r.i >= len(r.rows) {
		return io.EOF
	}
	for i, v := range r.rows[r.i] {
		if v == "" {
			v = nil
		}
		d[i] = v
	}
	r.i++
	return nil
}

func init() { sql.Register("orabrabo-shots", fakeDrv{}) }

func shotResult() ora.Result {
	cols := []string{"EMPLOYEE_ID", "FIRST_NAME", "LAST_NAME", "JOB_ID", "SALARY", "HIRE_DATE", "DEPARTMENT_NAME"}
	data := [][]string{
		{"100", "Steven", "King", "AD_PRES", "24000", "17/06/2013", "Executive"},
		{"101", "Neena", "Yang", "AD_VP", "17000", "21/09/2015", "Executive"},
		{"102", "Lex", "Garcia", "AD_VP", "17000", "13/01/2011", "Executive"},
		{"145", "John", "Singh", "SA_MAN", "14000", "01/10/2014", "Sales"},
		{"146", "Karen", "Partners", "SA_MAN", "13500", "05/01/2015", "Sales"},
		{"201", "Michael", "Martinez", "MK_MAN", "13000", "17/02/2014", "Marketing"},
		{"108", "Nancy", "Gruenberg", "FI_MGR", "12008", "17/08/2012", "Finance"},
		{"205", "Shelley", "Higgins", "AC_MGR", "12008", "07/06/2012", "Accounting"},
		{"147", "Alberto", "Errazuriz", "SA_MAN", "12000", "10/03/2015", "Sales"},
		{"168", "Lisa", "Ozer", "SA_REP", "11500", "11/03/2017", "Sales"},
	}
	r := ora.Result{Cols: cols, Elapsed: 38 * time.Millisecond}
	for _, d := range data {
		var row []ora.Cell
		for _, v := range d {
			row = append(row, ora.Cell{V: v})
		}
		r.Rows = append(r.Rows, row)
	}
	r.Stmt.Text = "select ... from hr.employees e join hr.departments d"
	return r
}

const shotSQL = `-- 10 maiores salários com o departamento
select e.employee_id, e.first_name, e.last_name, e.job_id,
       e.salary, e.hire_date, d.department_name
  from hr.employees e
  join hr.departments d on d.department_id = e.department_id
 order by e.salary desc
 fetch first 10 rows only;`

type shotInfo struct{ name, title string }

func TestShots(t *testing.T) {
	dir := os.Getenv("SHOTS_DIR")
	if dir == "" {
		t.Skip("SHOTS_DIR não definido")
	}
	_ = os.MkdirAll(dir, 0o755)
	for _, lang := range []string{"pt", "en"} {
		h := startLang(t, lang == "en")
		var shots []shotInfo
		shot := func(name, title string) {
			h.sync()
			time.Sleep(60 * time.Millisecond)
			h.sync()
			p := filepath.Join(dir, name+"-"+lang+".svg")
			if err := os.WriteFile(p, []byte(screenSVG(h.s, title)), 0o644); err != nil {
				t.Fatal(err)
			}
			shots = append(shots, shotInfo{name, title})
		}

		// conexões salvas + uma aberta no banco de mentira
		var c *Conn
		h.a.tv.QueueUpdateDraw(func() {
			h.a.CloseModal("connform")
			h.a.conns = append(h.a.conns, oraInfo("PROD · db01"), oraInfo("HML · db02"), oraInfo("lab-26ai"))
			h.a.conns[0].Color, h.a.conns[0].User, h.a.conns[0].Role = "red", "system", "SYSDBA"
			h.a.conns[0].Host, h.a.conns[0].Service = "db01.empresa.local", "PROD"
			h.a.conns[1].Host, h.a.conns[1].Service, h.a.conns[1].Color = "db02.empresa.local", "HML", "yellow"
			h.a.conns[2].Host, h.a.conns[2].Service, h.a.conns[2].User = "127.0.0.1", "FREEPDB1", "hr"
			h.a.buildConnTree()
			h.a.buildDBATree()
			h.a.ConnectDialog("PROD · db01")
		})
		shot("connect", "ORA BRABO Studio · "+L("Conectar", "Connect"))

		h.a.tv.QueueUpdateDraw(func() {
			h.a.CloseModal("connform")
			db, _ := sql.Open("orabrabo-shots", "")
			c = &Conn{Info: h.a.conns[0], DB: db, User: "SYSTEM", Version: "23.26.0.0.0", ConName: "FREEPDB1", packOK: map[string]bool{}}
			h.a.open[c.Info.Name] = c
			ch := h.a.dbaT.GetRoot().GetChildren()[0]
			ch.SetText(connLabel(c.Info.Name, true)).ClearChildren()
			for _, g := range catalog.DBA().Children {
				h.a.addDBAGroup(ch, c.Info.Name, g)
			}
			ch.SetExpanded(true)
			n := h.a.connT.GetRoot().GetChildren()[2]
			n.SetText(connLabel(c.Info.Name, true))
			h.a.fillConnNode(n, c)
			n.SetExpanded(true)
			h.a.connT.SetCurrentNode(n)
			w, err := NewWorksheet(h.a, c, shotSQL)
			if err != nil {
				t.Error(err)
				return
			}
			h.a.tabs.Add(w)
			w.showResult(shotResult(), "")
			w.appendOut("script", L("10 linhas selecionadas.\n", "10 rows selected.\n"))
			h.a.tv.SetFocus(w.ed)
		})
		shot("worksheet", "ORA BRABO Studio · Worksheet")

		h.a.tv.QueueUpdateDraw(func() { h.a.tv.SetFocus(h.a.connT) }) // no editor o F10 é o Plano
		h.key(tcell.KeyF10, 0, 0)
		for i := 0; i < 5; i++ {
			h.key(tcell.KeyRight, 0, 0) // Ferramentas
		}
		shot("menu", "ORA BRABO Studio · "+L("Menus (F10)", "Menus (F10)"))
		h.key(tcell.KeyEscape, 0, 0)

		h.a.tv.QueueUpdateDraw(func() {
			h.a.tv.SetFocus(h.a.connT)
			h.a.ContextMenu(h.a.connT, -1, -1)
		})
		shot("context", "ORA BRABO Studio · "+L("Clique direito", "Right click"))
		h.key(tcell.KeyEscape, 0, 0)

		h.a.tv.QueueUpdateDraw(func() {
			ch := h.a.dbaT.GetRoot().GetChildren()[0]
			for _, g := range ch.GetChildren() {
				g.SetExpanded(strings.Contains(g.GetText(), L("Armazenamento", "Storage")))
			}
			h.a.openViewID(c, "st.tablespaces")
			h.a.focusTree("dba")
		})
		time.Sleep(300 * time.Millisecond)
		h.a.tv.QueueUpdateDraw(func() {
			if t := h.a.tabs.Current(); t != nil {
				t.FocusPart(0)
			}
		})
		shot("dba", "ORA BRABO Studio · DBA")

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
		h.key(tcell.KeyEscape, 0, 0)

		h.a.tv.QueueUpdateDraw(func() { h.a.openHelp() })
		shot("help", "ORA BRABO Studio · F1")
		h.stop()

		renderShots(t, dir, lang, shots, "help", "action-form")
	}
	i18n.EN = false
}

// renderShots converte os SVG em PNG e junta os PNG (menos os de skip) no GIF.
func renderShots(t *testing.T, dir, lang string, shots []shotInfo, skip ...string) {
	if _, err := exec.LookPath("rsvg-convert"); err != nil {
		t.Log("rsvg-convert não encontrado: ficam só os SVG")
		return
	}
	var frames []string
	for _, s := range shots {
		svg := filepath.Join(dir, s.name+"-"+lang+".svg")
		out := strings.TrimSuffix(svg, ".svg") + ".png"
		if b, err := exec.Command("rsvg-convert", "-o", out, svg).CombinedOutput(); err != nil {
			t.Fatalf("rsvg-convert %s: %v %s", svg, err, b)
		}
		_ = os.Remove(svg)
		in := true
		for _, k := range skip {
			in = in && s.name != k
		}
		if in {
			frames = append(frames, out)
		}
	}
	if err := makeGIF(filepath.Join(dir, "demo-"+lang+".gif"), frames, 220); err != nil {
		t.Fatal(err)
	}
}

// makeGIF junta os PNG num GIF animado (paleta com as cores mais usadas: a
// tela do terminal tem poucas cores, então fica fiel).
func makeGIF(out string, pngs []string, delay int) error {
	var imgs []image.Image
	count := map[color.RGBA]int{}
	for _, p := range pngs {
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			return err
		}
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y += 2 {
			for x := b.Min.X; x < b.Max.X; x += 2 {
				r, g, bb, _ := img.At(x, y).RGBA()
				count[color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bb >> 8), 255}]++
			}
		}
		imgs = append(imgs, img)
	}
	type kv struct {
		c color.RGBA
		n int
	}
	var all []kv
	for c, n := range count {
		all = append(all, kv{c, n})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].n > all[j].n })
	var pal color.Palette
	for _, x := range all {
		if len(pal) == 256 {
			break
		}
		pal = append(pal, x.c)
	}
	cache := map[color.RGBA]uint8{}
	anim := &gif.GIF{}
	for _, img := range imgs {
		b := img.Bounds()
		pi := image.NewPaletted(b, pal)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, bb, _ := img.At(x, y).RGBA()
				c := color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bb >> 8), 255}
				i, ok := cache[c]
				if !ok {
					i = uint8(pal.Index(c))
					cache[c] = i
				}
				pi.SetColorIndex(x, y, i)
			}
		}
		anim.Image = append(anim.Image, pi)
		anim.Delay = append(anim.Delay, delay)
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	return gif.EncodeAll(f, anim)
}

func startLang(t *testing.T, en bool) *harness {
	t.Setenv("ORA_BRABO_STUDIO_HOME", t.TempDir())
	i18n.EN = en
	wsSeq = map[string]int{}
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, s: s, a: New(s), err: make(chan error, 1)}
	go func() { h.err <- h.a.tv.Run() }()
	h.wait(L("Conectar ao banco Oracle", "Connect to Oracle Database"))
	s.SetSize(150, 40)
	h.sync()
	return h
}
