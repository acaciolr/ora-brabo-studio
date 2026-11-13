// Prints do README com um Oracle de verdade (esquema LOJA de testdata/loja.sql,
// carga leve em segundo plano para as telas de sessões e Top Activity):
// SHOTS_DIR=$PWD/docs/img OBS_SHOTS_LIVE=1 OBS_TEST_PASSWORD=... go test -run TestShotsLive ./internal/ui/
// by • DBA BRABO
package ui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/acaciolr/ora-brabo-studio/internal/catalog"
	"github.com/acaciolr/ora-brabo-studio/internal/i18n"
	"github.com/acaciolr/ora-brabo-studio/internal/ora"
)

const lojaPwd = "Loja#2026x"

func liveSQL() string {
	return L("-- 10 clientes que mais compraram em 2025", "-- top 10 customers by 2025 purchases") + `
select c.nome, c.cidade, c.uf,
       count(distinct p.id_pedido) pedidos,
       sum(i.qtd * i.preco) total
  from loja.clientes c
  join loja.pedidos p on p.id_cliente = c.id_cliente
  join loja.itens_pedido i on i.id_pedido = p.id_pedido
 where p.status <> 'CANCELADO'
   and p.data_ped >= date '2025-01-01'
 group by c.nome, c.cidade, c.uf
 order by total desc
 fetch first 10 rows only;`
}

// waitFor espera o texto na tela (consultas reais demoram mais que a simulação).
func (h *harness) waitFor(text string, d time.Duration) {
	h.t.Helper()
	for t0 := time.Now(); time.Since(t0) < d; time.Sleep(100 * time.Millisecond) {
		h.sync()
		if strings.Contains(screenText(h.s), text) {
			return
		}
	}
	h.t.Fatalf("não apareceu %q na tela:\n%s", text, screenText(h.s))
}

func TestShotsLive(t *testing.T) {
	dir := os.Getenv("SHOTS_DIR")
	pw := os.Getenv("OBS_TEST_PASSWORD")
	if dir == "" || pw == "" || os.Getenv("OBS_SHOTS_LIVE") == "" {
		t.Skip("SHOTS_DIR, OBS_SHOTS_LIVE e OBS_TEST_PASSWORD (banco de teste)")
	}
	_ = os.MkdirAll(dir, 0o755)
	sys := liveInfo("FREEPDB1")
	sys.Password = pw
	sdb, err := ora.Open(sys)
	if err != nil {
		t.Fatal(err)
	}
	defer ora.Close(sdb)
	var n int
	_ = sdb.QueryRow("select count(*) from dba_users where username = 'LOJA'").Scan(&n)
	if n == 0 || os.Getenv("OBS_LOJA_RECREATE") != "" {
		b, err := os.ReadFile(filepath.Join("testdata", "loja.sql"))
		if err != nil {
			t.Fatal(err)
		}
		if out, fails, _ := runSQL(&Conn{Info: sys, DB: sdb}, string(b)); fails > 0 {
			t.Fatalf("loja.sql: %s", out)
		}
	}

	loja := ora.ConnInfo{Name: "lab-26ai · loja", User: "loja", Password: lojaPwd, Host: "127.0.0.1", Port: 15210, Service: "FREEPDB1", Color: "green"}
	cdb := liveInfo("FREE")
	cdb.Name, cdb.Password, cdb.Color = "lab-26ai · CDB (sys)", pw, "red"

	// carga leve: duas sessões repetindo consultas da loja (para sessões e Top Activity)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	load := func() {
		for i := 0; i < 3; i++ {
			db, err := ora.Open(loja)
			if err != nil {
				t.Fatal(err)
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer ora.Close(db)
				for {
					select {
					case <-stop:
						return
					default:
					}
					var x float64
					_ = db.QueryRow(`select sum(i.qtd * i.preco) from loja.pedidos p join loja.itens_pedido i on i.id_pedido = p.id_pedido
					  join loja.clientes c on c.id_cliente = p.id_cliente where c.uf = :1`, []string{"SP", "RJ", "MG", "PR"}[time.Now().Second()%4]).Scan(&x)
					_, _ = db.Exec("update loja.pedidos set status = status where id_pedido = :1", time.Now().Nanosecond()%30000+1)
					time.Sleep(20 * time.Millisecond)
				}
			}()
		}
	}
	load()
	// um bloqueio de verdade para a tela de bloqueios: A segura a linha, B espera
	blkA, err := ora.Open(loja)
	if err != nil {
		t.Fatal(err)
	}
	txA, err := blkA.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := txA.Exec("update loja.pedidos set status = 'EM ROTA' where id_pedido = 1"); err != nil {
		t.Fatal(err)
	}
	blkB, err := ora.Open(loja)
	if err != nil {
		t.Fatal(err)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = blkB.Exec("update loja.pedidos set status = 'ENTREGUE' where id_pedido = 1")
	}()
	defer func() {
		_ = txA.Rollback() // solta o B antes do wg.Wait
		close(stop)
		wg.Wait()
		ora.Close(blkA)
		ora.Close(blkB)
	}()

	for _, lang := range []string{"pt", "en"} {
		h := startLang(t, lang == "en")
		h.s.SetSize(160, 42)
		var shots []shotInfo
		shot := func(name, title string) {
			h.sync()
			time.Sleep(150 * time.Millisecond)
			h.sync()
			p := filepath.Join(dir, name+"-"+lang+".svg")
			if err := os.WriteFile(p, []byte(screenSVG(h.s, title)), 0o644); err != nil {
				t.Fatal(err)
			}
			shots = append(shots, shotInfo{name, title})
		}
		ui := func(f func()) {
			done := make(chan struct{})
			h.a.tv.QueueUpdateDraw(func() { f(); close(done) })
			<-done
		}

		ui(func() {
			h.a.CloseModal("connform")
			h.a.conns = append(h.a.conns, loja, cdb)
			h.a.buildConnTree()
			h.a.buildDBATree()
			h.a.ConnectDialog(loja.Name)
		})
		shot("connect", "ORA BRABO Studio · "+L("Conectar", "Connect"))

		ui(func() {
			h.a.CloseModal("connform")
			h.a.Connect(loja.Name, func(*Conn) {})
		})
		var c *Conn
		for t0 := time.Now(); c == nil && time.Since(t0) < 30*time.Second; time.Sleep(100 * time.Millisecond) {
			c = h.a.Connected(loja.Name)
		}
		if c == nil {
			t.Fatal("não conectou na LOJA")
		}
		h.sync()
		time.Sleep(500 * time.Millisecond)

		// navegador de objetos: Tabelas aberta
		ui(func() {
			var cn *tview.TreeNode
			for _, ch := range h.a.connT.GetRoot().GetChildren() {
				if r, ok := ch.GetReference().(*nodeRef); ok && r.kind == nkConn && r.conn == loja.Name {
					cn = ch
				}
			}
			cn.SetText(connLabel(c.Info.Name, true))
			h.a.fillConnNode(cn, c)
			cn.SetExpanded(true)
			for _, ch := range cn.GetChildren() {
				if r, ok := ch.GetReference().(*nodeRef); ok && r.kind == nkCategory && r.cat.Title == L("Tabelas", "Tables") {
					h.a.loadChildren(ch, r)
					h.a.connT.SetCurrentNode(ch)
				}
			}
		})
		h.waitFor("ITENS_PEDIDO", 20*time.Second)

		var w *Worksheet
		ui(func() {
			var err error
			if w, err = NewWorksheet(h.a, c, liveSQL()); err != nil {
				t.Error(err)
				return
			}
			h.a.tabs.Add(w)
			h.a.tv.SetFocus(w.ed)
			w.runCurrent(false)
		})
		h.waitFor(L("10 linha", "10 row"), 60*time.Second)
		shot("worksheet", "ORA BRABO Studio · Worksheet")

		ui(func() { w.explain() })
		h.waitFor("Plan hash value", 60*time.Second)
		shot("explain", "ORA BRABO Studio · "+L("Plano de execução", "Explain plan"))

		ui(func() { w.resize(-3); w.runCurrent(true) }) // editor menor: cabe o plano inteiro
		h.waitFor("A-Rows", 90*time.Second)
		ui(func() {
			if tv := w.texts["trace"]; tv != nil {
				tv.ScrollTo(6, 0) // pula o texto do SQL e mostra o plano
			}
		})
		shot("autotrace", "ORA BRABO Studio · Autotrace")
		ui(func() { w.resize(3) })

		ui(func() { h.a.openObject(c, "LOJA", "PEDIDOS", "TABLE") })
		h.waitFor("DATA_PED", 30*time.Second)
		shot("object", "ORA BRABO Studio · "+L("Objeto", "Object"))

		ui(func() { h.a.tv.SetFocus(h.a.connT) }) // no editor o F10 é o Plano
		h.key(tcell.KeyF10, 0, 0)
		for i := 0; i < 5; i++ {
			h.key(tcell.KeyRight, 0, 0) // Ferramentas
		}
		shot("menu", "ORA BRABO Studio · "+L("Menus (F10)", "Menus (F10)"))
		h.key(tcell.KeyEscape, 0, 0)

		ui(func() {
			h.a.tv.SetFocus(h.a.connT)
			h.a.connT.SetCurrentNode(h.a.connT.GetRoot().GetChildren()[2])
			h.a.ContextMenu(h.a.connT, -1, -1)
		})
		shot("context", "ORA BRABO Studio · "+L("Clique direito", "Right click"))
		h.key(tcell.KeyEscape, 0, 0)

		// aba DBA: árvore da LOJA aberta em Armazenamento
		dbaOpen := func(group, id, wait string) {
			ui(func() {
				ch := h.a.dbaT.GetRoot().GetChildren()[0]
				if len(ch.GetChildren()) == 0 {
					for _, g := range catalog.DBA().Children {
						h.a.addDBAGroup(ch, c.Info.Name, g)
					}
				}
				ch.SetExpanded(true)
				for _, g := range ch.GetChildren() {
					open := strings.Contains(g.GetText(), group)
					g.SetExpanded(open)
					if open {
						for _, v := range g.GetChildren() {
							if r, ok := v.GetReference().(*nodeRef); ok && r.view != nil && r.view.ID == id {
								h.a.dbaT.SetCurrentNode(v)
							}
						}
					}
				}
				h.a.openViewID(c, id)
			})
			h.waitFor(wait, 30*time.Second)
			ui(func() {
				if t := h.a.tabs.Current(); t != nil {
					t.FocusPart(0)
				}
			})
		}
		rows := L(" linha(s)", " row(s)")
		dbaOpen(L("Armazenamento", "Storage"), "st.tablespaces", rows)
		shot("dba", "ORA BRABO Studio · DBA")
		dbaOpen(L("Monitoramento", "Monitoring"), "mo.sessions", rows)
		shot("sessions", "ORA BRABO Studio · DBA · "+L("Sessões", "Sessions"))
		dbaOpen(L("Banco", "Database"), "db.instance", rows)
		shot("instance", "ORA BRABO Studio · DBA · "+L("Instância", "Instance"))
		dbaOpen(L("Banco", "Database"), "db.topactivity", "Top Activity")
		time.Sleep(30 * time.Second) // o gráfico enche com as amostras
		shot("topactivity", "ORA BRABO Studio · Top Activity")
		dbaOpen(L("Banco", "Database"), "db.panel", L("Memória", "Memory"))
		time.Sleep(6 * time.Second) // segunda amostra: taxas por segundo
		shot("panel", "ORA BRABO Studio · DBA · "+L("Painel da instância", "Instance viewer"))
		dbaOpen(L("Monitoramento", "Monitoring"), "mo.blockers", rows)
		shot("blockers", "ORA BRABO Studio · DBA · "+L("Bloqueios", "Blocking"))
		dbaOpen(L("Monitoramento", "Monitoring"), "mo.alert", rows)
		shot("alert", "ORA BRABO Studio · DBA · Alert log")
		dbaOpen(L("Segurança", "Security"), "sec.users", rows)
		shot("users", "ORA BRABO Studio · DBA · "+L("Usuários", "Users"))
		dbaOpen("Backup", "bk.sets", rows)
		shot("backups", "ORA BRABO Studio · DBA · RMAN")
		c.packOK["diag"] = true // o aviso de licença já foi aceito (lab)
		dbaOpen("Performance", "pf.snaps", rows)
		shot("awr", "ORA BRABO Studio · DBA · AWR")
		dbaOpen("Resource Manager", "rm.plans", rows)
		shot("rsrc", "ORA BRABO Studio · DBA · Resource Manager")
		dbaOpen("Scheduler", "sc.jobs", rows)
		shot("scheduler", "ORA BRABO Studio · DBA · Scheduler")

		// ação com a linha real do datafile da USERS
		var x *catalog.Action
		catalog.Walk(catalog.DBA(), func(_ []string, n *catalog.Node) {
			if n.View != nil && n.View.ID == "st.datafiles" {
				x = n.View.FindAction('z')
			}
		})
		var row catalog.Row
		r := Query(c, `select file_id, file_name, tablespace_name, round(bytes/1048576) mb, autoextensible
		  from dba_data_files where tablespace_name = 'USERS' fetch first 1 rows only`, nil, 1)
		if r.Err != nil || len(r.Rows) == 0 {
			t.Fatalf("datafile da USERS: %v", r.Err)
		}
		row = catalog.Row{}
		for i, col := range r.Cols {
			row[strings.ToUpper(col)] = r.Rows[0][i].V
		}
		ui(func() { h.a.askFields(x, row, func(catalog.Vals) {}) })
		shot("action-form", "ORA BRABO Studio · DBA")
		ui(func() {
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

		ui(func() { h.a.openHelp() })
		shot("help", "ORA BRABO Studio · F1")
		ui(func() {
			for _, cc := range h.a.open {
				ora.Close(cc.DB)
			}
		})
		h.stop()
		renderShots(t, dir, lang, shots, "help", "action-form", "instance", "alert", "users", "rsrc", "scheduler")
	}
	i18n.EN = false
}
