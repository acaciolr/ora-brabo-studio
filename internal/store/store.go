// Conexões salvas, histórico de comandos e preferências em ~/.config/ora-brabo-studio.
// A senha só vai para o disco se o usuário pedir, cifrada com AES-GCM por uma
// chave local (0600). Protege contra quem lê o JSON, não contra quem é dono da conta.
// by • DBA BRABO
package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/acaciolr/ora-brabo-studio/internal/ora"
)

// Dir é a pasta de configuração (ORA_BRABO_STUDIO_HOME muda, útil nos testes).
func Dir() string {
	if d := os.Getenv("ORA_BRABO_STUDIO_HOME"); d != "" {
		return d
	}
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "ora-brabo-studio")
	}
	return ".ora-brabo-studio"
}

func ensureDir() error { return os.MkdirAll(Dir(), 0o700) }

func key() ([]byte, error) {
	if err := ensureDir(); err != nil {
		return nil, err
	}
	p := filepath.Join(Dir(), "key")
	if b, err := os.ReadFile(p); err == nil && len(b) == 32 {
		return b, nil
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, err
	}
	return k, os.WriteFile(p, k, 0o600)
}

// Seal cifra um texto.
func Seal(plain string) (string, error) {
	k, err := key()
	if err != nil {
		return "", err
	}
	blk, _ := aes.NewCipher(k)
	g, _ := cipher.NewGCM(blk)
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(g.Seal(nonce, nonce, []byte(plain), []byte("ora-brabo-studio"))), nil
}

// Open decifra um texto do Seal.
func Open(sealed string) (string, error) {
	k, err := key()
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return "", err
	}
	blk, _ := aes.NewCipher(k)
	g, _ := cipher.NewGCM(blk)
	if len(raw) < g.NonceSize() {
		return "", errors.New("senha salva ilegível")
	}
	p, err := g.Open(nil, raw[:g.NonceSize()], raw[g.NonceSize():], []byte("ora-brabo-studio"))
	if err != nil {
		return "", errors.New("senha salva ilegível (a chave mudou?)")
	}
	return string(p), nil
}

type diskConn struct {
	ora.ConnInfo
	Sealed string `json:"sealedPassword,omitempty"`
}

var mu sync.Mutex

// LoadConns lê as conexões salvas.
func LoadConns() ([]ora.ConnInfo, error) {
	mu.Lock()
	defer mu.Unlock()
	b, err := os.ReadFile(filepath.Join(Dir(), "connections.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var d []diskConn
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	out := make([]ora.ConnInfo, 0, len(d))
	for _, c := range d {
		ci := c.ConnInfo
		ci.Password = ""
		if c.Sealed != "" {
			if p, err := Open(c.Sealed); err == nil {
				ci.Password = p
			}
		}
		out = append(out, ci)
	}
	return out, nil
}

// SaveConns grava as conexões (ordenadas por nome).
func SaveConns(cs []ora.ConnInfo) error {
	mu.Lock()
	defer mu.Unlock()
	if err := ensureDir(); err != nil {
		return err
	}
	sort.SliceStable(cs, func(i, j int) bool { return strings.ToLower(cs[i].Name) < strings.ToLower(cs[j].Name) })
	d := make([]diskConn, 0, len(cs))
	for _, c := range cs {
		dc := diskConn{ConnInfo: c}
		dc.Password = ""
		if c.SavePwd && c.Password != "" {
			s, err := Seal(c.Password)
			if err != nil {
				return err
			}
			dc.Sealed = s
		}
		d = append(d, dc)
	}
	b, _ := json.MarshalIndent(d, "", "  ")
	tmp := filepath.Join(Dir(), "connections.json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(Dir(), "connections.json"))
}

// ---- histórico ----

// Hist é um comando executado.
type Hist struct {
	When time.Time `json:"when"`
	Conn string    `json:"conn"`
	SQL  string    `json:"sql"`
	OK   bool      `json:"ok"`
	Ms   int64     `json:"ms"`
}

const maxHist = 2000

// AddHist acrescenta ao histórico (um JSON por linha; corta nos últimos maxHist).
func AddHist(h Hist) {
	mu.Lock()
	defer mu.Unlock()
	if ensureDir() != nil {
		return
	}
	p := filepath.Join(Dir(), "history.jsonl")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	b, _ := json.Marshal(h)
	_, _ = f.Write(append(b, '\n'))
	f.Close()
	if st, err := os.Stat(p); err == nil && st.Size() > 4<<20 {
		all := readHist(p)
		if len(all) > maxHist {
			all = all[len(all)-maxHist:]
		}
		var sb strings.Builder
		for _, x := range all {
			b, _ := json.Marshal(x)
			sb.Write(b)
			sb.WriteByte('\n')
		}
		_ = os.WriteFile(p, []byte(sb.String()), 0o600)
	}
}

// LoadHist devolve o histórico, mais recente primeiro.
func LoadHist() []Hist {
	mu.Lock()
	defer mu.Unlock()
	h := readHist(filepath.Join(Dir(), "history.jsonl"))
	for i, j := 0, len(h)-1; i < j; i, j = i+1, j-1 {
		h[i], h[j] = h[j], h[i]
	}
	return h
}

func readHist(p string) []Hist {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var out []Hist
	for _, l := range strings.Split(string(b), "\n") {
		var h Hist
		if json.Unmarshal([]byte(l), &h) == nil && h.SQL != "" {
			out = append(out, h)
		}
	}
	return out
}

// ---- importação do SQL Developer (connections.json) ----

type sqldevFile struct {
	Connections []struct {
		Name string            `json:"name"`
		Type string            `json:"type"`
		Info map[string]string `json:"info"`
	} `json:"connections"`
}

// ImportSQLDev lê o connections.json do SQL Developer (sem as senhas, que ficam
// cifradas por ele). Devolve as conexões encontradas.
func ImportSQLDev(path string) ([]ora.ConnInfo, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f sqldevFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	var out []ora.ConnInfo
	for _, c := range f.Connections {
		in := c.Info
		ci := ora.ConnInfo{Name: c.Name, User: in["user"], Host: in["hostname"], Service: in["serviceName"], SID: in["sid"]}
		if p := in["port"]; p != "" {
			for _, r := range p {
				if r < '0' || r > '9' {
					p = ""
					break
				}
			}
			if p != "" {
				n := 0
				for _, r := range p {
					n = n*10 + int(r-'0')
				}
				ci.Port = n
			}
		}
		if r := strings.ToUpper(in["role"]); r != "" && r != "DEFAULT" {
			ci.Role = r
		}
		if in["customUrl"] != "" {
			u := in["customUrl"]
			if i := strings.Index(u, "@"); i >= 0 {
				u = u[i+1:]
			}
			ci.TNS = u
		}
		if t := in["OS_AUTHENTICATION"]; t == "true" {
			continue
		}
		if a := in["TNS_ALIAS"]; a != "" {
			ci.TNS = a
		}
		out = append(out, ci)
	}
	return out, nil
}

// SQLDevConnFiles acha connections.json de instalações do SQL Developer.
func SQLDevConnFiles() []string {
	home, _ := os.UserHomeDir()
	var out []string
	for _, pat := range []string{
		filepath.Join(home, ".sqldeveloper", "system*", "o.jdeveloper.db.connection*", "connections.json"),
		filepath.Join(os.Getenv("APPDATA"), "SQL Developer", "system*", "o.jdeveloper.db.connection*", "connections.json"),
	} {
		m, _ := filepath.Glob(pat)
		out = append(out, m...)
	}
	return out
}
