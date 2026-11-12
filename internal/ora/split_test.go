// Testes do divisor de script. by • DBA BRABO
package ora

import "testing"

func TestSplitBasico(t *testing.T) {
	sc := "select 1 from dual;\nselect ';' x from dual;\n\nbegin\n  null;\nend;\n/\nDESC dual\ninsert into t values ('a;b');\n"
	st := Split(sc)
	want := []struct {
		txt  string
		kind Kind
	}{
		{"select 1 from dual", KindQuery},
		{"select ';' x from dual", KindQuery},
		{"begin\n  null;\nend;", KindPLSQL},
		{"DESC dual", KindClient},
		{"insert into t values ('a;b')", KindDML},
	}
	if len(st) != len(want) {
		t.Fatalf("esperava %d comandos, veio %d: %#v", len(want), len(st), st)
	}
	for i, w := range want {
		if st[i].Text != w.txt || st[i].Kind != w.kind {
			t.Errorf("%d: veio %q (%v), esperava %q (%v)", i, st[i].Text, st[i].Kind, w.txt, w.kind)
		}
	}
}

func TestSplitCreateProcedure(t *testing.T) {
	sc := "create or replace procedure p as\nbegin\n  dbms_output.put_line('x;y');\nend;\n/\nselect * from user_errors"
	st := Split(sc)
	if len(st) != 2 || st[0].Kind != KindPLSQL || st[1].Kind != KindQuery {
		t.Fatalf("divisao errada: %#v", st)
	}
}

func TestSplitSemPontoVirgulaLinhaEmBranco(t *testing.T) {
	st := Split("select 1 from dual\n\nselect 2 from dual")
	if len(st) != 2 {
		t.Fatalf("linha em branco deveria separar: %#v", st)
	}
}

func TestSplitComentariosEQQuote(t *testing.T) {
	st := Split("-- comentario;\n/* outro ; */ select q'[a;b]' from dual; select 2 from dual;")
	if len(st) != 2 || st[0].Kind != KindQuery {
		t.Fatalf("%#v", st)
	}
}

func TestAtCursor(t *testing.T) {
	sc := "select 1 from dual;\n\nselect 2 from dual;\n"
	s, ok := AtCursor(sc, 25)
	if !ok || s.Text != "select 2 from dual" {
		t.Fatalf("veio %q", s.Text)
	}
	s, _ = AtCursor(sc, 3)
	if s.Text != "select 1 from dual" {
		t.Fatalf("veio %q", s.Text)
	}
}

func TestBinds(t *testing.T) {
	s := Split("select * from t where a = :x and b = ':y' and c = :x and d = :Z1")[0]
	b := Binds(s)
	if len(b) != 2 || b[0] != "x" || b[1] != "Z1" {
		t.Fatalf("%v", b)
	}
}
