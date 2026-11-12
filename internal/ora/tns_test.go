// Teste do leitor de tnsnames.ora. by • DBA BRABO
package ora

import "testing"

func TestParseTNS(t *testing.T) {
	m := ParseTNS(`# comentario
PROD, PROD_ALT =
  (DESCRIPTION =
    (ADDRESS = (PROTOCOL = TCP)(HOST = db1)(PORT = 1521))
    (CONNECT_DATA = (SERVICE_NAME = orcl)))
dev=(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=d)(PORT=1522))(CONNECT_DATA=(SID=X)))
`)
	if len(m) != 3 || m["PROD"] != m["PROD_ALT"] || m["DEV"] == "" {
		t.Fatalf("%#v", m)
	}
	if m["PROD"] != "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=db1)(PORT=1521))(CONNECT_DATA=(SERVICE_NAME=orcl)))" {
		t.Fatal(m["PROD"])
	}
}
