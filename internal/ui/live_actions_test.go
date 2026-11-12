// Teste ao vivo das ações (aba DBA e objetos) contra um Oracle de teste:
// cada ação usa uma linha real da tela, monta o comando como a interface
// monta e executa pelo mesmo caminho (SQL, relatório, grade ou RMAN).
// Rode com OBS_ACTIONS=1 e OBS_TEST_PASSWORD (banco descartável!). Ao fim
// mostra o que passou, o que falhou e o que ficou sem teste (com o motivo).
// by • DBA BRABO
package ui

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/acaciolr/ora-brabo-studio/internal/catalog"
	"github.com/acaciolr/ora-brabo-studio/internal/ora"
)

type liveH struct {
	t     *testing.T
	pdb   *Conn // sys as sysdba no FREEPDB1
	cdb   *Conn // sys as sysdba no CDB$ROOT (com RMAN)
	views map[string]*catalog.View
	mu    sync.Mutex
	done  map[string]string // "view key" -> ok / erro
	skips map[string]string // "view key" -> motivo
	fails []string
	logf  *os.File
}

func liveInfo(svc string) ora.ConnInfo {
	return ora.ConnInfo{Name: "obs-" + svc, User: "sys", Password: os.Getenv("OBS_TEST_PASSWORD"), Role: "SYSDBA",
		Host: "127.0.0.1", Port: 15210, Service: svc, RmanCmd: "docker exec -i obs-oracle-free rman target /"}
}

func (h *liveH) open(svc string) *Conn {
	info := liveInfo(svc)
	var db, err = ora.Open(info)
	for i := 0; err != nil && i < 40; i++ { // depois de reiniciar o banco
		time.Sleep(5 * time.Second)
		db, err = ora.Open(info)
	}
	if err != nil {
		h.t.Fatalf("conectar %s: %v", svc, err)
	}
	return &Conn{Info: info, DB: db, packOK: map[string]bool{"diag": true, "tuning": true}}
}

func (h *liveH) reconnect() {
	ora.Close(h.pdb.DB)
	ora.Close(h.cdb.DB)
	h.cdb = h.open("FREE")
	h.sql(h.cdb, "alter pluggable database all open;")
	h.pdb = h.open("FREEPDB1")
}

func (h *liveH) isEE() bool {
	r := Query(h.cdb, "select count(*) from v$version where banner like '%Enterprise%'", nil, 1)
	return len(r.Rows) > 0 && r.Rows[0][0].V != "0"
}

func (h *liveH) log(format string, a ...any) {
	s := fmt.Sprintf(format, a...)
	fmt.Fprintln(h.logf, s)
	h.t.Log(s)
}

// sql roda um script de preparo (erros só no log).
func (h *liveH) sql(c *Conn, script string) string {
	out, fails, _ := runSQL(c, script)
	if fails > 0 {
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, "ORA-") || strings.Contains(l, "PLS-") {
				h.log("  preparo com erro: %s", strings.TrimSpace(l))
			}
		}
	}
	return out
}

func (h *liveH) rman(c *Conn, script string) {
	cmd := exec.Command("sh", "-c", c.Info.RmanCmd)
	cmd.Stdin = strings.NewReader(script + "\nexit\n")
	if b, err := cmd.CombinedOutput(); err != nil {
		h.log("  preparo rman: %v %s", err, lastLines(string(b), 3))
	}
}

func (h *liveH) view(id string) *catalog.View {
	v := h.views[id]
	if v == nil {
		h.t.Fatalf("tela %s não existe", id)
	}
	return v
}

// rows lê a tela (com os argumentos dela).
func (h *liveH) rows(c *Conn, v *catalog.View) ([]catalog.Row, error) {
	r := Query(c, v.SQL, v.Args, 5000) // o mesmo limite da tela
	if r.Err != nil {
		return nil, r.Err
	}
	if v.Special == "record" {
		r = vertical(r)
		m := catalog.Row{}
		for _, x := range r.Rows {
			if !x[1].Null {
				m[strings.ToUpper(x[0].V)] = x[1].V
			}
		}
		return []catalog.Row{m}, nil
	}
	var out []catalog.Row
	for _, x := range r.Rows {
		m := catalog.Row{}
		for i, col := range r.Cols {
			if !x[i].Null {
				m[strings.ToUpper(col)] = x[i].V
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// pick acha a linha: cada valor de where tem de bater (valor ~x ou chave ~COL =
// contém, sem caixa; chave ~COL com valor vazio = coluna preenchida).
func (h *liveH) pick(c *Conn, v *catalog.View, where map[string]string) (catalog.Row, error) {
	rs, err := h.rows(c, v)
	if err != nil {
		return nil, err
	}
next:
	for _, r := range rs {
		for k, want := range where {
			if strings.HasPrefix(k, "~") { // ~COL: contém; vazio = não vazio
				got := r[strings.ToUpper(k[1:])]
				if got == "" || !strings.Contains(strings.ToUpper(got), strings.ToUpper(want)) {
					continue next
				}
				continue
			}
			got := r[strings.ToUpper(k)]
			if strings.HasPrefix(want, "~") {
				if !strings.Contains(strings.ToUpper(got), strings.ToUpper(want[1:])) {
					continue next
				}
			} else if !strings.EqualFold(got, want) {
				continue next
			}
		}
		return r, nil
	}
	return nil, fmt.Errorf("nenhuma linha em %s com %v (%d linhas)", v.ID, where, len(rs))
}

type stepOpt struct {
	where map[string]string
	vals  map[string]string
	args  []any // argumentos da tela (drill)
}

func W(kv ...string) map[string]string {
	m := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	return m
}

// do executa a ação key da tela id.
func (h *liveH) do(c *Conn, id string, key rune, o stepOpt) bool {
	v := h.view(id)
	if o.args != nil {
		v = v.Clone(v.Title, o.args)
	}
	x := v.FindAction(key)
	name := fmt.Sprintf("%s %c", id, key)
	if x == nil {
		h.fail(name, "ação não existe")
		return false
	}
	var row catalog.Row
	if x.NeedRow || o.where != nil {
		r, err := h.pick(c, v, o.where)
		if err != nil {
			h.fail(name+" ("+x.Name+")", err.Error())
			return false
		}
		row = r
	} else {
		row = catalog.Row{}
	}
	return h.run(c, name, x, row, o.vals)
}

// obj executa a ação key de um objeto.
func (h *liveH) obj(c *Conn, owner, objName, typ string, key rune, vals map[string]string) bool {
	name := fmt.Sprintf("obj %s %c", typ, key)
	for _, x := range catalog.ObjectActions(typ) {
		if x.Key == key {
			return h.run(c, name, x, catalog.ObjRow(owner, objName, typ), vals)
		}
	}
	h.fail(name, "ação não existe")
	return false
}

func (h *liveH) run(c *Conn, name string, x *catalog.Action, row catalog.Row, over map[string]string) bool {
	t0 := time.Now()
	if x.Drill != nil {
		args := []any{}
		if x.DrillArgs != nil {
			args = x.DrillArgs(row)
		}
		r := Query(c, x.Drill.SQL, args, 100)
		if r.Err != nil {
			h.fail(name+" ("+x.Name+")", r.Err.Error())
			return false
		}
		h.ok(name, x.Name, fmt.Sprintf("%d linha(s)", len(r.Rows)), t0)
		return true
	}
	vals := catalog.Vals{}
	for _, f := range x.Fields {
		vals[f.Name] = catalog.Fill(f.Default, row)
	}
	for k, v := range over {
		vals[k] = v
	}
	for _, f := range x.Fields { // a tela não deixa seguir com obrigatório vazio
		if f.Required && strings.TrimSpace(vals[f.Name]) == "" {
			h.fail(name+" ("+x.Name+")", "teste sem o campo obrigatório "+f.Name)
			return false
		}
	}
	sc, err := x.Build(row, vals)
	if err != nil {
		h.fail(name+" ("+x.Name+")", "build: "+err.Error())
		return false
	}
	var out string
	switch sc.Kind {
	case catalog.RMAN:
		cmd := exec.Command("sh", "-c", c.Info.RmanCmd)
		cmd.Stdin = strings.NewReader(strings.TrimSpace(sc.Text) + "\nexit\n")
		b, err := cmd.CombinedOutput()
		out = string(b)
		if strings.Contains(out, "RMAN-06025") && x.Key == 'T' { // achou archive sem backup: é o que o teste de restore deve fazer
			h.ok(name, x.Name, "acusou archive sem backup (encarnação antiga do banco de teste)", t0)
			return true
		}
		if err != nil || strings.Contains(out, "RMAN-00569") {
			h.fail(name+" ("+x.Name+")", rmanErr(out)+"\n      script: "+oneLine(sc.Text))
			return false
		}
	case catalog.Report:
		txt, err := RunReport(c, sc.Text, nil)
		if err != nil {
			h.fail(name+" ("+x.Name+")", firstLine(err.Error())+" | "+lastErr(err.Error())+"\n      script: "+oneLine(sc.Text))
			return false
		}
		out = fmt.Sprintf("%d bytes", len(txt))
	case catalog.Grid:
		r := Query(c, sc.Text, nil, 100)
		if r.Err != nil {
			h.fail(name+" ("+x.Name+")", r.Err.Error()+"\n      script: "+oneLine(sc.Text))
			return false
		}
		out = fmt.Sprintf("%d linha(s)", len(r.Rows))
	default:
		o, fails, _ := runSQL(c, sc.Text)
		if fails > 0 && strings.Contains(o, "ORA-00031") && strings.Count(o, "ORA-") == 1 {
			fails, o = 0, "" // sessão no meio de uma chamada: o banco marca e mata quando ela voltar
			out = "marcada para kill (ORA-00031)"
		}
		if fails > 0 {
			h.fail(name+" ("+x.Name+")", lastErr(o)+"\n      script: "+oneLine(sc.Text))
			return false
		}
		if out == "" {
			out = "ok"
		}
	}
	h.ok(name, x.Name, out, t0)
	return true
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

func lastErr(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, "ORA-") || strings.Contains(l, "PLS-") || strings.Contains(l, "SP2-") {
			return strings.TrimSpace(l)
		}
	}
	return firstLine(strings.TrimSpace(s))
}

func rmanErr(s string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, "RMAN-") && !strings.Contains(l, "RMAN-00569") && !strings.Contains(l, "RMAN-00571") || strings.Contains(l, "ORA-") {
			out = append(out, strings.TrimSpace(l))
		}
	}
	if len(out) > 4 {
		out = out[:4]
	}
	if len(out) == 0 {
		return lastLines(s, 3)
	}
	return strings.Join(out, " | ")
}

func lastLines(s string, n int) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	if len(l) > n {
		l = l[len(l)-n:]
	}
	return strings.Join(l, " | ")
}

func (h *liveH) ok(name, title, out string, t0 time.Time) {
	h.mu.Lock()
	h.done[name] = "ok"
	h.mu.Unlock()
	h.log("ok    %-22s %-45s %s (%s)", name, title, out, time.Since(t0).Round(time.Millisecond))
}

func (h *liveH) fail(name, msg string) {
	h.mu.Lock()
	h.done[strings.SplitN(name, " (", 2)[0]] = "erro"
	h.fails = append(h.fails, name+": "+msg)
	h.mu.Unlock()
	h.log("ERRO  %s\n      %s", name, msg)
}

func (h *liveH) skip(name, why string) {
	h.skips[name] = why
}

// detalhes: as abas do visualizador e a pasta do navegador para o tipo.
func (h *liveH) details(c *Conn, owner, objName, typ string) {
	for _, d := range catalog.Details(typ) {
		name := "detalhe " + typ + " · " + d.Title
		var err error
		if d.Data {
			continue
		}
		if d.Text && strings.Contains(d.SQL, "dbms_metadata") {
			_, err = RunReport(c, catalog.DDLSetup()+"\n/\n"+d.SQL, []any{owner, objName})
		} else {
			err = Query(c, d.SQL, []any{owner, objName}, 100).Err
		}
		if err != nil {
			h.fail(name, firstLine(err.Error()))
		} else {
			h.mu.Lock()
			h.done[name] = "ok"
			h.mu.Unlock()
		}
	}
}

func TestLiveActions(t *testing.T) {
	if os.Getenv("OBS_ACTIONS") == "" || os.Getenv("OBS_TEST_PASSWORD") == "" {
		t.Skip("OBS_ACTIONS e OBS_TEST_PASSWORD (banco de teste descartável)")
	}
	t.Setenv("ORA_BRABO_STUDIO_HOME", t.TempDir())
	lf, _ := os.Create(os.Getenv("OBS_ACTIONS_LOG"))
	if lf == nil {
		lf, _ = os.Create(os.DevNull)
	}
	defer lf.Close()
	h := &liveH{t: t, views: map[string]*catalog.View{}, done: map[string]string{}, skips: map[string]string{}, logf: lf}
	catalog.Walk(catalog.DBA(), func(_ []string, n *catalog.Node) {
		if n.View != nil {
			h.views[n.View.ID] = n.View
		}
	})
	h.cdb = h.open("FREE")
	h.pdb = h.open("FREEPDB1")
	defer func() { ora.Close(h.pdb.DB); ora.Close(h.cdb.DB) }()

	only := os.Getenv("OBS_ONLY") // ex.: "obj,sec" roda só essas partes
	part := func(n string) bool { return only == "" || strings.Contains(","+only+",", ","+n+",") }

	if part("prep") {
		livePrep(h)
	}
	if part("obj") {
		liveObjects(h)
	}
	if part("st") {
		liveStorage(h)
	}
	if part("sec") {
		liveSecurity(h)
	}
	if part("sc") {
		liveScheduler(h)
	}
	if part("rm") {
		liveResource(h)
	}
	if part("mo") {
		liveMonitor(h)
	}
	if part("pf") {
		livePerf(h)
	}
	if part("dp") {
		liveDataPump(h)
	}
	if part("db") {
		liveDatabase(h)
	}
	if part("bk") {
		liveBackup(h)
	}
	if part("pdb") {
		livePDBs(h)
	}

	// cobertura: toda ação da aba DBA
	var miss []string
	catalog.Walk(catalog.DBA(), func(_ []string, n *catalog.Node) {
		if n.View == nil {
			return
		}
		for _, x := range n.View.Actions {
			k := fmt.Sprintf("%s %c", n.View.ID, x.Key)
			if _, ok := h.done[k]; ok {
				continue
			}
			why := h.skips[k]
			if why == "" {
				why = "(não testada)"
			}
			miss = append(miss, fmt.Sprintf("%-22s %-45s %s", k, x.Name, why))
		}
	})
	sort.Strings(miss)
	okN, errN := 0, 0
	for _, s := range h.done {
		if s == "ok" {
			okN++
		} else {
			errN++
		}
	}
	h.log("\n==== %d ok, %d com erro, %d sem teste ====", okN, errN, len(miss))
	for _, f := range h.fails {
		h.log("ERRO  %s", f)
	}
	for _, m := range miss {
		h.log("SKIP  %s", m)
	}
	if errN > 0 {
		t.Errorf("%d ação(ões) com erro", errN)
	}
}

const dfDir = "/opt/oracle/oradata/FREE/FREEPDB1/"

func livePrep(h *liveH) {
	h.sql(h.pdb, `begin
  for u in (select username from dba_users where username like 'OBS\_%' escape '\') loop
    execute immediate 'drop user ' || u.username || ' cascade';
  end loop;
end;
/
begin
  for r in (select role from dba_roles where role like 'OBS\_%' escape '\') loop execute immediate 'drop role ' || r.role; end loop;
  for t in (select tablespace_name n from dba_tablespaces where tablespace_name like 'OBS\_%' escape '\') loop
    execute immediate 'drop tablespace ' || t.n || ' including contents and datafiles';
  end loop;
end;
/
create user obs_app identified by "Obs#2026x" default tablespace users quota unlimited on users;
grant connect, resource, create table, create view, create synonym, create materialized view, create database link, create job to obs_app;
create user obs_u1 identified by "Obs#2026x" default tablespace users;
grant create session to obs_u1;
create table obs_app.t1 (id number primary key, v varchar2(20), n number) enable row movement;
insert into obs_app.t1 select level, 'linha ' || level, mod(level, 7) from dual connect by level <= 2000;
commit;
create index obs_app.t1_n on obs_app.t1 (n);
create sequence obs_app.s1;
create view obs_app.v1 as select id, v from obs_app.t1 where n = 1;
create or replace trigger obs_app.t1_trg before insert on obs_app.t1 for each row begin null; end;
/
create or replace package obs_app.pk as procedure p; end;
/
create or replace package body obs_app.pk as procedure p is begin null; end; end;
/
create or replace procedure obs_app.pr as begin null; end;
/
create or replace function obs_app.fn return number as begin return 1; end;
/
create or replace type obs_app.ty as object (a number);
/
create synonym obs_app.sy for obs_app.t1;
create materialized view obs_app.mv as select n, count(*) c from obs_app.t1 group by n;
begin dbms_scheduler.create_job(job_name => 'OBS_APP.OBS_OBJJOB', job_type => 'PLSQL_BLOCK', job_action => 'begin null; end;', enabled => false); end;
/
create table obs_app.lixo (id number);
drop table obs_app.lixo;
create or replace procedure obs_app.ruim as begin nul; end;
/`)
}

func liveObjects(h *liveH) {
	c, o := h.pdb, "OBS_APP"
	// link privado só o dono cria
	app := liveInfo("FREEPDB1")
	app.User, app.Password, app.Role = "obs_app", "Obs#2026x", ""
	if db, err := ora.Open(app); err == nil {
		if _, err := db.Exec(`create database link obs_app_lnk connect to obs_app identified by "Obs#2026x" using '//localhost:1521/FREEPDB1'`); err != nil {
			h.log("  preparo com erro: %v", err)
		}
		ora.Close(db)
	}
	for _, cat := range catalog.Categories() {
		if r := Query(c, cat.CategorySQL(), []any{o}, 100); r.Err != nil {
			h.fail("pasta "+cat.Title, r.Err.Error())
		} else {
			h.done["pasta "+cat.Title] = "ok"
		}
	}
	for _, x := range [][2]string{{"T1", "TABLE"}, {"V1", "VIEW"}, {"T1_N", "INDEX"}, {"PK", "PACKAGE"}, {"PR", "PROCEDURE"},
		{"FN", "FUNCTION"}, {"T1_TRG", "TRIGGER"}, {"TY", "TYPE"}, {"S1", "SEQUENCE"}, {"SY", "SYNONYM"}, {"MV", "MATERIALIZED VIEW"},
		{"OBS_APP_LNK", "DATABASE LINK"}, {"OBS_OBJJOB", "JOB"}} {
		h.details(c, o, x[0], x[1])
	}
	// tabela
	h.obj(c, o, "T1", "TABLE", 's', nil)
	h.obj(c, o, "T1", "TABLE", 'n', nil)
	h.obj(c, o, "T1", "TABLE", 'a', W("COL", "OBS_C2", "TYPE", "VARCHAR2(10)"))
	h.obj(c, o, "T1", "TABLE", 'o', W("TEXT", "tabela do teste ao vivo"))
	h.obj(c, o, "T1", "TABLE", 't', W("MODE", "DISABLE"))
	h.obj(c, o, "T1", "TABLE", 't', W("MODE", "ENABLE"))
	pk := Query(c, "select constraint_name from all_constraints where owner = 'OBS_APP' and table_name = 'T1' and constraint_type = 'P'", nil, 1)
	if len(pk.Rows) > 0 {
		h.obj(c, o, "T1", "TABLE", 'k', W("MODE", "DISABLE", "NAME", pk.Rows[0][0].V))
		h.obj(c, o, "T1", "TABLE", 'k', W("MODE", "ENABLE", "NAME", pk.Rows[0][0].V))
	}
	h.obj(c, o, "T1", "TABLE", 'h', nil)
	h.obj(c, o, "T1", "TABLE", 'v', nil)
	h.obj(c, o, "T1", "TABLE", 'l', nil)
	h.obj(c, o, "T1", "TABLE", 'g', W("TO", "OBS_U1", "PRIV", "SELECT"))
	h.obj(c, o, "T1", "TABLE", 'y', W("NAME", "T1_SYN"))
	h.obj(c, o, "T1", "TABLE", 'm', W("NEW", "T1B"))
	h.obj(c, o, "T1B", "TABLE", 'm', W("NEW", "T1"))
	h.sql(c, "create table obs_app.t2 as select * from obs_app.t1;")
	h.obj(c, o, "T2", "TABLE", 'T', nil)
	h.obj(c, o, "T2", "TABLE", 'X', nil)
	// índice
	h.obj(c, o, "T1_N", "INDEX", 'u', nil)
	h.obj(c, o, "T1_N", "INDEX", 'r', nil)
	h.obj(c, o, "T1_N", "INDEX", 'v', W("MODE", "INVISIBLE"))
	h.obj(c, o, "T1_N", "INDEX", 'v', W("MODE", "VISIBLE"))
	h.obj(c, o, "T1_N", "INDEX", 's', nil)
	h.obj(c, o, "T1_N", "INDEX", 'm', W("NEW", "T1_N2"))
	h.obj(c, o, "T1_N2", "INDEX", 'm', W("NEW", "T1_N"))
	// código
	for _, x := range [][2]string{{"PK", "PACKAGE"}, {"PR", "PROCEDURE"}, {"FN", "FUNCTION"}, {"T1_TRG", "TRIGGER"}, {"TY", "TYPE"}, {"V1", "VIEW"}} {
		h.obj(c, o, x[0], x[1], 'c', nil)
	}
	h.obj(c, o, "T1_TRG", "TRIGGER", 'd', nil)
	h.obj(c, o, "T1_TRG", "TRIGGER", 'e', nil)
	h.obj(c, o, "PR", "PROCEDURE", 'g', W("TO", "OBS_U1", "PRIV", "EXECUTE"))
	// sequência, mview, link, job
	h.obj(c, o, "S1", "SEQUENCE", 'e', W("INC", "2", "CACHE", "50"))
	h.obj(c, o, "S1", "SEQUENCE", 'r', W("START", "10"))
	h.obj(c, o, "MV", "MATERIALIZED VIEW", 'r', W("METHOD", "C"))
	h.obj(c, o, "OBS_APP_LNK", "DATABASE LINK", 't', nil)
	h.sql(c, `create public database link obs_pub_lnk connect to obs_app identified by "Obs#2026x" using '//localhost:1521/FREEPDB1';
create database link obs_sys_lnk connect to obs_app identified by "Obs#2026x" using '//localhost:1521/FREEPDB1';`)
	for _, x := range [][2]string{{"PUBLIC", "OBS_PUB_LNK"}, {"SYS", "OBS_SYS_LNK"}} {
		h.obj(c, x[0], x[1], "DATABASE LINK", 't', nil)
		h.obj(c, x[0], x[1], "DATABASE LINK", 'X', nil)
	}
	h.obj(c, o, "OBS_OBJJOB", "JOB", 'e', nil)
	h.obj(c, o, "OBS_OBJJOB", "JOB", 'r', nil)
	h.obj(c, o, "OBS_OBJJOB", "JOB", 'd', nil)
	// lixeira
	bin := Query(c, "select object_name from dba_recyclebin where owner = 'OBS_APP' and original_name = 'LIXO'", nil, 1)
	if len(bin.Rows) > 0 {
		h.details(c, o, bin.Rows[0][0].V, "RECYCLEBIN")
		h.obj(c, o, bin.Rows[0][0].V, "RECYCLEBIN", 'r', W("NEW", "LIXO2"))
		h.sql(c, "drop table obs_app.lixo2;")
		bin = Query(c, "select object_name from dba_recyclebin where owner = 'OBS_APP' and original_name = 'LIXO2'", nil, 1)
		if len(bin.Rows) > 0 {
			h.obj(c, o, bin.Rows[0][0].V, "RECYCLEBIN", 'p', nil)
		}
	}
	// drops no fim
	h.obj(c, o, "SY", "SYNONYM", 'X', nil)
	h.obj(c, o, "MV", "MATERIALIZED VIEW", 'X', nil)
	h.obj(c, o, "OBS_OBJJOB", "JOB", 'X', nil)
	h.obj(c, o, "OBS_APP_LNK", "DATABASE LINK", 'X', nil)
}

func liveStorage(h *liveH) {
	c := h.pdb
	h.do(c, "st.tablespaces", 'n', stepOpt{vals: W("NAME", "OBS_TS", "FILE", dfDir+"obs_ts01.dbf", "SIZE", "10M", "NEXT", "10M", "MAX", "200M")})
	ts := W("TABLESPACE_NAME", "OBS_TS")
	h.do(c, "st.tablespaces", 'a', stepOpt{where: ts, vals: W("FILE", dfDir+"obs_ts02.dbf", "SIZE", "5M", "NEXT", "5M", "MAX", "100M")})
	h.do(c, "st.tablespaces", 'r', stepOpt{where: ts, vals: W("MODE", "READ ONLY")})
	h.do(c, "st.tablespaces", 'r', stepOpt{where: ts, vals: W("MODE", "READ WRITE")})
	h.do(c, "st.tablespaces", 'f', stepOpt{where: ts, vals: W("MODE", "NORMAL")})
	h.do(c, "st.tablespaces", 'o', stepOpt{where: ts})
	h.do(c, "st.tablespaces", 'l', stepOpt{where: ts, vals: W("MODE", "NOLOGGING")})
	h.do(c, "st.tablespaces", 'l', stepOpt{where: ts, vals: W("MODE", "LOGGING")})
	h.do(c, "st.tablespaces", 'm', stepOpt{where: ts, vals: W("NEW", "OBS_TSB")})
	h.do(c, "st.tablespaces", 'm', stepOpt{where: W("TABLESPACE_NAME", "OBS_TSB"), vals: W("NEW", "OBS_TS")})
	h.do(c, "st.tablespaces", 'D', stepOpt{where: ts})
	h.do(c, "st.tablespaces", 'v', stepOpt{where: ts})
	h.do(c, "st.tablespaces", 'e', stepOpt{where: ts})
	h.do(c, "st.tablespaces", 'd', stepOpt{where: ts})
	h.do(c, "db.database", 'd', stepOpt{vals: W("KIND", "PERMANENT", "TS", "USERS")})
	// datafiles
	f2 := W("FILE_NAME", "~obs_ts02")
	h.do(c, "st.datafiles", 'z', stepOpt{where: f2, vals: W("SIZE", "8M")})
	h.do(c, "st.datafiles", 'a', stepOpt{where: f2, vals: W("NEXT", "8M", "MAX", "64M")})
	if h.isEE() {
		h.do(c, "st.datafiles", 'm', stepOpt{where: f2, vals: W("TO", dfDir+"obs_ts02b.dbf")})
		f2 = W("FILE_NAME", "~obs_ts02b")
	} else {
		h.skip("st.datafiles m", "mover datafile online é recurso do Enterprise Edition (ORA-00439 no Free)")
	}
	h.do(c, "st.datafiles", 'x', stepOpt{where: f2})
	// temporária e grupo
	h.do(c, "st.tablespaces", 'n', stepOpt{vals: W("NAME", "OBS_TEMP", "TYPE", "TEMPORARY", "FILE", dfDir+"obs_temp01.dbf", "SIZE", "10M", "NEXT", "10M", "MAX", "100M")})
	tmp := W("TABLESPACE_NAME", "OBS_TEMP")
	h.do(c, "st.tablespaces", 'k', stepOpt{where: tmp, vals: W("KEEP", "5M")})
	h.do(c, "st.tablespaces", 'g', stepOpt{where: tmp, vals: W("GROUP", "OBS_TG")})
	h.do(c, "st.tempgroups", 'n', stepOpt{vals: W("TS", "TEMP", "GROUP", "OBS_TG")})
	h.do(c, "st.tempgroups", 'd', stepOpt{where: W("GROUP_NAME", "OBS_TG")})
	h.do(c, "db.database", 'd', stepOpt{vals: W("KIND", "TEMPORARY", "TS", "TEMP")})
	h.do(c, "st.tempgroups", 'x', stepOpt{where: W("GROUP_NAME", "OBS_TG", "TABLESPACE_NAME", "TEMP")})
	h.do(c, "st.tempgroups", 'x', stepOpt{where: W("GROUP_NAME", "OBS_TG", "TABLESPACE_NAME", "OBS_TEMP")})
	// undo
	h.do(c, "st.tablespaces", 'n', stepOpt{vals: W("NAME", "OBS_UNDO", "TYPE", "UNDO", "FILE", dfDir+"obs_undo01.dbf", "SIZE", "20M", "NEXT", "10M", "MAX", "100M")})
	h.do(c, "st.tablespaces", 'u', stepOpt{where: W("TABLESPACE_NAME", "OBS_UNDO")})
	h.do(c, "st.tablespaces", 'u', stepOpt{where: W("TABLESPACE_NAME", "UNDOTBS1")})
	h.do(c, "st.undoext", 'r', stepOpt{vals: W("SEC", "900")})
	h.skip("st.rbs o", "undo automático (AUM): o Oracle não deixa pôr segmento de undo online/offline à mão (ORA-30019)")
	h.skip("st.rbs f", "undo automático (AUM): ORA-30019")
	// lixeira (tela DBA)
	h.sql(c, "create table obs_app.lixo3 (id number);\ndrop table obs_app.lixo3;\ncreate table obs_app.lixo4 (id number);\ndrop table obs_app.lixo4;\ncreate table obs_app.lixo5 (id number) tablespace obs_ts;\ndrop table obs_app.lixo5;")
	h.do(c, "st.recyclebin", 'r', stepOpt{where: W("ORIGINAL_NAME", "LIXO3"), vals: W("NEW", "LIXO3B")})
	h.do(c, "st.recyclebin", 'p', stepOpt{where: W("ORIGINAL_NAME", "LIXO4")})
	h.do(c, "st.recyclebin", 't', stepOpt{where: W("ORIGINAL_NAME", "LIXO5"), vals: W("USER", "OBS_APP")})
	h.do(c, "st.recyclebin", 'P', stepOpt{})
	// diretórios
	h.do(c, "st.directories", 'n', stepOpt{vals: W("NAME", "OBS_DIR", "PATH", "/opt/oracle/oradata/dp")})
	h.do(c, "st.directories", 'g', stepOpt{where: W("DIRECTORY_NAME", "OBS_DIR"), vals: W("TO", "OBS_APP")})
	h.do(c, "st.directories", 'v', stepOpt{where: W("DIRECTORY_NAME", "OBS_DIR")})
	// segmentos
	h.sql(c, "alter user obs_app quota unlimited on obs_ts;")
	seg := W("OWNER", "OBS_APP", "SEGMENT_NAME", "T1")
	h.do(c, "st.segments", 's', stepOpt{where: seg, args: []any{"USERS"}})
	h.do(c, "st.segments", 'm', stepOpt{where: seg, args: []any{"USERS"}, vals: W("TS", "OBS_TS")})
	h.do(c, "st.segments", 'm', stepOpt{where: seg, args: []any{"OBS_TS"}, vals: W("TS", "USERS")})
	// restore point (PDB)
	h.do(c, "st.restorepoints", 'n', stepOpt{vals: W("NAME", "OBS_RP")})
	h.do(c, "st.restorepoints", 'x', stepOpt{where: W("NAME", "OBS_RP")})
	// limpeza
	h.do(c, "st.tablespaces", 'x', stepOpt{where: W("TABLESPACE_NAME", "OBS_UNDO")})
	h.do(c, "st.tablespaces", 'x', stepOpt{where: W("TABLESPACE_NAME", "OBS_TEMP")})
	h.do(c, "st.tablespaces", 'x', stepOpt{where: ts, vals: W("CONTENTS", "1", "CASCADE", "1")})

	// CDB: control file, redo, standby logs, archive dest
	r := h.cdb
	// DROP LOGFILE não apaga arquivo fora do OMF: limpa as sobras de rodadas antigas
	_ = exec.Command("docker", "exec", "obs-oracle-free", "rm", "-f", "/opt/oracle/oradata/FREE/redo09a.log",
		"/opt/oracle/oradata/FREE/redo09b.log", "/opt/oracle/oradata/FREE/srl01.log").Run()
	h.do(r, "st.controlfiles", 't', stepOpt{})
	h.do(r, "st.controlfiles", 'b', stepOpt{vals: W("FILE", "/tmp/control_obs.bkp")})
	h.do(r, "st.controlfiles", 's', stepOpt{where: map[string]string{}})
	h.do(r, "st.redo", 's', stepOpt{})
	h.do(r, "st.redo", 'c', stepOpt{})
	h.do(r, "st.redo", 'n', stepOpt{vals: W("GROUP", "9", "FILES", "/opt/oracle/oradata/FREE/redo09a.log", "SIZE", "50M")})
	g9 := W("GROUP#", "9")
	h.do(r, "st.redo", 'm', stepOpt{where: g9, vals: W("FILE", "/opt/oracle/oradata/FREE/redo09b.log")})
	h.do(r, "st.redo", 'v', stepOpt{where: g9})
	h.do(r, "st.redo", 'l', stepOpt{where: g9})
	h.do(r, "st.redo", 'x', stepOpt{where: g9})
	h.do(r, "st.standbylogs", 'n', stepOpt{vals: W("FILES", "/opt/oracle/oradata/FREE/srl01.log", "SIZE", "50M")})
	h.do(r, "st.standbylogs", 'x', stepOpt{where: map[string]string{}})
	_ = exec.Command("docker", "exec", "obs-oracle-free", "rm", "-f", "/opt/oracle/oradata/FREE/redo09a.log",
		"/opt/oracle/oradata/FREE/redo09b.log", "/opt/oracle/oradata/FREE/srl01.log").Run()
	h.do(r, "st.archdest", 's', stepOpt{where: W("DEST_ID", "1"), vals: W("STATE", "ENABLE")})
}

func liveSecurity(h *liveH) {
	c := h.pdb
	u := W("USERNAME", "OBS_U2")
	h.do(c, "sec.users", 'n', stepOpt{vals: W("USER", "OBS_U2", "PWD", "Obs#2026x", "ROLES", "CONNECT")})
	h.do(c, "sec.users", 'e', stepOpt{where: u, vals: W("PWD", "Obs#2026z")})
	h.do(c, "sec.users", 'p', stepOpt{where: u, vals: W("PWD", "Obs#2026y")})
	h.do(c, "sec.users", 'l', stepOpt{where: u})
	h.do(c, "sec.users", 'u', stepOpt{where: u})
	h.do(c, "sec.users", 'x', stepOpt{where: u})
	h.do(c, "sec.users", 'q', stepOpt{where: u, vals: W("TS", "USERS", "QUOTA", "10M")})
	h.do(c, "sec.users", 'g', stepOpt{where: u, vals: W("ROLE", "RESOURCE")})
	h.do(c, "sec.users", 's', stepOpt{where: u, vals: W("PRIV", "CREATE TABLE")})
	h.do(c, "sec.users", 'o', stepOpt{where: u, vals: W("PRIV", "SELECT", "OBJ", "OBS_APP.T1")})
	h.do(c, "sec.users", 'D', stepOpt{where: u})
	for _, k := range "rybz" {
		h.do(c, "sec.users", k, stepOpt{where: u})
	}
	h.do(c, "sec.users.app", 'l', stepOpt{where: u})
	h.do(c, "sec.users.app", 'u', stepOpt{where: u})
	// roles
	r := W("ROLE", "OBS_R")
	h.do(c, "sec.roles", 'n', stepOpt{vals: W("ROLE", "OBS_R")})
	h.do(c, "sec.roles", 't', stepOpt{where: r, vals: W("TO", "OBS_U2")})
	h.do(c, "sec.roles", 'g', stepOpt{where: r, vals: W("ROLE", "CONNECT")})
	h.do(c, "sec.roles", 's', stepOpt{where: r, vals: W("PRIV", "CREATE VIEW")})
	h.do(c, "sec.roles", 'o', stepOpt{where: r, vals: W("PRIV", "SELECT", "OBJ", "OBS_APP.T1")})
	for _, k := range "mryb" {
		h.do(c, "sec.roles", k, stepOpt{where: r})
	}
	// profiles
	h.sql(c, "create profile obs_prof limit failed_login_attempts 10;")
	h.do(c, "sec.profiles", 'e', stepOpt{where: W("PROFILE", "OBS_PROF", "RESOURCE_NAME", "FAILED_LOGIN_ATTEMPTS"), vals: W("VALUE", "5")})
	h.do(c, "sec.profiles", 'u', stepOpt{where: W("PROFILE", "OBS_PROF")})
	h.do(c, "sec.profiles", 'X', stepOpt{where: W("PROFILE", "OBS_PROF")})
	// privilégios
	h.do(c, "sec.roleprivs", 'd', stepOpt{where: W("GRANTEE", "OBS_U2", "GRANTED_ROLE", "RESOURCE"), vals: W("MODE", "ALL EXCEPT")})
	h.do(c, "sec.roleprivs", 'v', stepOpt{where: W("GRANTEE", "OBS_U2", "GRANTED_ROLE", "RESOURCE")})
	h.do(c, "sec.sysprivs", 'v', stepOpt{where: W("GRANTEE", "OBS_U2", "PRIVILEGE", "CREATE TABLE")})
	h.do(c, "sec.tabprivs", 'v', stepOpt{where: W("GRANTEE", "OBS_U2")})
	h.do(c, "sec.quotas", 'q', stepOpt{where: W("USERNAME", "OBS_U2"), vals: W("QUOTA", "20M")})
	// auditoria
	h.do(h.cdb, "sec.auditcfg", 'e', stepOpt{where: W("NAME", "audit_sys_operations"), vals: W("VALUE", "TRUE", "SCOPE", "SPFILE")})
	h.do(c, "sec.policies", 'n', stepOpt{vals: W("NAME", "OBS_POL", "ACTIONS", "DELETE ON OBS_APP.T1")})
	pol := W("POLICY_NAME", "OBS_POL")
	h.do(c, "sec.policies", 'e', stepOpt{where: pol})
	h.do(c, "sec.policies", 'v', stepOpt{where: pol})
	h.do(c, "sec.policies", 'd', stepOpt{where: pol})
	h.do(c, "sec.policies", 'X', stepOpt{where: pol})
	h.do(c, "sec.trail", 'p', stepOpt{})
	if ver := Query(c, "select to_number(regexp_substr(version, '^[0-9]+')) from v$instance", nil, 1); len(ver.Rows) > 0 && len(ver.Rows[0][0].V) == 2 && ver.Rows[0][0].V < "23" {
		for _, id := range []string{"sec.stmtaudit", "sec.privaudit"} {
			h.do(c, id, 'n', stepOpt{vals: W("OPT", "CREATE TABLE", "BY", "OBS_U2")})
			h.do(c, id, 'v', stepOpt{where: W("USER_NAME", "OBS_U2")})
		}
		h.do(c, "sec.objaudit", 'n', stepOpt{vals: W("OBJ", "OBS_APP.T1")})
		h.do(c, "sec.objaudit", 'v', stepOpt{where: W("OWNER", "OBS_APP", "OBJECT_NAME", "T1")})
	} else {
		for _, k := range []string{"sec.stmtaudit n", "sec.stmtaudit v", "sec.privaudit n", "sec.privaudit v", "sec.objaudit n", "sec.objaudit v"} {
			h.skip(k, "auditoria tradicional descontinuada no 23ai+ (ORA-46401 é a resposta certa do banco; a ajuda da tela explica)")
		}
	}
	// fim
	h.do(c, "sec.roles", 'X', stepOpt{where: r})
	h.do(c, "sec.users", 'X', stepOpt{where: u, vals: W("CASCADE", "1")})
}

func liveScheduler(h *liveH) {
	c := h.pdb
	j := W("JOB_NAME", "OBS_JOB")
	h.do(c, "sc.jobs", 'n', stepOpt{vals: W("NAME", "OBS_APP.OBS_JOB", "ACTION", "BEGIN dbms_session.sleep(30); END;", "ENABLED", "")})
	h.do(c, "sc.jobs", 'e', stepOpt{where: j})
	h.do(c, "sc.jobs", 'i', stepOpt{where: j, vals: W("RI", "FREQ=HOURLY")})
	h.do(c, "sc.jobs", 'r', stepOpt{where: j})
	time.Sleep(2 * time.Second)
	h.do(c, "sc.jobs", 's', stepOpt{where: j, vals: W("FORCE", "1")})
	h.do(c, "sc.jobs", 'd', stepOpt{where: j})
	h.do(c, "sc.jobs", 'D', stepOpt{where: j})
	h.do(c, "sc.jobs", 'l', stepOpt{where: j})
	h.do(c, "sc.jobs", 'x', stepOpt{where: j})
	h.do(c, "sc.runs", 'p', stepOpt{})
	h.sql(c, "begin dbms_scheduler.create_program('SYS.OBS_PROG', 'PLSQL_BLOCK', 'begin null; end;'); end;\n/\nbegin dbms_scheduler.create_chain('SYS.OBS_CHAIN'); end;\n/")
	pg := W("PROGRAM_NAME", "OBS_PROG")
	h.do(c, "sc.programs", 'e', stepOpt{where: pg})
	h.do(c, "sc.programs", 'd', stepOpt{where: pg})
	h.do(c, "sc.programs", 'x', stepOpt{where: pg})
	h.do(c, "sc.schedules", 'n', stepOpt{vals: W("NAME", "SYS.OBS_SCH")})
	h.do(c, "sc.schedules", 'x', stepOpt{where: W("SCHEDULE_NAME", "OBS_SCH")})
	w := W("WINDOW_NAME", "SATURDAY_WINDOW")
	h.do(c, "sc.windows", 'd', stepOpt{where: w})
	h.do(c, "sc.windows", 'e', stepOpt{where: w})
	h.do(c, "sc.windows", 'p', stepOpt{where: w})
	h.do(c, "sc.windows", 'o', stepOpt{where: w, vals: W("MIN", "5")})
	h.do(c, "sc.windows", 'c', stepOpt{where: w})
	ch := W("CHAIN_NAME", "OBS_CHAIN")
	h.do(c, "sc.chains", 'd', stepOpt{where: ch})
	h.do(c, "sc.chains", 'e', stepOpt{where: ch})
	h.do(c, "sc.chains", 'x', stepOpt{where: ch})
	h.do(c, "sc.credentials", 'n', stepOpt{vals: W("NAME", "SYS.OBS_CRED", "PWD", "x")})
	h.do(c, "sc.credentials", 'x', stepOpt{where: W("CREDENTIAL_NAME", "OBS_CRED")})
	h.sql(c, "declare j number; begin dbms_job.submit(j, 'null;', sysdate + 1, 'sysdate + 1'); commit; end;\n/")
	dj := W("WHAT", "~null;")
	h.do(c, "sc.dbmsjob", 'b', stepOpt{where: dj, vals: W("B", "TRUE")})
	h.do(c, "sc.dbmsjob", 'b', stepOpt{where: dj, vals: W("B", "FALSE")})
	h.do(c, "sc.dbmsjob", 'r', stepOpt{where: dj})
	h.sql(c, "begin for j in (select job from dba_jobs where what = 'null;') loop dbms_job.remove(j.job); end loop; commit; end;\n/")
	at := W("CLIENT_NAME", "~sql tuning")
	h.do(c, "sc.autotask", 'd', stepOpt{where: at})
	h.do(c, "sc.autotask", 'e', stepOpt{where: at})
}

func liveResource(h *liveH) {
	c := h.pdb
	h.do(c, "rm.plans", 'n', stepOpt{vals: W("PLAN", "OBS_PLAN", "OTHER", "50")})
	h.do(c, "rm.groups", 'n', stepOpt{vals: W("GROUP", "OBS_G")})
	h.do(c, "rm.directives", 'n', stepOpt{vals: W("PLAN", "OBS_PLAN", "GROUP", "OBS_G", "MGMT_P1", "50")})
	d := W("PLAN", "OBS_PLAN", "GROUP_OR_SUBPLAN", "OBS_G")
	h.do(c, "rm.directives", 'e', stepOpt{where: d, vals: W("MGMT_P1", "40")})
	h.do(c, "rm.groups", 'g', stepOpt{where: W("CONSUMER_GROUP", "OBS_G"), vals: W("TO", "OBS_U1")})
	h.do(c, "rm.mappings", 'n', stepOpt{vals: W("VALUE", "OBS_U1", "GROUP", "OBS_G")})
	h.do(c, "rm.mappings", 'x', stepOpt{where: W("VALUE", "OBS_U1")})
	h.do(c, "rm.priorities", 'e', stepOpt{})
	h.do(c, "rm.plans", 'v', stepOpt{where: W("PLAN", "OBS_PLAN")})
	h.do(c, "rm.plans", 'a', stepOpt{where: W("PLAN", "OBS_PLAN")})
	h.do(c, "rm.plans", 'd', stepOpt{})
	h.do(c, "rm.directives", 'x', stepOpt{where: d})
	h.do(c, "rm.groups", 'x', stepOpt{where: W("CONSUMER_GROUP", "OBS_G")})
	h.do(c, "rm.plans", 'x', stepOpt{where: W("PLAN", "OBS_PLAN"), vals: W("CASCADE", "1")})
	if !h.isEE() {
		for _, k := range "nvax" {
			h.skip("rm.cdbplans "+string(k), "plano de CDB é recurso do Enterprise Edition (ORA-00439 no Free)")
		}
		return
	}
	r := h.cdb
	h.do(r, "rm.cdbplans", 'n', stepOpt{vals: W("PLAN", "OBS_CDBP")})
	h.do(r, "rm.cdbplans", 'v', stepOpt{where: W("PLAN", "OBS_CDBP")})
	h.do(r, "rm.cdbplans", 'a', stepOpt{where: W("PLAN", "OBS_CDBP")})
	h.sql(r, "alter system set resource_manager_plan = '' scope = memory;")
	h.do(r, "rm.cdbplans", 'x', stepOpt{where: W("PLAN", "OBS_CDBP")})
}

// segunda sessão: OBS_U1 segura um lock; outra fica esperando (bloqueio).
func liveMonitor(h *liveH) {
	c := h.pdb
	h.sql(c, "grant update on obs_app.t1 to obs_u1;\ngrant select on obs_app.t1 to obs_u1;\ngrant execute on dbms_session to obs_u1;")
	info := liveInfo("FREEPDB1")
	info.User, info.Password, info.Role = "obs_u1", "Obs#2026x", ""
	// cada sessão de OBS_U1 numa conexão só, com o SID dela
	var open []*sql.DB
	defer func() {
		for _, db := range open {
			ora.Close(db)
		}
	}()
	sess := func() (*sql.DB, string) {
		db, err := ora.Open(info)
		if err != nil {
			h.fail("monitor: sessão OBS_U1", err.Error())
			return nil, "-1"
		}
		db.SetMaxOpenConns(1)
		open = append(open, db)
		var sid string
		_ = db.QueryRow("select sys_context('userenv', 'sid') from dual").Scan(&sid)
		return db, sid
	}
	by := func(sid string) stepOpt { return stepOpt{where: W("SID", sid)} }

	// A segura a linha (transação aberta), B espera por ela
	dbA, sidA := sess()
	if dbA == nil {
		return
	}
	txA, err := dbA.Begin()
	if err == nil {
		_, err = txA.Exec("update obs_app.t1 set v = 'travada' where id = 1")
	}
	if err != nil {
		h.fail("monitor: lock", err.Error())
		return
	}
	dbB, sidB := sess()
	go func() { _, _ = dbB.Exec("update obs_app.t1 set v = 'esperando' where id = 1") }()
	dbC, sidC := sess()
	time.Sleep(3 * time.Second)

	h.do(c, "mo.blockers", 's', by(sidB))
	h.do(c, "mo.blockers", 'p', by(sidB))
	h.do(c, "mo.locks", 's', by(sidB))
	for _, k := range "sptuwoe" {
		h.do(c, "mo.sessions", k, by(sidB))
	}
	for _, k := range "spwoe" {
		h.do(c, "mo.allsessions", k, by(sidB))
	}
	for _, k := range "sptuwoe" {
		h.skip("mo.active "+string(k), "mesma ação de mo.sessions (testada lá)")
	}
	h.skip("mo.longops k", "precisa de uma operação longa em andamento (mesma ação de mo.sessions k, testada lá)")
	// mata o que segura: B segue
	h.do(c, "mo.blockers", 'k', by(sidA))
	// "Ativas" ignora espera ociosa (sleep): C e D ficam em CPU de verdade
	busy := func(db *sql.DB) {
		go func() {
			var n int // o trabalho do count(*) é feito no fetch
			_ = db.QueryRow("select count(*) from obs_app.t1 a, obs_app.t1 b, obs_app.t1 c where a.n + b.n + c.n < 0").Scan(&n)
		}()
		time.Sleep(2 * time.Second)
	}
	busy(dbC)
	h.do(c, "mo.active", 'c', by(sidC))
	time.Sleep(time.Second)
	busy(dbC)
	h.do(c, "mo.active", 'k', by(sidC))
	dbD, sidD := sess()
	busy(dbD)
	h.do(c, "mo.active", 'd', by(sidD))
	_, sidE := sess()
	h.do(c, "mo.sessions", 'd', by(sidE))
	_, sidF := sess()
	h.do(c, "mo.sessions", 'k', by(sidF))
	dbG, sidG := sess()
	if tx, err := dbG.Begin(); err == nil {
		_, _ = tx.Exec("update obs_app.t1 set v = 'lock G' where id = 2")
	}
	time.Sleep(time.Second)
	h.do(c, "mo.locks", 'k', by(sidG))
	_, sidH := sess()
	h.do(c, "mo.allsessions", 'k', by(sidH))
	h.sql(c, "update obs_app.t1 set v = 'linha 1' where id = 1;\ncommit;")
	// top SQL
	h.sql(c, "select /*+ monitor */ /* obs_topsql */ count(*) from obs_app.t1 a, obs_app.t1 b, (select level from dual connect by level <= 300) where a.n = b.n;")
	ts := W("~SQL_TEXT", "obs_topsql")
	// o SQL Tuning Advisor roda variantes do mesmo texto: fixa o SQL_ID do original
	if id := Query(c, "select sql_id from v$sqlstats where sql_text like 'select /*+ monitor */ /* obs_topsql */%'", nil, 1); len(id.Rows) > 0 {
		ts = W("SQL_ID", id.Rows[0][0].V)
	}
	for _, k := range "sphmb" {
		h.do(c, "mo.topsql", k, stepOpt{where: ts})
	}
	h.do(c, "mo.topsql", 't', stepOpt{where: map[string]string{}, vals: W("SEC", "30")})
	// depois do baseline e do Tuning Advisor o cursor original some do top 100;
	// o PURGE vale para qualquer cursor: usa o primeiro
	h.do(c, "mo.topsql", 'x', stepOpt{where: map[string]string{}})
	// inválidos
	h.do(c, "mo.invalid", 'v', stepOpt{where: W("OBJECT_NAME", "RUIM")})
	h.do(c, "mo.invalid", 'c', stepOpt{where: W("OBJECT_NAME", "RUIM")})
	h.sql(c, "create or replace procedure obs_app.pr2 as begin obs_app.pr; end;\n/\ncreate or replace procedure obs_app.pr as x number; begin null; end;\n/")
	h.do(c, "mo.invalid", 's', stepOpt{where: W("OWNER", "OBS_APP"), vals: W("SCHEMA", "OBS_APP")})
	h.do(c, "mo.invalid", 'a', stepOpt{})
}

func livePerf(h *liveH) {
	r := h.cdb
	h.do(r, "pf.awrcfg", 'e', stepOpt{vals: W("INTERVAL", "60", "RETENTION", "8")})
	h.do(r, "pf.snaps", 'n', stepOpt{})
	h.sql(r, "select count(*) from dba_objects a, (select 1 from dual connect by level <= 20);")
	h.do(r, "pf.snaps", 'n', stepOpt{})
	snaps := Query(r, "select max(snap_id), max(snap_id) - 1 from dba_hist_snapshot where dbid = (select dbid from v$database)", nil, 1)
	last, prev := snaps.Rows[0][0].V, snaps.Rows[0][1].V
	sw := W("SNAP_ID", last)
	h.do(r, "pf.snaps", 'r', stepOpt{where: sw})
	h.do(r, "pf.snaps", 'h', stepOpt{where: sw})
	h.do(r, "pf.snaps", 'd', stepOpt{where: sw, vals: W("B1", prev, "E1", last)})
	sq := Query(r, "select sql_id from dba_hist_sqlstat where snap_id = "+last+" and rownum = 1", nil, 1)
	if len(sq.Rows) > 0 {
		h.do(r, "pf.snaps", 's', stepOpt{where: sw, vals: W("SQLID", sq.Rows[0][0].V)})
	}
	h.do(r, "pf.snaps", 'a', stepOpt{where: sw})
	h.do(r, "pf.snaps", 'b', stepOpt{where: sw, vals: W("NAME", "OBS_BL1")})
	h.do(r, "pf.baselines", 'n', stepOpt{vals: W("NAME", "OBS_BL2", "BEGIN", prev, "END", last)})
	bl := W("BASELINE_NAME", "OBS_BL2")
	h.do(r, "pf.baselines", 'm', stepOpt{where: bl, vals: W("NEW", "OBS_BL3")})
	h.do(r, "pf.baselines", 'r', stepOpt{where: W("BASELINE_NAME", "OBS_BL3")})
	h.do(r, "pf.baselines", 'w', stepOpt{vals: W("DAYS", "8")})
	h.do(r, "pf.baselines", 'x', stepOpt{where: W("BASELINE_NAME", "OBS_BL3")})
	h.do(r, "pf.baselines", 'x', stepOpt{where: W("BASELINE_NAME", "OBS_BL1")})
	h.do(r, "pf.templates", 'n', stepOpt{vals: W("NAME", "OBS_TPL1", "BASE", "OBS_TPL1_BL")})
	h.do(r, "pf.templates", 'p', stepOpt{vals: W("NAME", "OBS_TPL2")})
	h.do(r, "pf.templates", 'x', stepOpt{where: W("TEMPLATE_NAME", "OBS_TPL1")})
	h.do(r, "pf.templates", 'x', stepOpt{where: W("TEMPLATE_NAME", "OBS_TPL2")})
	h.do(r, "pf.addm", 'n', stepOpt{vals: W("BEGIN", prev, "END", last)})
	h.do(r, "pf.addm", 'r', stepOpt{where: map[string]string{}})
	h.do(r, "pf.addm", 'v', stepOpt{where: map[string]string{}})
	h.do(r, "pf.addm", 'p', stepOpt{})
	h.do(r, "pf.addm", 'x', stepOpt{where: map[string]string{}})
	h.do(r, "pf.ash.events", 'r', stepOpt{})
	h.do(r, "pf.ash.sql", 'r', stepOpt{})
	h.do(r, "pf.ash.sql", 'p', stepOpt{where: map[string]string{}})
	h.do(r, "pf.snaps", 'x', stepOpt{where: W("SNAP_ID", prev), vals: W("LOW", prev, "HIGH", prev)})
	// SQL Monitor, Tuning, SPM (na PDB, onde rodou o obs_topsql)
	c := h.pdb
	h.do(c, "pf.sqlmon", 'r', stepOpt{where: W("~SQL_TEXT", "obs_topsql")})
	h.do(c, "pf.sqltune", 'r', stepOpt{where: map[string]string{}})
	h.do(c, "pf.sqltune", 'x', stepOpt{where: map[string]string{}})
	spm := W("~SQL_TEXT", "/* obs_topsql */")
	h.do(c, "pf.spm", 'p', stepOpt{where: spm})
	h.do(c, "pf.spm", 'e', stepOpt{where: spm, vals: W("V", "NO")})
	h.do(c, "pf.spm", 'e', stepOpt{where: spm, vals: W("V", "YES")})
	h.do(c, "pf.spm", 'f', stepOpt{where: spm, vals: W("V", "YES")})
	h.do(c, "pf.spm", 'v', stepOpt{where: spm})
	h.do(c, "pf.spm", 'x', stepOpt{where: spm})
	h.sql(c, "begin dbms_stats.delete_table_stats('OBS_APP', 'T1'); end;\n/")
	st := W("OWNER", "OBS_APP", "TABLE_NAME", "T1")
	h.do(c, "pf.stats", 'l', stepOpt{where: st, vals: W("OP", "LOCK")})
	h.do(c, "pf.stats", 'l', stepOpt{where: st, vals: W("OP", "UNLOCK")})
	h.do(c, "pf.stats", 's', stepOpt{where: st})
	h.sql(c, "begin dbms_stats.delete_table_stats('OBS_APP', 'T1'); end;\n/")
	h.do(c, "pf.stats", 'g', stepOpt{where: st})
	h.do(c, "pf.stats", 'd', stepOpt{})
	// Statspack (instala no PDB, tira dois snapshots, apaga)
	out, err := exec.Command("docker", "exec", "-i", "obs-oracle-free", "bash", "-c",
		`sqlplus -s / as sysdba <<'E'
alter session set container = FREEPDB1;
define default_tablespace = 'SYSAUX'
define temporary_tablespace = 'TEMP'
define perfstat_password = 'Obs2026x'
set termout off
@?/rdbms/admin/spcreate.sql
E`).CombinedOutput()
	if err != nil {
		h.log("  statspack: %v %s", err, lastLines(string(out), 3))
	}
	h.do(c, "pf.statspack", 'n', stepOpt{vals: W("LEVEL", "5")})
	h.do(c, "pf.statspack", 'n', stepOpt{vals: W("LEVEL", "7")})
	h.do(c, "pf.statspack", 'x', stepOpt{where: map[string]string{}})
}

func liveDataPump(h *liveH) {
	c := h.pdb
	h.sql(c, `begin
  for t in (select owner, table_name from dba_tables where table_name in ('OBS_EXP', 'OBS_IMP', 'OBS_ORFAO')) loop
    execute immediate 'drop table "' || t.owner || '"."' || t.table_name || '" purge';
  end loop;
end;
/`)
	h.do(c, "dp.jobs", 'e', stepOpt{vals: W("MODE", "SCHEMA", "OBJECTS", "OBS_APP", "DIR", "OBS_DIR", "DUMP", "obs_%U.dmp", "LOG", "obs_exp.log", "JOB", "OBS_EXP", "REUSE", "1")})
	time.Sleep(time.Second)
	j := W("JOB_NAME", "OBS_EXP")
	if h.do(c, "dp.jobs", 'p', stepOpt{where: j}) {
		time.Sleep(3 * time.Second)
		h.do(c, "dp.jobs", 'r', stepOpt{where: j})
		h.do(c, "dp.jobs", 'g', stepOpt{where: j, vals: W("N", "1")})
		h.do(c, "dp.jobs", 'v', stepOpt{where: j})
	}
	for i := 0; i < 60; i++ {
		if r := Query(c, "select count(*) from dba_datapump_jobs where job_name = 'OBS_EXP' and state <> 'NOT RUNNING'", nil, 1); len(r.Rows) > 0 && r.Rows[0][0].V == "0" {
			break
		}
		time.Sleep(2 * time.Second)
	}
	h.do(c, "dp.jobs", 'i', stepOpt{vals: W("MODE", "SCHEMA", "OBJECTS", "OBS_APP", "DIR", "OBS_DIR", "DUMP", "obs_%U.dmp", "LOG", "obs_imp.log", "JOB", "OBS_IMP", "REMAP_SCHEMA", "OBS_APP:OBS_APP2")})
	h.do(c, "dp.jobs", 'k', stepOpt{where: W("JOB_NAME", "OBS_IMP")})
	// a sessão que abriu o job precisa sumir para ele ficar órfão (NOT RUNNING)
	if odb, err := ora.Open(liveInfo("FREEPDB1")); err == nil {
		odb.SetMaxOpenConns(1)
		// job que termina e deixa a master table (KEEP_MASTER): fica NOT RUNNING, órfão
		if _, err := odb.Exec(`declare h number; st varchar2(30); begin
  h := dbms_datapump.open('EXPORT', 'TABLE', job_name => 'OBS_ORFAO');
  dbms_datapump.add_file(h, 'obs_orfao.dmp', 'OBS_DIR', reusefile => 1);
  dbms_datapump.metadata_filter(h, 'SCHEMA_EXPR', 'IN (''OBS_APP'')');
  dbms_datapump.metadata_filter(h, 'NAME_EXPR', 'IN (''T1'')');
  dbms_datapump.set_parameter(h, 'KEEP_MASTER', 1);
  dbms_datapump.start_job(h);
  dbms_datapump.wait_for_job(h, st);
end;`); err != nil {
			h.log("  preparo com erro: %v", err)
		}
		ora.Close(odb)
	}
	for i := 0; i < 30; i++ { // o job fica DEFINING até a sessão que abriu sumir
		if r := Query(c, "select state from dba_datapump_jobs where job_name = 'OBS_ORFAO'", nil, 1); len(r.Rows) > 0 && r.Rows[0][0].V == "NOT RUNNING" {
			break
		}
		time.Sleep(2 * time.Second)
	}
	h.do(c, "dp.jobs", 'x', stepOpt{where: W("JOB_NAME", "OBS_ORFAO")})
}

func liveDatabase(h *liveH) {
	c, r := h.pdb, h.cdb
	h.do(c, "db.params", 'e', stepOpt{where: W("NAME", "open_cursors"), vals: W("VALUE", "400", "SCOPE", "MEMORY")})
	h.do(c, "db.params.mod", 'e', stepOpt{where: W("NAME", "open_cursors"), vals: W("VALUE", "300", "SCOPE", "MEMORY")})
	h.do(c, "db.params", 's', stepOpt{where: W("NAME", "optimizer_index_cost_adj"), vals: W("VALUE", "100")})
	h.sql(r, "alter system set resumable_timeout = 60 scope = both;")
	h.do(r, "db.params", 'r', stepOpt{where: W("NAME", "resumable_timeout"), vals: W("SCOPE", "SPFILE")})
	h.sql(r, "alter system set resumable_timeout = 60 scope = both;")
	h.do(r, "db.params", 'p', stepOpt{vals: W("FILE", "/tmp/init_obs.ora")})
	h.do(r, "db.params", 'm', stepOpt{vals: W("FILE", "/tmp/spfile_obs.ora")})
	for _, k := range "rpm" {
		h.do(r, "db.params.mod", k, stepOpt{where: W("NAME", "resumable_timeout"), vals: W("SCOPE", "SPFILE", "FILE", "/tmp/obs_"+string(k)+".ora")})
	}
	h.skip("sec.auditcfg s", "nenhum parâmetro de auditoria muda por sessão (ISSES_MODIFIABLE = FALSE)")
	h.sql(r, "alter system set audit_sys_operations = true scope = spfile;")
	for _, k := range "rpm" {
		h.do(r, "sec.auditcfg", k, stepOpt{where: W("NAME", "audit_sys_operations"), vals: W("SCOPE", "SPFILE", "FILE", "/tmp/obs_a"+string(k)+".ora")})
	}
	h.do(c, "db.services", 'n', stepOpt{vals: W("NAME", "OBS_SVC")})
	sv := W("NAME", "OBS_SVC")
	h.do(c, "db.services", 'p', stepOpt{where: sv})
	h.do(c, "db.services", 's', stepOpt{where: sv})
	h.do(c, "db.services", 'p', stepOpt{where: sv})
	h.do(c, "db.services", 'x', stepOpt{where: sv})
	h.do(r, "db.instance", 'f', stepOpt{vals: W("WHAT", "SHARED_POOL")})
	h.do(r, "db.instance", 'f', stepOpt{vals: W("WHAT", "BUFFER_CACHE")})
	h.do(r, "db.instance", 'r', stepOpt{vals: W("MODE", "ENABLE")})
	h.do(r, "db.instance", 'r', stepOpt{vals: W("MODE", "DISABLE")})
	h.do(r, "db.instance", 's', stepOpt{vals: W("MODE", "SUSPEND")})
	h.do(r, "db.instance", 's', stepOpt{vals: W("MODE", "RESUME")})
	h.skip("db.instance q", "o 26ai recusa QUIESCE no CDB$ROOT (ORA-25510); gera o comando certo")
	h.do(r, "db.database", 'l', stepOpt{vals: W("MODE", "ON")})
	h.do(r, "db.database", 'l', stepOpt{vals: W("MODE", "OFF")})
	h.do(r, "db.database", 'g', stepOpt{vals: W("MODE", "ADD")})
	h.do(r, "db.database", 'g', stepOpt{vals: W("MODE", "DROP")})
	h.skip("db.database n", "renomear o global_name não volta atrás sem mexer no dicionário (props$); gera o comando certo (catalog_test)")
	// reinicia pelo RMAN (o mesmo caminho do ARCHIVELOG)
	h.do(r, "db.database", 'S', stepOpt{vals: W("OP", "RESTART")})
	h.reconnect()
}

func liveBackup(h *liveH) {
	r := h.cdb
	h.do(r, "bk.fra", 'd', stepOpt{vals: W("DEST", "/opt/oracle/oradata/fra", "SIZE", "5G")})
	h.do(r, "bk.fra", 'z', stepOpt{vals: W("SIZE", "6G")})
	h.do(r, "bk.fra", 'm', stepOpt{vals: W("SEC", "300")})
	h.do(r, "bk.fra", 't', stepOpt{vals: W("MIN", "60")})
	h.do(r, "bk.fra", 'f', stepOpt{vals: W("FMT", "%t_%s_%r.arc")})
	h.do(r, "bk.fra", 'u', stepOpt{where: map[string]string{}})
	if h.do(r, "bk.fra", 'A', stepOpt{}) {
		h.reconnect()
		r = h.cdb
	}
	h.skip("db.database A", "mesma ação de bk.fra A (testada lá)")
	h.do(r, "bk.fra", 'b', stepOpt{vals: W("MODE", "ON")})
	h.do(r, "db.database", 'f', stepOpt{vals: W("MODE", "OFF")})
	h.do(r, "db.database", 'f', stepOpt{vals: W("MODE", "ON")})
	// configuração do RMAN
	h.do(r, "bk.config", 'w', stepOpt{})
	h.do(r, "bk.config", 'r', stepOpt{vals: W("TYPE", "REDUNDANCY", "N", "1")})
	h.do(r, "bk.config", 'o', stepOpt{vals: W("MODE", "ON")})
	h.do(r, "bk.config", 'd', stepOpt{vals: W("DEV", "DISK")})
	h.do(r, "bk.config", 'p', stepOpt{vals: W("N", "1")})
	h.do(r, "bk.config", 'c', stepOpt{vals: W("ALG", "BASIC")})
	h.do(r, "bk.config", 'a', stepOpt{vals: W("MODE", "ON")})
	h.do(r, "bk.config", 'f', stepOpt{vals: W("FORMAT", "/opt/oracle/oradata/bkp/%d_%T_%U")})
	h.do(r, "bk.config", 't', stepOpt{})
	h.do(r, "bk.config", 'l', stepOpt{vals: W("POL", "NONE")})
	h.do(r, "bk.config", 'e', stepOpt{vals: W("MODE", "OFF")})
	h.do(r, "bk.config", 's', stepOpt{vals: W("FILE", "/opt/oracle/oradata/bkp/snapcf_free.f")})
	h.do(r, "bk.config", 'x', stepOpt{where: W("~NAME", "RETENTION")})
	// backups
	h.do(r, "bk.jobs", 't', stepOpt{vals: W("TS", "USERS", "TAG", "OBS_TS")})
	h.do(r, "bk.jobs", 'b', stepOpt{vals: W("TAG", "OBS_FULL")})
	h.do(r, "bk.jobs", 'f', stepOpt{vals: W("FILES", "1", "TAG", "OBS_DF")})
	h.do(r, "bk.jobs", 'a', stepOpt{vals: W("TAG", "OBS_ARCH")})
	h.do(r, "bk.jobs", 'c', stepOpt{vals: W("SCRIPT", "BACKUP AS COPY DATAFILE 7 TAG OBS_COPY;")})
	h.skip("bk.jobs r", "manda a FRA para fita (SBT): precisa de uma biblioteca de mídia")
	for _, k := range "kRLge" {
		h.do(r, "bk.jobs", k, stepOpt{})
	}
	h.do(r, "bk.jobs", 'v', stepOpt{where: map[string]string{}})
	set := W("~TAG", "OBS_DF")
	for _, k := range "kve" {
		h.do(r, "bk.sets", k, stepOpt{where: set})
	}
	h.do(r, "bk.sets", 'u', stepOpt{where: set, vals: W("ST", "UNAVAILABLE")})
	h.do(r, "bk.sets", 'u', stepOpt{where: set, vals: W("ST", "AVAILABLE")})
	h.do(r, "bk.sets", 'p', stepOpt{where: W("~TAG", "OBS_TS"), vals: W("MODE", "NOKEEP")})
	cp := W("~TAG", "OBS_COPY")
	h.do(r, "bk.copies", 'k', stepOpt{where: cp})
	h.do(r, "bk.copies", 'v', stepOpt{where: cp})
	h.do(r, "bk.copies", 'u', stepOpt{where: cp, vals: W("ST", "UNAVAILABLE")})
	h.do(r, "bk.copies", 'u', stepOpt{where: cp, vals: W("ST", "AVAILABLE")})
	h.do(r, "bk.copies", 's', stepOpt{where: cp})
	h.do(r, "bk.copies", 'x', stepOpt{where: map[string]string{}})
	// recuperação
	h.do(r, "bk.recovery", 'V', stepOpt{})
	h.do(r, "bk.recovery", 'P', stepOpt{})
	// archives apagados sem backup em rodadas antigas: tira do repositório antes do VALIDATE
	h.rman(r, "CROSSCHECK ARCHIVELOG ALL;\nDELETE NOPROMPT EXPIRED ARCHIVELOG ALL;\nBACKUP ARCHIVELOG ALL NOT BACKED UP 1 TIMES;")
	h.do(r, "bk.recovery", 'T', stepOpt{})
	h.do(r, "bk.recovery", 'C', stepOpt{})
	h.skip("bk.recovery X", "o Data Recovery Advisor saiu no 23ai (RMAN não conhece LIST FAILURE); o nome da ação avisa")
	h.skip("bk.recovery B", "precisa de um bloco corrompido na lista (V$DATABASE_BLOCK_CORRUPTION)")
	h.do(r, "bk.recovery", 'S', stepOpt{vals: W("TS", "USERS")})
	h.do(r, "bk.recovery", 'F', stepOpt{vals: W("FILE", "7")})
	seq := Query(r, "select min(sequence#), max(sequence#) from v$archived_log where deleted = 'NO' and resetlogs_change# = (select resetlogs_change# from v$database)", nil, 1)
	if len(seq.Rows) > 0 && seq.Rows[0][0].V != "" {
		h.do(r, "bk.recovery", 'A', stepOpt{vals: W("FROM", seq.Rows[0][1].V, "TO", seq.Rows[0][1].V, "DEST", "/opt/oracle/oradata/bkp")})
	}
	h.skip("bk.recovery R", "RECOVER TABLE sobe uma instância auxiliar (~1,5 GB de RAM): não cabe neste host")
	h.skip("bk.recovery D", "restore do banco inteiro: coberto em partes (tablespace, datafile, archives, validate, preview)")
	// restore point garantido + flashback do banco (reinicia)
	h.do(r, "st.restorepoints", 'n', stepOpt{vals: W("NAME", "OBS_GRP", "G", "1")})
	h.sql(h.pdb, "create table obs_app.depois_do_rp (id number);")
	if h.do(r, "st.restorepoints", 'F', stepOpt{where: W("NAME", "OBS_GRP")}) {
		h.reconnect()
		r = h.cdb
		if n := Query(h.pdb, "select count(*) from dba_tables where owner = 'OBS_APP' and table_name = 'DEPOIS_DO_RP'", nil, 1); len(n.Rows) > 0 && n.Rows[0][0].V != "0" {
			h.fail("st.restorepoints F", "o flashback não voltou: a tabela criada depois do ponto continua lá")
		}
	}
	h.do(r, "st.restorepoints", 'n', stepOpt{vals: W("NAME", "OBS_GRP2", "G", "1")})
	if h.do(r, "bk.recovery", 'L', stepOpt{vals: W("TO", "RESTORE POINT", "VALUE", "OBS_GRP2")}) {
		h.reconnect()
		r = h.cdb
	}
	h.do(r, "st.restorepoints", 'x', stepOpt{where: W("NAME", "OBS_GRP2")})
	h.do(r, "st.restorepoints", 'x', stepOpt{where: W("NAME", "OBS_GRP")})
	h.do(r, "st.redo", 'a', stepOpt{})
	h.do(r, "st.archdest", 'e', stepOpt{where: W("DEST_ID", "1"), vals: W("VALUE", "LOCATION=USE_DB_RECOVERY_FILE_DEST")})
	h.do(r, "st.archdest", 's', stepOpt{where: W("DEST_ID", "1"), vals: W("STATE", "ENABLE")})
	// apagar
	h.do(r, "bk.sets", 'x', stepOpt{where: W("~TAG", "OBS_DF")})
	h.do(r, "bk.jobs", 'o', stepOpt{})
	// volta ao NOARCHIVELOG
	if h.do(r, "bk.fra", 'N', stepOpt{}) {
		h.reconnect()
	}
	h.skip("db.database N", "mesma ação de bk.fra N (testada lá)")
}

func livePDBs(h *liveH) {
	r := h.cdb
	h.sql(r, "alter system set db_create_file_dest = '/opt/oracle/oradata' scope = both;")
	h.sql(r, `begin
  for p in (select name from v$pdbs where name like 'OBSPDB%') loop
    begin execute immediate 'alter pluggable database ' || p.name || ' close immediate'; exception when others then null; end;
    execute immediate 'drop pluggable database ' || p.name || ' including datafiles';
  end loop;
end;
/`)
	_ = exec.Command("docker", "exec", "obs-oracle-free", "rm", "-f", "/tmp/obspdb2.xml").Run()
	h.do(r, "db.pdbs", 'n', stepOpt{vals: W("NAME", "OBSPDB", "PWD", "Obs#2026x")})
	p := W("NAME", "OBSPDB")
	h.do(r, "db.pdbs", 'c', stepOpt{where: p})
	h.do(r, "db.pdbs", 'o', stepOpt{where: p, vals: W("MODE", "READ ONLY")})
	h.do(r, "db.pdbs", 'c', stepOpt{where: p})
	h.do(r, "db.pdbs", 'o', stepOpt{where: p, vals: W("MODE", "READ WRITE")})
	h.do(r, "db.pdbs", 's', stepOpt{where: p, vals: W("MODE", "SAVE")})
	h.do(r, "db.pdbs", 's', stepOpt{where: p, vals: W("MODE", "DISCARD")})
	h.do(r, "db.pdbs", 'k', stepOpt{where: p, vals: W("NAME", "OBSPDB2")})
	p2 := W("NAME", "OBSPDB2")
	h.sql(r, "begin execute immediate 'alter pluggable database obspdb2 open'; exception when others then null; end;\n/")
	h.do(r, "db.pdbs", 'c', stepOpt{where: p2})
	h.do(r, "db.pdbs", 'u', stepOpt{where: p2, vals: W("XML", "/tmp/obspdb2.xml")})
	h.do(r, "db.pdbs", 'x', stepOpt{where: p2, vals: W("FILES", "KEEP DATAFILES")})
	h.do(r, "db.pdbs", 'p', stepOpt{vals: W("NAME", "OBSPDB2", "XML", "/tmp/obspdb2.xml", "COPY", "NOCOPY")})
	h.skip("db.pdbs r", "precisa de um PDB refreshable (clone remoto com REFRESH MODE)")
	h.skip("db.pdbs l", "relocate precisa de outro CDB e um database link")
	for _, n := range []string{"OBSPDB2", "OBSPDB"} {
		h.do(r, "db.pdbs", 'x', stepOpt{where: W("NAME", n), vals: W("FILES", "INCLUDING DATAFILES")})
	}
}
