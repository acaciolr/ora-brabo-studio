// Nomes de serviço fora do tnsnames.ora: Cloud Wallet (.zip do Autonomous
// Database) e diretório LDAP (OID / Active Directory). by • DBA BRABO
package ora

import (
	"archive/zip"
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

// DataDir é a pasta de dados do programa (wallets extraídos, known_hosts).
var DataDir string

func dataDir() string {
	if DataDir != "" {
		return DataDir
	}
	d, _ := os.UserConfigDir()
	return filepath.Join(d, "ora-brabo-studio")
}

// CloudServices lista os serviços do tnsnames.ora de dentro do .zip.
func CloudServices(zipPath string) ([]string, error) {
	z, err := zip.OpenReader(expandHome(zipPath))
	if err != nil {
		return nil, err
	}
	defer z.Close()
	for _, f := range z.File {
		if strings.EqualFold(filepath.Base(f.Name), "tnsnames.ora") {
			r, err := f.Open()
			if err != nil {
				return nil, err
			}
			b, err := io.ReadAll(io.LimitReader(r, 1<<20))
			r.Close()
			if err != nil {
				return nil, err
			}
			return AliasNames(ParseTNS(string(b))), nil
		}
	}
	return nil, fmt.Errorf(L("%s não tem tnsnames.ora (é o wallet do Autonomous?)", "%s has no tnsnames.ora (is it the Autonomous wallet?)"), zipPath)
}

// ExtractCloudWallet extrai o .zip numa pasta só dele (de novo se o .zip
// mudou) e devolve a pasta, que serve de TNS_ADMIN e de WALLET.
func ExtractCloudWallet(zipPath, _ string) (string, error) {
	zp, err := filepath.Abs(expandHome(zipPath))
	if err != nil {
		return "", err
	}
	st, err := os.Stat(zp)
	if err != nil {
		return "", err
	}
	h := sha1.Sum([]byte(zp))
	base := strings.TrimSuffix(filepath.Base(zp), filepath.Ext(zp))
	dir := filepath.Join(dataDir(), "wallets", safeName(base)+"-"+hex.EncodeToString(h[:4]))
	mark := filepath.Join(dir, ".zip-mtime")
	if b, err := os.ReadFile(mark); err == nil && string(b) == st.ModTime().UTC().Format(time.RFC3339Nano) {
		return dir, nil
	}
	z, err := zip.OpenReader(zp)
	if err != nil {
		return "", err
	}
	defer z.Close()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(f.Name) // o wallet é plano; nada de caminhos do .zip
		if name == "." || name == ".." || strings.HasPrefix(name, ".") {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return "", err
		}
		out, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			r.Close()
			return "", err
		}
		_, err = io.Copy(out, io.LimitReader(r, 50<<20))
		r.Close()
		out.Close()
		if err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(mark, []byte(st.ModTime().UTC().Format(time.RFC3339Nano)), 0o600); err != nil {
		return "", err
	}
	return dir, nil
}

func safeName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, s)
	if s == "" {
		s = "wallet"
	}
	return s
}

// ldapDial aceita host, host:porta, ldap://host:porta[/contexto] e ldaps://.
// Devolve também o contexto que vier na URL.
func ldapDial(server string) (*ldap.Conn, string, error) {
	server = strings.TrimSpace(server)
	ctx := ""
	if !strings.Contains(server, "://") {
		host, port, err := net.SplitHostPort(server)
		if err != nil {
			host, port = server, "389"
		}
		sch := "ldap"
		if port == "636" {
			sch = "ldaps"
		}
		server = sch + "://" + net.JoinHostPort(host, port)
	} else if u, err := url.Parse(server); err == nil {
		ctx = strings.TrimPrefix(u.Path, "/")
		u.Path = ""
		server = u.String()
	}
	c, err := ldap.DialURL(server, ldap.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}),
		ldap.DialWithTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12}))
	if err != nil {
		return nil, "", err
	}
	c.SetTimeout(10 * time.Second)
	return c, ctx, nil
}

func oracleContext(ctx, fromURL string) string {
	if ctx == "" {
		ctx = fromURL
	}
	if !strings.HasPrefix(strings.ToLower(ctx), "cn=oraclecontext") {
		ctx = "cn=OracleContext," + ctx
	}
	return ctx
}

// LDAPLookup busca o descritor (orclNetDescString) do serviço no diretório.
func LDAPLookup(server, context, service string) (string, error) {
	c, uctx, err := ldapDial(server)
	if err != nil {
		return "", err
	}
	defer c.Close()
	base := oracleContext(context, uctx)
	f := fmt.Sprintf("(&(|(objectclass=orclNetService)(objectclass=orclService)(objectclass=orclNetServiceAlias))(cn=%s))", ldap.EscapeFilter(service))
	r, err := c.Search(ldap.NewSearchRequest(base, ldap.ScopeWholeSubtree, ldap.DerefAlways, 2, 10, false, f,
		[]string{"orclNetDescString", "aliasedObjectName"}, nil))
	if err != nil {
		return "", fmt.Errorf("ldap %s: %w", base, err)
	}
	for _, e := range r.Entries {
		if d := e.GetAttributeValue("orclNetDescString"); d != "" {
			return d, nil
		}
		if dn := e.GetAttributeValue("aliasedObjectName"); dn != "" {
			r2, err := c.Search(ldap.NewSearchRequest(dn, ldap.ScopeBaseObject, ldap.DerefAlways, 1, 10, false,
				"(objectclass=*)", []string{"orclNetDescString"}, nil))
			if err == nil && len(r2.Entries) > 0 {
				if d := r2.Entries[0].GetAttributeValue("orclNetDescString"); d != "" {
					return d, nil
				}
			}
		}
	}
	return "", fmt.Errorf(L("serviço %q não encontrado no LDAP (%s)", "service %q not found in LDAP (%s)"), service, base)
}

// LDAPServices lista os serviços cadastrados no contexto.
func LDAPServices(server, context string) ([]string, error) {
	c, uctx, err := ldapDial(server)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	base := oracleContext(context, uctx)
	r, err := c.Search(ldap.NewSearchRequest(base, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 2000, 10, false,
		"(|(objectclass=orclNetService)(objectclass=orclService)(objectclass=orclNetServiceAlias))", []string{"cn"}, nil))
	if err != nil {
		return nil, fmt.Errorf("ldap %s: %w", base, err)
	}
	var out []string
	for _, e := range r.Entries {
		if n := e.GetAttributeValue("cn"); n != "" {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out, nil
}
