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
