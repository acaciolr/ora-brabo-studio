// Testes do armazenamento. by • DBA BRABO
package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acaciolr/ora-brabo-studio/internal/ora"
)

func TestConnsSenhaCifrada(t *testing.T) {
	t.Setenv("ORA_BRABO_STUDIO_HOME", t.TempDir())
	cs := []ora.ConnInfo{
		{Name: "b", User: "system", Password: "segredo123", SavePwd: true, Host: "h", Port: 1521, Service: "FREEPDB1"},
		{Name: "a", User: "hr", Password: "naosalva", Host: "h", Port: 1521, Service: "X"},
	}
	if err := SaveConns(cs); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(Dir(), "connections.json"))
	if strings.Contains(string(raw), "segredo123") || strings.Contains(string(raw), "naosalva") {
		t.Fatal("senha em claro no disco")
	}
	got, err := LoadConns()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "a" || got[1].Password != "segredo123" || got[0].Password != "" {
		t.Fatalf("%#v", got)
	}
}

func TestImportSQLDev(t *testing.T) {
	p := filepath.Join(t.TempDir(), "connections.json")
	_ = os.WriteFile(p, []byte(`{"connections":[{"name":"prod","type":"jdbc","info":{"user":"system","hostname":"db1","port":"1521","serviceName":"ORCL","role":"SYSDBA"}}]}`), 0o600)
	cs, err := ImportSQLDev(p)
	if err != nil || len(cs) != 1 || cs[0].Port != 1521 || cs[0].Role != "SYSDBA" || cs[0].Service != "ORCL" {
		t.Fatalf("%v %#v", err, cs)
	}
}

func TestHist(t *testing.T) {
	t.Setenv("ORA_BRABO_STUDIO_HOME", t.TempDir())
	AddHist(Hist{SQL: "select 1 from dual", OK: true})
	AddHist(Hist{SQL: "select 2 from dual", OK: true})
	h := LoadHist()
	if len(h) != 2 || h[0].SQL != "select 2 from dual" {
		t.Fatalf("%#v", h)
	}
}
