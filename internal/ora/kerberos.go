// Autenticação Kerberos: usa o ticket do cache (kinit), como o SQL Developer
// com "Kerberos Authentication". by • DBA BRABO
package ora

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/jcmturner/gokrb5/v8/client"
	"github.com/jcmturner/gokrb5/v8/config"
	"github.com/jcmturner/gokrb5/v8/credentials"
	"github.com/jcmturner/gokrb5/v8/gssapi"
	"github.com/jcmturner/gokrb5/v8/spnego"
)

type kerberosAuth struct{}

// KrbConfig e KrbCache: KRB5_CONFIG / KRB5CCNAME ou os padrões do sistema.
func KrbConfig() string {
	if p := os.Getenv("KRB5_CONFIG"); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return `C:\ProgramData\MIT\Kerberos5\krb5.ini`
	}
	return "/etc/krb5.conf"
}

func KrbCache() string {
	if p := os.Getenv("KRB5CCNAME"); p != "" {
		return strings.TrimPrefix(p, "FILE:")
	}
	return fmt.Sprintf("/tmp/krb5cc_%d", os.Getuid())
}

func (kerberosAuth) Authenticate(server, service string) ([]byte, error) {
	cfg, err := config.Load(KrbConfig())
	if err != nil {
		return nil, fmt.Errorf("kerberos %s: %w", KrbConfig(), err)
	}
	cc, err := credentials.LoadCCache(KrbCache())
	if err != nil {
		return nil, fmt.Errorf(L("kerberos: sem ticket em %s (rode kinit): %w", "kerberos: no ticket in %s (run kinit): %w"), KrbCache(), err)
	}
	cl, err := client.NewFromCCache(cc, cfg, client.DisablePAFXFAST(true))
	if err != nil {
		return nil, err
	}
	tkt, key, err := cl.GetServiceTicket(service + "/" + server)
	if err != nil {
		return nil, err
	}
	tok, err := spnego.NewKRB5TokenAPREQ(cl, tkt, key, []int{gssapi.ContextFlagInteg, gssapi.ContextFlagConf}, []int{})
	if err != nil {
		return nil, err
	}
	return tok.APReq.Marshal()
}
