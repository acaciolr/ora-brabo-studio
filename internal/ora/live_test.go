// Testes contra um Oracle real: rodam só com OBS_TEST_PASSWORD (e OBS_TEST_HOST/PORT/SERVICE).
// by • DBA BRABO
package ora

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func liveConn(t *testing.T, user, role string) ConnInfo {
	pw := os.Getenv("OBS_TEST_PASSWORD")
	if pw == "" {
		t.Skip("sem OBS_TEST_PASSWORD")
	}
	port, _ := strconv.Atoi(os.Getenv("OBS_TEST_PORT"))
	if port == 0 {
		port = 15210
	}
	svc := os.Getenv("OBS_TEST_SERVICE")
	if svc == "" {
		svc = "FREEPDB1"
	}
	host := os.Getenv("OBS_TEST_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	return ConnInfo{Name: "teste", User: user, Password: pw, Host: host, Port: port, Service: svc, Role: role}
}

func run(t *testing.T, s *Session, script string) []Result {
	var out []Result
	for _, st := range Split(script) {
		r := s.Run(st, nil)
		out = append(out, r)
	}
	return out
}

func TestLiveWorksheet(t *testing.T) {
	db, err := Open(liveConn(t, "system", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s, err := NewSession(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	res := run(t, s, `begin execute immediate 'drop table obs_t purge'; exception when others then null; end;
/
create table obs_t (id number(10) primary key, nome varchar2(30), criado date default sysdate, valor number(12,2), bin raw(4));
insert into obs_t (id, nome, valor, bin) values (1, 'um', 1.5, hextoraw('CAFE'));
insert into obs_t (id, nome) values (2, 'dois');
select id, nome, valor, bin, criado from obs_t order by id;
begin dbms_output.put_line('ola do dbms_output'); end;
/
create or replace procedure obs_p as begin nul; end;
/
create or replace procedure obs_ok as begin null; end;
/
desc obs_t
show errors procedure obs_p
insert into obs_t (id) values (3);
rollback;
select count(*) n from obs_t;
exec dbms_output.put_line('exec ok')
explicar;`)
	for i, r := range res {
		t.Logf("%d %q -> err=%v msg=%q text=%q out=%v rows=%v", i, r.Stmt.Text[:min(len(r.Stmt.Text), 40)], r.Err, r.Msg, r.Text, r.Output, r.Rows)
	}
	if res[0].Err != nil || res[1].Err != nil || res[2].Err != nil {
		t.Fatal("criação / insert falhou")
	}
	q := res[4]
	if q.Err != nil || len(q.Rows) != 2 || q.Rows[0][1].V != "um" || q.Rows[0][2].V != "1.5" || q.Rows[0][3].V != "CAFE" || !q.Rows[1][2].Null {
		t.Fatalf("select: %+v", q)
	}
	if len(res[5].Output) != 1 || res[5].Output[0] != "ola do dbms_output" {
		t.Fatalf("dbms_output: %#v", res[5].Output)
	}
	if res[6].Err == nil || !strings.Contains(res[6].Text, "PLS-") {
		t.Fatalf("procedure com erro deveria acusar: %+v", res[6])
	}
	if res[7].Err != nil {
		t.Fatalf("procedure ok: %v", res[7].Err)
	}
	if !strings.Contains(res[8].Text, "NOME") || !strings.Contains(res[8].Text, "NUMBER(10)") || !strings.Contains(res[8].Text, "NOT NULL") {
		t.Fatalf("desc: %q", res[8].Text)
	}
	if !strings.Contains(res[9].Text, "PLS-") {
		t.Fatalf("show errors: %q", res[9].Text)
	}
	// o CREATE PROCEDURE fez commit implícito dos inserts 1 e 2; o rollback desfaz só o 3
	if res[12].Rows[0][0].V != "2" {
		t.Fatalf("rollback não desfez: %v", res[12].Rows)
	}
	if len(res[13].Output) != 1 || res[13].Output[0] != "exec ok" {
		t.Fatalf("exec: %+v", res[13])
	}
	if res[1].Msg != L("Tabela criada.", "Table created.") {
		t.Fatalf("mensagem do DDL: %q", res[1].Msg)
	}
	if res[14].Err == nil {
		t.Fatal("comando inválido deveria dar erro")
	}
	st := Split("select * from obs_t where id = :id")[0]
	e := s.Explain(st)
	if e.Err != nil || !strings.Contains(e.Text, "OBS_T") {
		t.Fatalf("explain: %v %q", e.Err, e.Text)
	}
	a := s.Autotrace(st, []any{1})
	if a.Err != nil || !strings.Contains(a.Text, "consistent gets") || !strings.Contains(a.Text, "A-Rows") {
		t.Fatalf("autotrace: %v %q", a.Err, a.Text)
	}
	t.Log(a.Text)
	run(t, s, "drop table obs_t purge;\ndrop procedure obs_p;\ndrop procedure obs_ok;")
}

// TestLiveConnTypes conecta de verdade por cada tipo que dá para montar só com
// o banco de teste: básico (serviço e SID), TNS (pasta com tnsnames.ora), URL
// personalizada (EZConnect, SID e descritor), proxy e túnel SSH (sshd local,
// se a chave do usuário entrar em localhost).
func TestLiveConnTypes(t *testing.T) {
	base := liveConn(t, "system", "")
	DataDir = t.TempDir()
	who := func(c ConnInfo) (string, error) {
		db, err := Open(c)
		if err != nil {
			return "", err
		}
		defer Close(db)
		var u, con string
		err = db.QueryRow("select sys_context('userenv', 'session_user'), sys_context('userenv', 'con_name') from dual").Scan(&u, &con)
		return u + "@" + con, err
	}
	check := func(name string, c ConnInfo, want string) {
		t.Helper()
		got, err := who(c)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			return
		}
		if got != want {
			t.Errorf("%s: conectou como %s, queria %s", name, got, want)
			return
		}
		t.Logf("ok  %-28s %s", name, got)
	}
	port := strconv.Itoa(base.Port)
	check("básico (serviço)", base, "SYSTEM@FREEPDB1")
	sid := base
	sid.Service, sid.SID = "", "FREE"
	check("básico (SID)", sid, "SYSTEM@CDB$ROOT")
	sysdba := base
	sysdba.User, sysdba.Role = "sys", "SYSDBA"
	check("básico SYSDBA", sysdba, "SYS@FREEPDB1")

	dir := t.TempDir()
	tns := "OBSPDB =\n  (DESCRIPTION =\n    (ADDRESS = (PROTOCOL = TCP)(HOST = " + base.Host + ")(PORT = " + port + "))\n    (CONNECT_DATA = (SERVICE_NAME = FREEPDB1)))\n" +
		"OBSCDB.MUNDO = (DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=" + base.Host + ")(PORT=" + port + "))(CONNECT_DATA=(SERVICE_NAME=FREE)))\n"
	if err := os.WriteFile(dir+"/tnsnames.ora", []byte(tns), 0o644); err != nil {
		t.Fatal(err)
	}
	tc := ConnInfo{Name: "tns", Type: TypeTNS, TNSAdmin: dir, TNS: "OBSPDB", User: "system", Password: base.Password}
	check("TNS (alias)", tc, "SYSTEM@FREEPDB1")
	tc.TNS = "obscdb.mundo"
	check("TNS (alias com domínio)", tc, "SYSTEM@CDB$ROOT")
	tc.TNS = "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=" + base.Host + ")(PORT=" + port + "))(CONNECT_DATA=(SERVICE_NAME=FREEPDB1)))"
	check("TNS (descritor)", tc, "SYSTEM@FREEPDB1")

	for _, u := range []string{
		"jdbc:oracle:thin:@//" + base.Host + ":" + port + "/FREEPDB1",
		base.Host + ":" + port + "/FREEPDB1",
		"jdbc:oracle:thin:@" + base.Host + ":" + port + ":FREE",
		"jdbc:oracle:thin:@OBSPDB?TNS_ADMIN=" + dir,
	} {
		want := "SYSTEM@FREEPDB1"
		if strings.HasSuffix(u, ":FREE") {
			want = "SYSTEM@CDB$ROOT"
		}
		check("URL "+u, ConnInfo{Name: "url", Type: TypeCustom, CustomURL: u, User: "system", Password: base.Password}, want)
	}

	// proxy: OBS_PXA entra como OBS_PXB
	db, err := Open(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"begin for u in (select username from dba_users where username in ('OBS_PXA', 'OBS_PXB')) loop execute immediate 'drop user ' || u.username || ' cascade'; end loop; end;",
		`create user obs_pxa identified by "Obs#2026x"`, `create user obs_pxb identified by "Obs#2026y"`,
		"grant create session to obs_pxa, obs_pxb", "alter user obs_pxb grant connect through obs_pxa",
	} {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	Close(db)
	px := base
	px.User, px.Password, px.Proxy = "obs_pxa", "Obs#2026x", "obs_pxb"
	check("proxy (obs_pxa[obs_pxb])", px, "OBS_PXB@FREEPDB1")

	// túnel SSH pelo sshd desta máquina
	if u := os.Getenv("USER"); u != "" && os.Getenv("OBS_TEST_SSH") != "" {
		st := base
		st.SSHHost, st.SSHPort, st.SSHUser = "127.0.0.1", 22, u
		check("túnel SSH (127.0.0.1:22)", st, "SYSTEM@FREEPDB1")
		if _, err := os.Stat(DataDir + "/known_hosts"); err != nil {
			t.Errorf("túnel SSH: não gravou o known_hosts: %v", err)
		}
	}
}
