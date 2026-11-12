// Testes da interface com a tela simulada do tcell (sem banco). by • DBA BRABO
package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/acaciolr/ora-brabo-studio/internal/catalog"
	"github.com/acaciolr/ora-brabo-studio/internal/i18n"
	"github.com/acaciolr/ora-brabo-studio/internal/ora"
)

func TestInline(t *testing.T) {
	q := `select * from x where a = :1 and b = nvl(:2, b) and c = ':1' and d = :1 and t = to_char(sysdate, 'hh24:mi:ss')`
	got := Inline(q, []any{"O'K", nil})
	want := `select * from x where a = 'O''K' and b = nvl(NULL, b) and c = ':1' and d = 'O''K' and t = to_char(sysdate, 'hh24:mi:ss')`
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
	if Inline("select :1x from dual", []any{"a"}) != "select 'a'x from dual" {
		// :1 seguido de letra ainda é a bind 1 (o Oracle também lê assim)
		t.Fatal(Inline("select :1x from dual", []any{"a"}))
	}
}

func screenText(s tcell.SimulationScreen) string {
	cells, w, h := s.GetContents()
	var b strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := cells[y*w+x]
			if len(c.Runes) > 0 {
				b.WriteRune(c.Runes[0])
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

type harness struct {
	t   *testing.T
	s   tcell.SimulationScreen
	a   *App
	err chan error
}

func start(t *testing.T) *harness {
	t.Setenv("ORA_BRABO_STUDIO_HOME", t.TempDir())
	i18n.EN = false
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	s.SetSize(160, 45)
	h := &harness{t: t, s: s, a: New(s), err: make(chan error, 1)}
	go func() { h.err <- h.a.tv.Run() }()
	h.wait("ORA BRABO")
	s.SetSize(160, 45) // o Run reinicia a tela em 80x25
	h.sync()
	return h
}

func (h *harness) sync() {
	done := make(chan struct{})
	h.a.tv.QueueUpdateDraw(func() { close(done) })
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		h.t.Fatal("interface travada")
	}
}

func (h *harness) wait(text string) string {
	h.t.Helper()
	for i := 0; i < 60; i++ {
		h.sync()
		if s := screenText(h.s); strings.Contains(s, text) {
			return s
		}
		time.Sleep(50 * time.Millisecond)
	}
	h.t.Fatalf("não apareceu %q na tela:\n%s", text, screenText(h.s))
	return ""
}

func (h *harness) key(k tcell.Key, r rune, m tcell.ModMask) {
	h.s.InjectKey(k, r, m)
	h.sync()
}

func (h *harness) typeText(s string) {
	for _, r := range s {
		h.key(tcell.KeyRune, r, 0)
	}
}

func (h *harness) stop() {
	h.a.tv.Stop()
	<-h.err
}

func TestUIFlow(t *testing.T) {
	h := start(t)
	defer h.stop()
	scr := h.wait("Bem-vindo")
	if !strings.Contains(strings.SplitN(scr, "\n", 2)[0], "⛁") {
		t.Fatalf("ícone verde não está no canto superior esquerdo:\n%s", strings.SplitN(scr, "\n", 2)[0])
	}
	h.wait("+ Nova conexão")

	// F1 ajuda; F1 de novo não duplica a aba
	h.key(tcell.KeyF1, 0, 0)
	h.wait("Navegador de objetos")
	h.key(tcell.KeyF1, 0, 0)
	if n := len(h.a.tabs.All()); n != 2 {
		t.Fatalf("esperava 2 abas (boas-vindas + ajuda), tem %d", n)
	}
	h.key(tcell.KeyCtrlW, 0, 0)

	// F2 vai para a aba DBA e volta
	h.key(tcell.KeyF2, 0, 0)
	h.wait("crie uma conexão")
	h.key(tcell.KeyF2, 0, 0)

	// nova conexão pelo formulário
	h.a.tv.QueueUpdateDraw(func() { h.a.connForm(nil) })
	h.wait("Nova conexão ")
	h.typeText("lab")
	h.key(tcell.KeyTab, 0, 0)
	h.typeText("system")
	for i := 0; i < 5; i++ { // senha, salvar, papel, tipo → host
		h.key(tcell.KeyTab, 0, 0)
	}
	h.typeText("127.0.0.1")
	h.key(tcell.KeyTab, 0, 0)
	h.key(tcell.KeyTab, 0, 0)
	h.typeText("FREEPDB1")
	h.a.tv.QueueUpdateDraw(func() {
		f := h.a.root
		_ = f
	})
	// botão Salvar: Tab até ele seria frágil; usa o form direto
	var saved bool
	h.a.tv.QueueUpdateDraw(func() {
		name, ok := h.a.root.GetFrontPage()
		if name != "connform" || ok == nil {
			return
		}
		saved = true
	})
	h.sync()
	if !saved {
		t.Fatal("formulário não está na frente")
	}
	// Enter no último campo não salva; aperta os botões com Tab
	for i := 0; i < 8; i++ { // SID, TNS, wallet, SSL, RMAN, cor, Testar → Salvar
		h.key(tcell.KeyTab, 0, 0)
	}
	h.key(tcell.KeyEnter, 0, 0) // Salvar
	scr = h.wait("Conexão salva")
	if !strings.Contains(scr, "○ lab") {
		t.Fatalf("conexão não apareceu na árvore:\n%s", scr)
	}
	if len(h.a.conns) != 1 || h.a.conns[0].Host != "127.0.0.1" || h.a.conns[0].Service != "FREEPDB1" || h.a.conns[0].User != "system" {
		t.Fatalf("conexão salva errada: %+v", h.a.conns)
	}

	// Ctrl+Q pergunta antes de sair
	h.key(tcell.KeyCtrlQ, 0, 0)
	h.wait("Sair do ORA BRABO Studio?")
	h.key(tcell.KeyRight, 0, 0)
	h.key(tcell.KeyEnter, 0, 0) // Não
	h.sync()
	if h.a.modals != 0 {
		t.Fatalf("modal ficou aberto: %d", h.a.modals)
	}
}

func TestEnglish(t *testing.T) {
	t.Setenv("ORA_BRABO_STUDIO_HOME", t.TempDir())
	i18n.EN = true
	defer func() { i18n.EN = false }()
	s := tcell.NewSimulationScreen("UTF-8")
	_ = s.Init()
	s.SetSize(160, 45)
	a := New(s)
	errc := make(chan error, 1)
	go func() { errc <- a.tv.Run() }()
	h := &harness{t: t, s: s, a: a, err: errc}
	defer h.stop()
	h.wait("ORA BRABO")
	s.SetSize(160, 45)
	h.wait("Welcome")
	h.wait("+ New connection")
}

func TestActionPreviewAndDanger(t *testing.T) {
	h := start(t)
	defer h.stop()
	c := &Conn{Info: oraInfo("lab"), User: "HR", packOK: map[string]bool{}}
	var ren, trunc *catalog.Action
	for _, x := range catalog.ObjectActions("TABLE") {
		switch x.Key {
		case 'm':
			ren = x
		case 'T':
			trunc = x
		}
	}
	row := catalog.ObjRow("HR", "EMPLOYEES", "TABLE")
	h.a.tv.QueueUpdateDraw(func() { h.a.RunAction(c, ren, row, nil) })
	h.wait("Novo nome")
	h.typeText("emp2")
	h.key(tcell.KeyTab, 0, 0)
	h.key(tcell.KeyEnter, 0, 0) // Continuar
	h.wait("ALTER TABLE HR.EMPLOYEES RENAME TO EMP2;")
	h.key(tcell.KeyEscape, 0, 0)
	h.sync()
	if h.a.modals != 0 {
		t.Fatalf("prévia não fechou: %d", h.a.modals)
	}

	// destrutiva: Ctrl+G na prévia pede para digitar o nome; errado não executa
	h.a.tv.QueueUpdateDraw(func() { h.a.RunAction(c, trunc, row, nil) })
	h.key(tcell.KeyTab, 0, 0)
	h.key(tcell.KeyEnter, 0, 0)
	h.wait("TRUNCATE TABLE HR.EMPLOYEES DROP STORAGE;")
	h.key(tcell.KeyCtrlG, 0, 0)
	h.wait("Digite EMPLOYEES para confirmar")
	h.typeText("EMPLOYEE")
	h.key(tcell.KeyEnter, 0, 0)
	h.wait("Não confere")
	if h.a.modals != 0 {
		t.Fatalf("confirmação não fechou: %d", h.a.modals)
	}
}

func oraInfo(name string) ora.ConnInfo { return ora.ConnInfo{Name: name, User: "hr", Host: "x", Port: 1521, Service: "s"} }
