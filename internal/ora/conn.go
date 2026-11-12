// Dados de conexão e montagem da URL do go-ora (EZConnect, SID, TNS, wallet, SYSDBA).
// by • DBA BRABO
package ora

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	go_ora "github.com/sijms/go-ora/v2"
)

// ConnInfo é uma conexão salva.
type ConnInfo struct {
	Name     string `json:"name"`
	User     string `json:"user"`
	Password string `json:"password,omitempty"` // em memória; no disco vai cifrada
	SavePwd  bool   `json:"savePassword"`
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Service  string `json:"service,omitempty"`
	SID      string `json:"sid,omitempty"`
	TNS      string `json:"tns,omitempty"`  // alias do tnsnames.ora ou descritor completo
	Role     string `json:"role,omitempty"` // SYSDBA, SYSOPER, SYSBACKUP, SYSDG, SYSKM
	Wallet   string `json:"wallet,omitempty"`
	SSL      bool   `json:"ssl,omitempty"`
	// Comando que recebe o script RMAN pela entrada padrão. Ex.:
	//   ssh oracle@db01 '. ~/.bash_profile; rman target /'
	//   docker exec -i oracle-free rman target /
	RmanCmd string `json:"rmanCmd,omitempty"`
	Color   string `json:"color,omitempty"` // cor da aba: prod em vermelho, como manda o bom senso
}

// Display é o rótulo curto: usuario@destino.
func (c ConnInfo) Display() string {
	dest := c.TNS
	if dest == "" {
		svc := c.Service
		if svc == "" {
			svc = c.SID
		}
		dest = fmt.Sprintf("%s:%d/%s", c.Host, c.Port, svc)
	}
	if len(dest) > 60 {
		dest = dest[:57] + "..."
	}
	r := ""
	if c.Role != "" {
		r = " as " + strings.ToLower(c.Role)
	}
	return c.User + "@" + dest + r
}

// URL monta a URL do go-ora.
func (c ConnInfo) URL() (string, error) {
	opt := map[string]string{"TIMEOUT": "0", "CONNECTION TIMEOUT": "15"}
	if c.Role != "" {
		opt["DBA PRIVILEGE"] = strings.ToUpper(c.Role)
	}
	if c.Wallet != "" {
		opt["WALLET"] = c.Wallet
	}
	if c.SSL {
		opt["SSL"] = "true"
		opt["SSL VERIFY"] = "false"
	}
	if t := strings.TrimSpace(c.TNS); t != "" {
		desc := t
		if !strings.HasPrefix(t, "(") {
			d, err := ResolveTNS(t)
			if err != nil {
				return "", err
			}
			desc = d
		}
		return go_ora.BuildJDBC(c.User, c.Password, desc, opt), nil
	}
	if c.Host == "" {
		return "", fmt.Errorf("conexão sem host / connection without host")
	}
	port := c.Port
	if port == 0 {
		port = 1521
	}
	svc := c.Service
	if svc == "" && c.SID != "" {
		opt["SID"] = c.SID
	}
	return go_ora.BuildUrl(c.Host, port, svc, c.User, c.Password, opt), nil
}

// Open abre o pool da conexão e testa com um ping.
func Open(c ConnInfo) (*sql.DB, error) {
	u, err := c.URL()
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("oracle", u)
	if err != nil {
		return nil, err
	}
	db.SetMaxIdleConns(2)
	db.SetConnMaxIdleTime(30 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// TNSFiles devolve os tnsnames.ora que serão lidos, na ordem.
func TNSFiles() []string {
	var out []string
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
func TNSAliases() map[string]string {
	all := map[string]string{}
	for _, f := range TNSFiles() {
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
func ResolveTNS(alias string) (string, error) {
	if d, ok := TNSAliases()[strings.ToUpper(alias)]; ok {
		return d, nil
	}
	return "", fmt.Errorf("alias TNS %q não encontrado em %s", alias, strings.Join(TNSFiles(), ", "))
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
