// Gera docs/CATALOGO.md / docs/CATALOG.md (todas as telas e ações da aba DBA):
// CATALOG_DOC=docs go test -run TestCatalogDoc ./internal/catalog/  by • DBA BRABO
package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acaciolr/ora-brabo-studio/internal/i18n"
)

func TestCatalogDoc(t *testing.T) {
	dir := os.Getenv("CATALOG_DOC")
	if dir == "" {
		t.Skip("CATALOG_DOC não definido")
	}
	defer func() { i18n.EN = false }()
	for _, en := range []bool{false, true} {
		i18n.EN = en
		var b strings.Builder
		views, acts := 0, 0
		var body strings.Builder
		for _, g := range DBA().Children {
			fmt.Fprintf(&body, "\n## %s\n\n", g.Title)
			body.WriteString(L("| Tela | Ações (tecla) |\n|---|---|\n", "| Screen | Actions (key) |\n|---|---|\n"))
			var row func(prefix string, n *Node)
			row = func(prefix string, n *Node) {
				if n.View != nil {
					views++
					var as []string
					for _, a := range n.View.Actions {
						acts++
						s := fmt.Sprintf("`%c` %s", a.Key, a.Name)
						if a.Danger {
							s += " ⚠"
						}
						if a.Pack != "" || n.View.Pack != "" {
							s += " 💲"
						}
						as = append(as, strings.ReplaceAll(s, "|", "/"))
					}
					title := prefix + n.View.Title
					if n.View.Pack != "" {
						title += " 💲"
					}
					fmt.Fprintf(&body, "| %s | %s |\n", strings.ReplaceAll(title, "|", "/"), strings.Join(as, " · "))
				}
				for _, c := range n.Children {
					p := prefix
					if n.View == nil && n != g {
						p = prefix + n.Title + " › "
					}
					row(p, c)
				}
			}
			row("", g)
		}
		b.WriteString(L("# Aba DBA: todas as telas e ações\n\n", "# DBA tab: every screen and action\n\n"))
		fmt.Fprintf(&b, L("%d telas na árvore e %d ações nelas (fora as telas de detalhe abertas por elas). ⚠ = destrutiva (confirma digitando o nome). 💲 = exige Diagnostics/Tuning Pack.\n\nGerado do catálogo por `internal/catalog/doc_test.go`. by • DBA BRABO\n",
			"%d screens in the tree and %d actions on them (plus the detail screens they open). ⚠ = destructive (type the name to confirm). 💲 = requires the Diagnostics/Tuning Pack.\n\nGenerated from the catalog by `internal/catalog/doc_test.go`. by • DBA BRABO\n"), views, acts)
		b.WriteString(body.String())
		name := map[bool]string{false: "CATALOGO.md", true: "CATALOG.md"}[en]
		if err := os.WriteFile(filepath.Join(dir, name), []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
