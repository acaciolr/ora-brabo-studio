// Idioma da interface: PT-BR (padrão) ou EN, escolhido por --lang ou pelo LANG.
// by • DBA BRABO
package i18n

import (
	"os"
	"strings"
)

// EN liga o inglês.
var EN = detect()

func detect() bool {
	for _, v := range []string{"ORA_BRABO_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if s := strings.ToLower(os.Getenv(v)); s != "" {
			return strings.HasPrefix(s, "en")
		}
	}
	return false
}

// L escolhe o texto do idioma atual.
func L(pt, en string) string {
	if EN {
		return en
	}
	return pt
}
