// Atalho do i18n dentro do pacote ora. by • DBA BRABO
package ora

import "github.com/acaciolr/ora-brabo-studio/internal/i18n"

// L escolhe o texto do idioma atual.
func L(pt, en string) string { return i18n.L(pt, en) }
