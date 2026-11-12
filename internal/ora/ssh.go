// Túnel SSH (port forward) para conexões atrás de um bastion, como o
// "SSH host" do SQL Developer. O go-ora disca pelo túnel. by • DBA BRABO
package ora

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

type sshTunnel struct {
	c  ConnInfo
	mu sync.Mutex
	cl *ssh.Client
}

func newSSHTunnel(c ConnInfo) *sshTunnel { return &sshTunnel{c: c} }

func (t *sshTunnel) client() (*ssh.Client, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cl != nil {
		return t.cl, nil
	}
	port := t.c.SSHPort
	if port == 0 {
		port = 22
	}
	user := t.c.SSHUser
	if user == "" {
		user = os.Getenv("USER")
	}
	auth, err := sshAuth(t.c)
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{User: user, Auth: auth, HostKeyCallback: hostKeyCheck(), Timeout: 15 * time.Second}
	cl, err := ssh.Dial("tcp", net.JoinHostPort(t.c.SSHHost, strconv.Itoa(port)), cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh %s: %w", t.c.SSHHost, err)
	}
	t.cl = cl
	return cl, nil
}

// DialContext abre a conexão TCP com o banco a partir do host SSH.
func (t *sshTunnel) DialContext(ctx context.Context, _, addr string) (net.Conn, error) {
	for try := 0; try < 2; try++ {
		cl, err := t.client()
		if err != nil {
			return nil, err
		}
		c, err := cl.DialContext(ctx, "tcp", addr)
		if err == nil {
			return c, nil
		}
		// sessão SSH caiu: refaz uma vez
		t.mu.Lock()
		if t.cl == cl {
			t.cl.Close()
			t.cl = nil
		}
		t.mu.Unlock()
		if try == 1 {
			return nil, fmt.Errorf("ssh %s → %s: %w", t.c.SSHHost, addr, err)
		}
	}
	return nil, errors.New("ssh")
}

func (t *sshTunnel) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cl != nil {
		err := t.cl.Close()
		t.cl = nil
		return err
	}
	return nil
}

// sshAuth: chave (com frase, se tiver), agente SSH e senha, nessa ordem.
func sshAuth(c ConnInfo) ([]ssh.AuthMethod, error) {
	var out []ssh.AuthMethod
	keys := []string{c.SSHKey}
	if c.SSHKey == "" {
		if h, err := os.UserHomeDir(); err == nil {
			for _, n := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
				keys = append(keys, filepath.Join(h, ".ssh", n))
			}
		}
	}
	var signers []ssh.Signer
	for _, k := range keys {
		if k == "" {
			continue
		}
		b, err := os.ReadFile(expandHome(k))
		if err != nil {
			if k == c.SSHKey {
				return nil, err
			}
			continue
		}
		s, err := ssh.ParsePrivateKey(b)
		var miss *ssh.PassphraseMissingError
		if errors.As(err, &miss) && c.SSHPwd != "" {
			s, err = ssh.ParsePrivateKeyWithPassphrase(b, []byte(c.SSHPwd))
		}
		if err != nil {
			if k == c.SSHKey {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			continue
		}
		signers = append(signers, s)
	}
	if len(signers) > 0 {
		out = append(out, ssh.PublicKeys(signers...))
	}
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			out = append(out, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}
	if c.SSHPwd != "" && c.SSHKey == "" {
		pw := c.SSHPwd
		out = append(out, ssh.Password(pw), ssh.KeyboardInteractive(func(_, _ string, q []string, _ []bool) ([]string, error) {
			ans := make([]string, len(q))
			for i := range ans {
				ans[i] = pw
			}
			return ans, nil
		}))
	}
	if len(out) == 0 {
		return nil, errors.New(L("SSH sem chave, agente ou senha", "SSH without key, agent or password"))
	}
	return out, nil
}

// KnownHostsFile é onde ficam as chaves dos hosts SSH aceitos pelo programa
// (o ~/.ssh/known_hosts também é lido).
func KnownHostsFile() string { return filepath.Join(dataDir(), "known_hosts") }

// hostKeyCheck confere a chave do host: conhecida passa, diferente bloqueia,
// nova é gravada (primeiro uso, como o ssh com accept-new).
func hostKeyCheck() ssh.HostKeyCallback {
	var files []string
	if h, err := os.UserHomeDir(); err == nil {
		files = append(files, filepath.Join(h, ".ssh", "known_hosts"))
	}
	files = append(files, KnownHostsFile())
	var ok []string
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			ok = append(ok, f)
		}
	}
	var cb ssh.HostKeyCallback
	if len(ok) > 0 {
		cb, _ = knownhosts.New(ok...)
	}
	return func(host string, remote net.Addr, key ssh.PublicKey) error {
		if cb != nil {
			err := cb(host, remote, key)
			if err == nil {
				return nil
			}
			var ke *knownhosts.KeyError
			if errors.As(err, &ke) && len(ke.Want) > 0 {
				return fmt.Errorf(L("a chave do host SSH %s mudou (confira antes de seguir): %w", "the SSH host key of %s changed (check before going on): %w"), host, err)
			}
		}
		if err := os.MkdirAll(filepath.Dir(KnownHostsFile()), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(KnownHostsFile(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = fmt.Fprintln(f, knownhosts.Line([]string{knownhosts.Normalize(host)}, key))
		return err
	}
}
