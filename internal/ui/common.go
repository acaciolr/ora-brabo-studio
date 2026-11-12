// Peças comuns das abas: base, aba de texto, consulta com argumentos e
// gravação de arquivos. by • DBA BRABO
package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/acaciolr/ora-brabo-studio/internal/catalog"
	"github.com/acaciolr/ora-brabo-studio/internal/ora"
)

// baseTab traz o básico de Tab.
type baseTab struct {
	a     *App
	c     *Conn
	title string
	main  tview.Primitive
}

func (b *baseTab) Title() string { return b.title }
func (b *baseTab) Conn() *Conn   { return b.c }
func (b *baseTab) Hint() string  { return "" }
func (b *baseTab) Close()        {}
func (b *baseTab) FocusPart(step int) bool {
	if step == 0 && b.main != nil {
		b.a.tv.SetFocus(b.main)
		return true
	}
	return false
}

// Inline troca :1, :2... pelos valores como literais (nil = NULL). As telas do
// catálogo repetem a mesma bind várias vezes; assim não depende do driver.
func Inline(q string, args []any) string {
	if len(args) == 0 {
		return q
	}
	var b strings.Builder
	for i := 0; i < len(q); i++ {
		ch := q[i]
		if ch == '\'' {
			j := i + 1
			for j < len(q) {
				if q[j] == '\'' {
					if j+1 < len(q) && q[j+1] == '\'' {
						j += 2
						continue
					}
					break
				}
				j++
			}
			if j >= len(q) {
				j = len(q) - 1
			}
			b.WriteString(q[i : j+1])
			i = j
			continue
		}
		if ch == ':' && i+1 < len(q) && q[i+1] >= '1' && q[i+1] <= '9' && (i == 0 || !isWord(q[i-1])) {
			j := i + 1
			for j < len(q) && q[j] >= '0' && q[j] <= '9' {
				j++
			}
			n, _ := strconv.Atoi(q[i+1 : j])
			if n >= 1 && n <= len(args) {
				b.WriteString(lit(args[n-1]))
				i = j - 1
				continue
			}
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func isWord(c byte) bool {
	return c == '_' || c == '$' || c == '#' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func lit(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case string:
		if x == "" {
			return "NULL"
		}
		return catalog.Lit(x)
	case int, int64, float64:
		return fmt.Sprint(x)
	}
	return catalog.Lit(fmt.Sprint(v))
}

// Query roda uma consulta (com argumentos inline) no pool da conexão.
func Query(c *Conn, q string, args []any, limit int) ora.Result {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	t0 := time.Now()
	rows, err := c.DB.QueryContext(ctx, Inline(q, args))
	if err != nil {
		return ora.Result{Err: err, Elapsed: time.Since(t0)}
	}
	defer rows.Close()
	r := ora.Collect(rows, limit)
	r.Elapsed = time.Since(t0)
	return r
}

// TextOf junta a primeira coluna de todas as linhas (DDL, código, relatórios).
func TextOf(r ora.Result) string {
	var b strings.Builder
	for _, row := range r.Rows {
		if len(row) == 0 || row[0].Null {
			continue
		}
		s := row[0].V
		b.WriteString(s)
		if !strings.HasSuffix(s, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// ---- aba de texto ----

// TextTab mostra um texto (relatório, DDL, saída do RMAN).
type TextTab struct {
	*tview.TextView
	baseTab
	text string
	wrap bool
}

// NewTextTab cria a aba.
func NewTextTab(a *App, c *Conn, title, text string) *TextTab {
	t := &TextTab{TextView: tview.NewTextView().SetScrollable(true).SetWrap(false), text: text}
	t.baseTab = baseTab{a: a, c: c, title: title, main: t.TextView}
	t.SetText(text)
	a.SetContext(t.TextView, func() []menuItem {
		return []menuItem{
			item(L("Copiar tudo", "Copy all"), "y", func() { Copy(t.text); a.Msg(L("Texto copiado.", "Text copied.")) }),
			item(L("Salvar em arquivo…", "Save to file…"), "s", func() { a.SaveFile(sanitize(title)+".txt", t.text) }),
			itemIf(c != nil, L("Abrir numa worksheet", "Open in a worksheet"), "w", func() {
				if w, err := NewWorksheet(a, c, t.text); err == nil {
					a.tabs.Add(w)
				}
			}),
			item(L("Quebrar linhas", "Wrap lines"), "W", func() { t.wrap = !t.wrap; t.SetWrap(t.wrap) }),
		}
	})
	a.mouseText(t.TextView)
	t.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		switch ev.Rune() {
		case 'y':
			Copy(t.text)
			a.Msg(L("Texto copiado.", "Text copied."))
			return nil
		case 's':
			a.SaveFile(sanitize(title)+".txt", t.text)
			return nil
		case 'w':
			if c != nil {
				a.Connect(c.Info.Name, func(c *Conn) {
					if w, err := NewWorksheet(a, c, t.text); err == nil {
						a.tabs.Add(w)
					}
				})
			}
			return nil
		case 'W':
			t.wrap = !t.wrap
			t.SetWrap(t.wrap)
			return nil
		}
		return ev
	})
	return t
}

// SetContent troca o texto.
func (t *TextTab) SetContent(s string) {
	t.text = s
	t.SetText(s)
}

// Append acrescenta texto (saída do RMAN chegando aos poucos).
func (t *TextTab) Append(s string) {
	t.text += s
	fmt.Fprint(t.TextView, tview.Escape(s))
	t.ScrollToEnd()
}

func (t *TextTab) Hint() string {
	return L("[yellow]y[-] copiar  [yellow]s[-] salvar em arquivo  [yellow]w[-] abrir numa worksheet  [yellow]↑↓ PgUp PgDn[-] rolar  [yellow]Ctrl+W[-] fechar",
		"[yellow]y[-] copy  [yellow]s[-] save to file  [yellow]w[-] open in a worksheet  [yellow]↑↓ PgUp PgDn[-] scroll  [yellow]Ctrl+W[-] close")
}

func sanitize(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == ' ' || r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '_'
		}
		return r
	}, s)
	if s == "" {
		s = "saida"
	}
	return s
}

// SaveFile pergunta o caminho e grava.
func (a *App) SaveFile(def, content string) {
	home, _ := os.UserHomeDir()
	f := tview.NewForm()
	f.SetBorder(true).SetTitle(" " + L("Salvar em arquivo", "Save to file") + " ")
	f.AddInputField(L("Arquivo", "File"), filepath.Join(home, def), 70, nil, nil)
	f.AddButton(L("Salvar", "Save"), func() {
		p := strings.TrimSpace(f.GetFormItem(0).(*tview.InputField).GetText())
		if strings.HasPrefix(p, "~/") {
			p = filepath.Join(home, p[2:])
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			a.Errf("%v", err)
			return
		}
		a.CloseModal("save")
		a.Msg("[green]" + L("Gravado: ", "Saved: ") + tview.Escape(p) + "[-]")
	})
	f.AddButton(L("Cancelar", "Cancel"), func() { a.CloseModal("save") })
	f.SetCancelFunc(func() { a.CloseModal("save") })
	a.Modal("save", f, 90, 7)
}

// openText roda um script (blocos + consulta final com argumentos) e mostra o
// texto da última consulta numa aba.
func (a *App) openText(c *Conn, title, script string, args []any) {
	t := NewTextTab(a, c, title, L("executando…", "running…"))
	a.tabs.Add(t)
	a.Go(func() func() {
		txt, err := RunReport(c, script, args)
		return func() {
			if err != nil {
				t.SetContent(err.Error())
				return
			}
			t.SetContent(txt)
		}
	})
}

// RunReport roda os comandos do script numa sessão própria; o texto é a saída
// (DBMS_OUTPUT) mais a primeira coluna da última consulta.
func RunReport(c *Conn, script string, args []any) (string, error) {
	s, err := ora.NewSession(c.DB)
	if err != nil {
		return "", err
	}
	defer s.Close()
	s.AutoCommit = true
	s.FetchLimit = 1 << 30
	var out strings.Builder
	for _, st := range ora.Split(script) {
		st.Text = Inline(st.Text, args)
		r := s.Run(st, nil)
		for _, l := range r.Output {
			out.WriteString(l + "\n")
		}
		if r.Err != nil {
			return out.String(), fmt.Errorf("%s\n\n%v", strings.TrimSpace(st.Text), r.Err)
		}
		if r.Text != "" {
			out.WriteString(r.Text)
		}
		if st.Kind == ora.KindQuery {
			out.WriteString(TextOf(r))
		}
	}
	return out.String(), nil
}
