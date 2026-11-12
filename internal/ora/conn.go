// Dados de conexão e montagem da URL do go-ora: básica, TNS, Cloud Wallet, LDAP,
// URL personalizada; senha, SO, Kerberos, proxy, wallet, SSL e túnel SSH.
// by • DBA BRABO
package ora

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	go_ora "github.com/sijms/go-ora/v2"
)

// Tipos de conexão (os mesmos do SQL Developer).
const (
	TypeBasic  = "basic"  // host, porta, service name ou SID
	TypeTNS    = "tns"    // alias do tnsnames.ora ou descritor
	TypeCloud  = "cloud"  // Cloud Wallet (.zip do Autonomous Database)
	TypeLDAP   = "ldap"   // descritor buscado no diretório (OID/AD)
	TypeCustom = "custom" // URL: jdbc:oracle:thin:@..., oracle://..., host:porta/serviço
)

// Autenticação.
const (
	AuthPassword = ""         // usuário e senha
	AuthOS       = "os"       // usuário externo (OPS$ / IDENTIFIED EXTERNALLY)
	AuthKerberos = "kerberos" // ticket do cache do Kerberos (kinit)
)

// ConnInfo é uma conexão salva.
type ConnInfo struct {
	Name     string `json:"name"`
	User     string `json:"user"`
	Password string `json:"password,omitempty"` // em memória; no disco vai cifrada
	SavePwd  bool   `json:"savePassword"`
	Type     string `json:"type,omitempty"` // basic (vazio), tns, cloud, ldap, custom
	Auth     string `json:"auth,omitempty"` // vazio (senha), os, kerberos
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Service  string `json:"service,omitempty"`
	SID      string `json:"sid,omitempty"`
	// TNS: alias ou descritor. Cloud e LDAP: o serviço (alias do tnsnames do
	// wallet / nome no diretório).
	TNS         string `json:"tns,omitempty"`
	TNSAdmin    string `json:"tnsAdmin,omitempty"` // pasta do tnsnames.ora (senão TNS_ADMIN, ORACLE_HOME)
	CloudZip    string `json:"cloudZip,omitempty"`
	LDAPServer  string `json:"ldapServer,omitempty"`  // host:porta (636 = ldaps) ou ldap://host:porta
	LDAPContext string `json:"ldapContext,omitempty"` // ex.: dc=empresa,dc=com
	CustomURL   string `json:"customUrl,omitempty"`
	Role        string `json:"role,omitempty"`  // SYSDBA, SYSOPER, SYSBACKUP, SYSDG, SYSKM, SYSRAC, SYSASM
	Proxy       string `json:"proxy,omitempty"` // usuário cliente do proxy: entra como User[Proxy]
	Wallet      string `json:"wallet,omitempty"`
	WalletPwd   string `json:"walletPassword,omitempty"` // em memória; no disco vai cifrada
	SSL         bool   `json:"ssl,omitempty"`
	SSLVerify   bool   `json:"sslVerify,omitempty"`
	// túnel SSH (port forward, como no SQL Developer)
	SSHHost string `json:"sshHost,omitempty"`
	SSHPort int    `json:"sshPort,omitempty"`
	SSHUser string `json:"sshUser,omitempty"`
	SSHKey  string `json:"sshKey,omitempty"`      // arquivo da chave privada (vazio: agente SSH / senha)
	SSHPwd  string `json:"sshPassword,omitempty"` // senha ou frase da chave; em memória
	// Comando que recebe o script RMAN pela entrada padrão. Ex.:
	//   ssh oracle@db01 '. ~/.bash_profile; rman target /'
	//   docker exec -i oracle-free rman target /
	RmanCmd string `json:"rmanCmd,omitempty"`
	Color   string `json:"color,omitempty"` // cor da aba: prod em vermelho, como manda o bom senso
}

// Kind devolve o tipo, deduzindo nas conexões antigas (sem Type).
func (c ConnInfo) Kind() string {
	if c.Type != "" {
		return c.Type
	}
	if c.TNS != "" {
		return TypeTNS
	}
	return TypeBasic
}

// Display é o rótulo curto: usuario@destino.
func (c ConnInfo) Display() string {
	var dest string
	switch c.Kind() {
	case TypeTNS:
		dest = c.TNS
	case TypeCloud:
		dest = c.TNS + " (cloud)"
	case TypeLDAP:
		dest = "ldap:" + c.TNS
	case TypeCustom:
		dest = c.CustomURL
	default:
		svc := c.Service
		if svc == "" {
			svc = c.SID
		}
		dest = fmt.Sprintf("%s:%d/%s", c.Host, c.Port, svc)
	}
	if len(dest) > 60 {
		dest = dest[:57] + "..."
	}
	u := c.User
	switch c.Auth {
	case AuthOS:
		u = "(os)"
	case AuthKerberos:
		u = "(kerberos)"
	}
	if c.Proxy != "" {
		u += "[" + c.Proxy + "]"
	}
	r := ""
	if c.Role != "" {
		r = " as " + strings.ToLower(c.Role)
	}
	if c.SSHHost != "" {
		r += " via ssh " + c.SSHHost
	}
	return u + "@" + dest + r
}

// NeedsPassword diz se a senha tem de ser pedida antes de conectar (não pede
// em SO, Kerberos, wallet com credencial ou URL com usuário/senha).
func (c ConnInfo) NeedsPassword() bool {
	if c.Auth != AuthPassword || c.Password != "" || c.User == "" {
		return false
	}
	if c.Kind() == TypeCustom {
		u := c.CustomURL
		if i := strings.Index(u, "@"); i >= 0 && strings.Contains(u[:i], "/") && !strings.HasSuffix(u[:i], ":") {
			return false
		}
	}
	return true
}

// Check confere os campos obrigatórios do tipo.
func (c ConnInfo) Check() error {
	if c.Auth == AuthPassword && c.User == "" && c.Wallet == "" && c.Kind() != TypeCloud && c.Kind() != TypeCustom {
		return fmt.Errorf("%s", L("informe o usuário", "fill the user"))
	}
	need := func(v, pt, en string) error {
		if strings.TrimSpace(v) == "" {
			return fmt.Errorf("%s", L("informe "+pt, "fill "+en))
		}
		return nil
	}
	switch c.Kind() {
	case TypeTNS:
		return need(c.TNS, "o alias TNS ou o descritor", "the TNS alias or descriptor")
	case TypeCloud:
		if err := need(c.CloudZip, "o arquivo do wallet (.zip)", "the wallet file (.zip)"); err != nil {
			return err
		}
		return need(c.TNS, "o serviço", "the service")
	case TypeLDAP:
		if err := need(c.LDAPServer, "o servidor LDAP", "the LDAP server"); err != nil {
			return err
		}
		if err := need(c.LDAPContext, "o contexto LDAP", "the LDAP context"); err != nil {
			return err
		}
		return need(c.TNS, "o serviço", "the service")
	case TypeCustom:
		return need(c.CustomURL, "a URL", "the URL")
	}
	if err := need(c.Host, "o host", "the host"); err != nil {
		return err
	}
	if c.Service == "" && c.SID == "" {
		return fmt.Errorf("%s", L("informe o service name ou o SID", "fill the service name or SID"))
	}
	return nil
}

// options monta as opções comuns do go-ora.
func (c ConnInfo) options() map[string]string {
	opt := map[string]string{"TIMEOUT": "0", "CONNECTION TIMEOUT": "15"}
	if c.Role != "" {
		opt["DBA PRIVILEGE"] = strings.ToUpper(c.Role)
	}
	switch c.Auth {
	case AuthOS:
		opt["AUTH TYPE"] = "OS"
	case AuthKerberos:
		opt["AUTH TYPE"] = "KERBEROS"
	}
	if c.Proxy != "" {
		opt["PROXY CLIENT NAME"] = c.Proxy
	}
	if c.Wallet != "" {
		opt["WALLET"] = c.Wallet
	}
	if c.WalletPwd != "" {
		opt["WALLET PASSWORD"] = c.WalletPwd
	}
	if c.SSL {
		opt["SSL"] = "true"
		opt["SSL VERIFY"] = fmt.Sprint(c.SSLVerify)
	}
	return opt
}

// URL monta a URL do go-ora conforme o tipo.
func (c ConnInfo) URL() (string, error) {
	if err := c.Check(); err != nil {
		return "", err
	}
	opt := c.options()
	jdbc := func(desc string) string { return go_ora.BuildJDBC(c.User, c.Password, desc, opt) }
	switch c.Kind() {
	case TypeTNS:
		t := strings.TrimSpace(c.TNS)
		if strings.HasPrefix(t, "(") {
			return jdbc(t), nil
		}
		d, err := ResolveTNSIn(c.TNSAdmin, t)
		if err != nil {
			return "", err
		}
		return jdbc(d), nil
	case TypeCloud:
		dir, err := ExtractCloudWallet(c.CloudZip, c.Name)
		if err != nil {
			return "", err
		}
		d, err := ResolveTNSIn(dir, c.TNS)
		if err != nil {
			return "", err
		}
		opt["WALLET"] = dir
		opt["SSL"] = "true"
		opt["SSL VERIFY"] = fmt.Sprint(c.SSLVerify)
		return jdbc(d), nil
	case TypeLDAP:
		d, err := LDAPLookup(c.LDAPServer, c.LDAPContext, c.TNS)
		if err != nil {
			return "", err
		}
		return jdbc(d), nil
	case TypeCustom:
		return c.customURL(opt)
	}
	port := c.Port
	if port == 0 {
		port = 1521
	}
	svc := c.Service
	if svc == "" {
		opt["SID"] = c.SID
	}
	return go_ora.BuildUrl(c.Host, port, svc, c.User, c.Password, opt), nil
}

// customURL aceita jdbc:oracle:thin:@..., oracle://... ou host:porta/serviço.
func (c ConnInfo) customURL(opt map[string]string) (string, error) {
	u := strings.TrimSpace(c.CustomURL)
	if strings.HasPrefix(strings.ToLower(u), "oracle://") {
		return u, nil
	}
	low := strings.ToLower(u)
	for _, p := range []string{"jdbc:oracle:thin:", "jdbc:oracle:oci:"} {
		if strings.HasPrefix(low, p) {
			u, low = u[len(p):], low[len(p):]
		}
	}
	// usuário/senha embutidos: jdbc:oracle:thin:scott/tiger@...
	if i := strings.Index(u, "@"); i >= 0 {
		if i > 0 && c.User == "" {
			cred := u[:i]
			if j := strings.IndexByte(cred, '/'); j >= 0 {
				c.User, c.Password = cred[:j], cred[j+1:]
			} else {
				c.User = cred
			}
		}
		u = u[i+1:]
	}
	u = strings.TrimPrefix(u, "//")
	if q := strings.IndexByte(u, '?'); q >= 0 { // parâmetros JDBC (TNS_ADMIN=...)
		for _, kv := range strings.Split(u[q+1:], "&") {
			if k, v, ok := strings.Cut(kv, "="); ok && strings.EqualFold(k, "TNS_ADMIN") {
				c.TNSAdmin = v
			}
		}
		u = u[:q]
	}
	if u == "" {
		return "", fmt.Errorf("%s", L("URL sem destino", "URL without a destination"))
	}
	if !strings.HasPrefix(u, "(") {
		if !strings.ContainsAny(u, ":/") {
			d, err := ResolveTNSIn(c.TNSAdmin, u) // só o alias
			if err != nil {
				return "", err
			}
			u = d
		} else {
			// EZConnect: host[:porta][/serviço][:SID antigo host:porta:SID]
			host, rest, _ := strings.Cut(u, "/")
			port, svc := 1521, rest
			h, p, ok := strings.Cut(host, ":")
			if ok {
				if p2, sid, old := strings.Cut(p, ":"); old && rest == "" { // host:porta:SID
					p, opt["SID"], svc = p2, sid, ""
				}
				n, err := strconv.Atoi(p)
				if err != nil {
					return "", fmt.Errorf(L("porta inválida na URL: %q", "invalid port in URL: %q"), p)
				}
				port = n
			}
			if i := strings.IndexByte(svc, ':'); i >= 0 { // /serviço:server
				svc = svc[:i]
			}
			return go_ora.BuildUrl(h, port, svc, c.User, c.Password, opt), nil
		}
	}
	return go_ora.BuildJDBC(c.User, c.Password, u, opt), nil
}

var (
	tunMu   sync.Mutex
	tunnels = map[*sql.DB]io.Closer{}
)

// Open abre o pool da conexão (com túnel SSH e Kerberos, se pedidos) e testa
// com um ping.
func Open(c ConnInfo) (*sql.DB, error) {
	u, err := c.URL()
	if err != nil {
		return nil, err
	}
	conn := go_ora.NewConnector(u).(*go_ora.OracleConnector)
	var tun *sshTunnel
	if c.SSHHost != "" {
		tun = newSSHTunnel(c)
		conn.Dialer(tun)
	}
	if c.Auth == AuthKerberos {
		conn.WithKerberosAuth(kerberosAuth{})
	}
	db := sql.OpenDB(conn)
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(30 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		if tun != nil {
			tun.Close()
		}
		return nil, err
	}
	if tun != nil {
		tunMu.Lock()
		tunnels[db] = tun
		tunMu.Unlock()
	}
	return db, nil
}

// Close fecha o pool e o túnel SSH dele.
func Close(db *sql.DB) {
	if db == nil {
		return
	}
	db.Close()
	tunMu.Lock()
	t := tunnels[db]
	delete(tunnels, db)
	tunMu.Unlock()
	if t != nil {
		t.Close()
	}
}

// TNSFiles devolve os tnsnames.ora que serão lidos, na ordem.
func TNSFiles() []string { return TNSFilesIn("") }

// TNSFilesIn põe a pasta dada (TNS_ADMIN da conexão) na frente.
func TNSFilesIn(dir string) []string {
	var out []string
	if dir != "" {
		out = append(out, filepath.Join(expandHome(dir), "tnsnames.ora"))
	}
	if t := os.Getenv("TNS_ADMIN"); t != "" {
		out = append(out, filepath.Join(t, "tnsnames.ora"))
	}
	if h := os.Getenv("ORACLE_HOME"); h != "" {
		out = append(out, filepath.Join(h, "network", "admin", "tnsnames.ora"))
	}
	if home, _ := os.UserHomeDir(); home != "" {
		out = append(out, filepath.Join(home, ".tnsnames.ora"))
	}
	return out
}

// TNSAliases lê os aliases de todos os tnsnames.ora encontrados.
func TNSAliases() map[string]string { return TNSAliasesIn("") }

// TNSAliasesIn lê os aliases, começando pela pasta dada.
func TNSAliasesIn(dir string) map[string]string {
	all := map[string]string{}
	for _, f := range TNSFilesIn(dir) {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for k, v := range ParseTNS(string(b)) {
			if _, ok := all[k]; !ok {
				all[k] = v
			}
		}
	}
	return all
}

// ResolveTNS acha o descritor de um alias.
func ResolveTNS(alias string) (string, error) { return ResolveTNSIn("", alias) }

// ResolveTNSIn acha o descritor de um alias, começando pela pasta dada.
func ResolveTNSIn(dir, alias string) (string, error) {
	if d, ok := TNSAliasesIn(dir)[strings.ToUpper(strings.TrimSpace(alias))]; ok {
		return d, nil
	}
	return "", fmt.Errorf(L("alias TNS %q não encontrado em %s", "TNS alias %q not found in %s"), alias, strings.Join(TNSFilesIn(dir), ", "))
}

// AliasNames devolve os aliases em ordem.
func AliasNames(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

// ParseTNS lê um tnsnames.ora: ALIAS[,ALIAS2] = (DESCRIPTION=...)
func ParseTNS(txt string) map[string]string {
	out := map[string]string{}
	// tira comentários
	var b strings.Builder
	sc := bufio.NewScanner(strings.NewReader(txt))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		l := sc.Text()
		if i := strings.IndexByte(l, '#'); i >= 0 {
			l = l[:i]
		}
		b.WriteString(l)
		b.WriteByte('\n')
	}
	s := b.String()
	i := 0
	for i < len(s) {
		eq := strings.IndexByte(s[i:], '=')
		if eq < 0 {
			break
		}
		names := strings.TrimSpace(s[i : i+eq])
		j := i + eq + 1
		for j < len(s) && strings.ContainsRune(" \t\r\n", rune(s[j])) {
			j++
		}
		if j >= len(s) || s[j] != '(' {
			i = j
			continue
		}
		depth, k := 0, j
		for k < len(s) {
			if s[k] == '(' {
				depth++
			} else if s[k] == ')' {
				depth--
				if depth == 0 {
					k++
					break
				}
			}
			k++
		}
		desc := strings.Join(strings.Fields(s[j:k]), "")
		for _, n := range strings.Split(names, ",") {
			n = strings.ToUpper(strings.TrimSpace(n))
			if n != "" {
				out[n] = desc
			}
		}
		i = k
	}
	return out
}
