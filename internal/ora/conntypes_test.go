// Testes dos tipos de conexão (sem banco): a URL do go-ora sai certa para
// básica, TNS, Cloud Wallet, URL personalizada, SO, Kerberos e proxy; e o
// túnel SSH leva a conexão até o destino. by • DBA BRABO
package ora

import (
	"archive/zip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/sijms/go-ora/v2/configurations"
	"golang.org/x/crypto/ssh"
)

func parse(t *testing.T, c ConnInfo) *configurations.ConnectionConfig {
	t.Helper()
	u, err := c.URL()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := configurations.ParseConfig(u)
	if err != nil {
		t.Fatalf("%s: %v", u, err)
	}
	return cfg
}

func TestURLBasicAndAuth(t *testing.T) {
	cfg := parse(t, ConnInfo{User: "hr", Password: "x", Host: "db1", Port: 1522, Service: "PDB1", Role: "SYSBACKUP"})
	if cfg.Servers[0].Addr != "db1" || cfg.Servers[0].Port != 1522 || cfg.ServiceName != "PDB1" || cfg.UserID != "hr" || cfg.DBAPrivilege != configurations.SYSBACKUP {
		t.Fatalf("%+v", cfg.DatabaseInfo)
	}
	cfg = parse(t, ConnInfo{User: "hr", Host: "db1", SID: "ORCL"})
	if cfg.SID != "ORCL" {
		t.Fatal(cfg.SID)
	}
	cfg = parse(t, ConnInfo{Auth: AuthOS, Host: "db1", Service: "s"})
	if cfg.AuthType != configurations.OS {
		t.Fatal("auth os")
	}
	cfg = parse(t, ConnInfo{Auth: AuthKerberos, Host: "db1", Service: "s"})
	if !strings.Contains(strings.Join(cfg.AuthService, ","), "KERBEROS5") {
		t.Fatal("auth kerberos")
	}
	cfg = parse(t, ConnInfo{User: "app_pool", Password: "x", Proxy: "SCOTT", Host: "db1", Service: "s", Role: "SYSASM"})
	if cfg.ProxyClientName != "SCOTT" || cfg.DBAPrivilege != configurations.SYSASM {
		t.Fatalf("proxy %q %v", cfg.ProxyClientName, cfg.DBAPrivilege)
	}
	if _, err := (ConnInfo{User: "hr", Host: "db1"}).URL(); err == nil {
		t.Fatal("sem serviço devia falhar")
	}
}

func TestURLTNSFolder(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "tnsnames.ora"), []byte("HML = (DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=h2)(PORT=1530))(CONNECT_DATA=(SERVICE_NAME=hml)))\n"), 0o600)
	cfg := parse(t, ConnInfo{Type: TypeTNS, User: "hr", TNS: "hml", TNSAdmin: dir})
	if cfg.Servers[0].Addr != "h2" || cfg.Servers[0].Port != 1530 || cfg.ServiceName != "hml" {
		t.Fatalf("%+v", cfg.DatabaseInfo)
	}
	cfg = parse(t, ConnInfo{Type: TypeTNS, User: "hr", TNS: "(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=h3)(PORT=1521))(CONNECT_DATA=(SID=X)))"})
	if cfg.Servers[0].Addr != "h3" || cfg.SID != "X" {
		t.Fatalf("%+v", cfg.DatabaseInfo)
	}
	// conexão antiga, sem Type, com TNS preenchido continua TNS
	if (ConnInfo{TNS: "x"}).Kind() != TypeTNS {
		t.Fatal("kind")
	}
}

func TestURLCustom(t *testing.T) {
	for _, u := range []string{
		"jdbc:oracle:thin:@//db9:1600/svc9",
		"jdbc:oracle:thin:@db9:1600/svc9",
		"db9:1600/svc9",
		"jdbc:oracle:thin:@(DESCRIPTION=(ADDRESS=(PROTOCOL=TCP)(HOST=db9)(PORT=1600))(CONNECT_DATA=(SERVICE_NAME=svc9)))",
	} {
		cfg := parse(t, ConnInfo{Type: TypeCustom, User: "hr", Password: "x", CustomURL: u})
		if len(cfg.Servers) == 0 || cfg.Servers[0].Addr != "db9" || cfg.Servers[0].Port != 1600 || cfg.ServiceName != "svc9" {
			t.Fatalf("%s → %+v", u, cfg.DatabaseInfo)
		}
	}
	cfg := parse(t, ConnInfo{Type: TypeCustom, User: "hr", CustomURL: "jdbc:oracle:thin:@db9:1600:ORCL"})
	if cfg.SID != "ORCL" || cfg.Servers[0].Port != 1600 {
		t.Fatalf("host:porta:SID → %+v", cfg.DatabaseInfo)
	}
	cfg = parse(t, ConnInfo{Type: TypeCustom, CustomURL: "jdbc:oracle:thin:scott/tiger@db9:1600/svc9"})
	if cfg.UserID != "scott" || cfg.Password != "tiger" {
		t.Fatalf("credenciais da URL: %q", cfg.UserID)
	}
	cfg = parse(t, ConnInfo{Type: TypeCustom, CustomURL: "oracle://u:p@db9:1600/svc9"})
	if cfg.UserID != "u" || cfg.ServiceName != "svc9" {
		t.Fatal("oracle://")
	}
}

func TestCloudWallet(t *testing.T) {
	DataDir = t.TempDir()
	defer func() { DataDir = "" }()
	zp := filepath.Join(t.TempDir(), "Wallet_ADB1.zip")
	f, _ := os.Create(zp)
	z := zip.NewWriter(f)
	for name, body := range map[string]string{
		"tnsnames.ora":     "adb1_high = (description=(address=(protocol=tcps)(port=1522)(host=adb.sa-saopaulo-1.oraclecloud.com))(connect_data=(service_name=x_adb1_high.adb.oraclecloud.com))(security=(ssl_server_dn_match=yes)))\nadb1_low = (description=(address=(protocol=tcps)(port=1522)(host=adb.sa-saopaulo-1.oraclecloud.com))(connect_data=(service_name=x_adb1_low.adb.oraclecloud.com)))\n",
		"cwallet.sso":      "sso",
		"../../escape.txt": "não pode sair da pasta",
	} {
		w, _ := z.Create(name)
		io.WriteString(w, body)
	}
	z.Close()
	f.Close()
	svcs, err := CloudServices(zp)
	if err != nil || strings.Join(svcs, ",") != "ADB1_HIGH,ADB1_LOW" {
		t.Fatalf("%v %v", svcs, err)
	}
	c := ConnInfo{Type: TypeCloud, User: "ADMIN", Password: "x", CloudZip: zp, TNS: "adb1_high"}
	u, err := c.URL()
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := ExtractCloudWallet(zp, "")
	if _, err := os.Stat(filepath.Join(dir, "cwallet.sso")); err != nil {
		t.Fatal("wallet não extraído")
	}
	if _, err := os.Stat(filepath.Join(dir, "escape.txt")); err != nil {
		t.Fatal("arquivo com caminho devia ficar na pasta do wallet")
	}
	if _, err := os.Stat(filepath.Join(DataDir, "escape.txt")); err == nil {
		t.Fatal("zip-slip: saiu da pasta")
	}
	q, _ := url.Parse(u)
	if q.Query().Get("WALLET") != dir || q.Query().Get("SSL") != "true" || !strings.Contains(q.Query().Get("connStr"), "x_adb1_high") {
		t.Fatal(u)
	}
}

// TestSSHTunnel sobe um servidor SSH de teste com port forward e confere que
// o túnel chega ao destino (um eco TCP); a chave do host fica gravada.
func TestSSHTunnel(t *testing.T) {
	DataDir = t.TempDir()
	defer func() { DataDir = "" }()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SSH_AUTH_SOCK", "")

	echo, _ := net.Listen("tcp", "127.0.0.1:0")
	defer echo.Close()
	go func() {
		for {
			c, err := echo.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()

	_, hk, _ := ed25519.GenerateKey(rand.Reader)
	hs, _ := ssh.NewSignerFromKey(hk)
	cfg := &ssh.ServerConfig{PasswordCallback: func(m ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
		if m.User() == "bastion" && string(p) == "s3nha" {
			return nil, nil
		}
		return nil, io.EOF
	}}
	cfg.AddHostKey(hs)
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_, chans, reqs, err := ssh.NewServerConn(nc, cfg)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					if ch.ChannelType() != "direct-tcpip" {
						ch.Reject(ssh.UnknownChannelType, "")
						continue
					}
					d := ch.ExtraData()
					n := binary.BigEndian.Uint32(d)
					host := string(d[4 : 4+n])
					port := binary.BigEndian.Uint32(d[4+n:])
					dst, err := net.Dial("tcp", net.JoinHostPort(host, itoa(int(port))))
					if err != nil {
						ch.Reject(ssh.ConnectionFailed, err.Error())
						continue
					}
					c, r, _ := ch.Accept()
					go ssh.DiscardRequests(r)
					go func() { io.Copy(c, dst); c.Close() }()
					go func() { io.Copy(dst, c); dst.Close() }()
				}
			}()
		}
	}()

	_, p, _ := net.SplitHostPort(ln.Addr().String())
	tun := newSSHTunnel(ConnInfo{SSHHost: "127.0.0.1", SSHPort: atoi(p), SSHUser: "bastion", SSHPwd: "s3nha"})
	defer tun.Close()
	c, err := tun.DialContext(context.Background(), "tcp", echo.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(c, "ping")
	buf := make([]byte, 4)
	if _, err := io.ReadFull(c, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("%q %v", buf, err)
	}
	c.Close()
	if b, _ := os.ReadFile(KnownHostsFile()); !strings.Contains(string(b), "ssh-ed25519") {
		t.Fatal("chave do host não foi gravada")
	}
	// senha errada falha
	bad := newSSHTunnel(ConnInfo{SSHHost: "127.0.0.1", SSHPort: atoi(p), SSHUser: "bastion", SSHPwd: "errada"})
	if _, err := bad.DialContext(context.Background(), "tcp", echo.Addr().String()); err == nil {
		t.Fatal("senha errada passou")
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }
