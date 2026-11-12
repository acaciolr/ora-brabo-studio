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

func TestImportSQLDevTypes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "connections.json")
	_ = os.WriteFile(p, []byte(`{"connections":[
 {"name":"tns","type":"jdbc","info":{"user":"hr","OracleConnectionType":"TNS","customUrl":"jdbc:oracle:thin:@PRODTNS"}},
 {"name":"ldap","type":"jdbc","info":{"user":"hr","OracleConnectionType":"LDAP","customUrl":"jdbc:oracle:thin:@ldap://oid.corp:389/orcl,cn=OracleContext,dc=corp,dc=com"}},
 {"name":"url","type":"jdbc","info":{"user":"hr","OracleConnectionType":"ADVANCED","customUrl":"jdbc:oracle:thin:@//db9:1600/svc9"}},
 {"name":"adb","type":"jdbc","info":{"user":"ADMIN","OracleConnectionType":"CLOUD","customUrl":"jdbc:oracle:thin:@adb_high?TNS_ADMIN=/w","Cloud_Wallet_File":"/home/x/Wallet_adb.zip"}},
 {"name":"ext","type":"jdbc","info":{"OS_AUTHENTICATION":"true","hostname":"db1","port":"1521","serviceName":"S"}},
 {"name":"basic","type":"jdbc","info":{"user":"hr","OracleConnectionType":"BASIC","hostname":"db1","port":"1521","serviceName":"S","customUrl":"jdbc:oracle:thin:@//db1:1521/S"}},
 {"name":"prx","type":"jdbc","info":{"user":"pool","PROXY_USER_NAME":"SCOTT","KERBEROS_AUTHENTICATION":"true","hostname":"db1","port":"1521","serviceName":"S"}}
]}`), 0o600)
	cs, err := ImportSQLDev(p)
	if err != nil || len(cs) != 7 {
		t.Fatalf("%v %d", err, len(cs))
	}
	m := map[string]ora.ConnInfo{}
	for _, c := range cs {
		m[c.Name] = c
	}
	if c := m["tns"]; c.Kind() != ora.TypeTNS || c.TNS != "PRODTNS" {
		t.Fatalf("tns %+v", c)
	}
	if c := m["ldap"]; c.Kind() != ora.TypeLDAP || c.LDAPServer != "oid.corp:389" || c.TNS != "orcl" || c.LDAPContext != "dc=corp,dc=com" {
		t.Fatalf("ldap %+v", c)
	}
	if c := m["url"]; c.Kind() != ora.TypeCustom || c.CustomURL != "jdbc:oracle:thin:@//db9:1600/svc9" {
		t.Fatalf("url %+v", c)
	}
	if c := m["adb"]; c.Kind() != ora.TypeCloud || c.CloudZip != "/home/x/Wallet_adb.zip" || c.TNS != "adb_high" {
		t.Fatalf("cloud %+v", c)
	}
	if c := m["ext"]; c.Auth != ora.AuthOS || c.Host != "db1" {
		t.Fatalf("os %+v", c)
	}
	if c := m["basic"]; c.Kind() != ora.TypeBasic || c.Host != "db1" || c.Service != "S" {
		t.Fatalf("basic %+v", c)
	}
	if c := m["prx"]; c.Auth != ora.AuthKerberos || c.Proxy != "SCOTT" {
		t.Fatalf("kerberos/proxy %+v", c)
	}
}

func TestSecretsSealed(t *testing.T) {
	t.Setenv("ORA_BRABO_STUDIO_HOME", t.TempDir())
	c := ora.ConnInfo{Name: "x", User: "u", Password: "p1", WalletPwd: "w1", SSHPwd: "s1", SavePwd: true, Host: "h", Service: "s"}
	if err := SaveConns([]ora.ConnInfo{c}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(Dir(), "connections.json"))
	for _, plain := range []string{`"p1"`, `"w1"`, `"s1"`} {
		if strings.Contains(string(b), plain) {
			t.Fatalf("segredo em claro no arquivo: %s", plain)
		}
	}
	cs, _ := LoadConns()
	if len(cs) != 1 || cs[0].Password != "p1" || cs[0].WalletPwd != "w1" || cs[0].SSHPwd != "s1" {
		t.Fatalf("%+v", cs)
	}
	c.SavePwd = false
	_ = SaveConns([]ora.ConnInfo{c})
	cs, _ = LoadConns()
	if cs[0].Password != "" || cs[0].WalletPwd != "" || cs[0].SSHPwd != "" {
		t.Fatal("sem 'salvar' não pode guardar segredo")
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
