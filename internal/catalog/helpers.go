// Construtores curtos para as telas do catálogo. by • DBA BRABO
package catalog

import (
	"fmt"
	"strings"

	"github.com/acaciolr/ora-brabo-studio/internal/i18n"
)

// L escolhe o texto do idioma atual.
func L(pt, en string) string { return i18n.L(pt, en) }

func fT(name, label, def string) Field { return Field{Name: name, Label: label, Default: def} }
func fR(name, label, def string) Field {
	return Field{Name: name, Label: label, Default: def, Required: true}
}
func fP(name, label string) Field {
	return Field{Name: name, Label: label, Kind: Password, Required: true}
}
func fB(name, label string, def bool) Field {
	d := ""
	if def {
		d = "1"
	}
	return Field{Name: name, Label: label, Kind: Bool, Default: d}
}
func fC(name, label, def string, choices ...string) Field {
	return Field{Name: name, Label: label, Kind: Choice, Default: def, Choices: choices}
}
func fN(name, label, def string) Field {
	return Field{Name: name, Label: label, Kind: Number, Default: def}
}
func fM(name, label, def string) Field {
	return Field{Name: name, Label: label, Kind: Multi, Default: def}
}

// act cria uma ação que gera comando.
func act(k rune, name string, needRow bool, fields []Field, b func(Row, Vals) (Script, error)) *Action {
	return &Action{Key: k, Name: name, NeedRow: needRow, Fields: fields, Build: b}
}

// danger marca a ação como destrutiva: confirma digitando o valor da coluna.
func danger(a *Action, col string) *Action {
	a.Danger = true
	a.Confirm = col
	return a
}

// rep cria uma ação de relatório em texto (só leitura, roda direto).
func rep(k rune, name string, needRow bool, fields []Field, b func(Row, Vals) (Script, error)) *Action {
	a := act(k, name, needRow, fields, b)
	a.Auto = true
	return a
}

// drill abre outra tela filtrada pela linha.
func drill(k rune, name string, v *View, args func(Row) []any) *Action {
	return &Action{Key: k, Name: name, NeedRow: true, Drill: v, DrillArgs: args,
		DrillName: func(r Row) string {
			parts := []string{}
			for _, a := range args(r) {
				if a != nil {
					parts = append(parts, fmt.Sprint(a))
				}
			}
			return v.Title + " · " + strings.Join(parts, ".")
		}}
}

func pack(a *Action, p string) *Action { a.Pack = p; return a }

// reportQ monta um Script de relatório.
func reportQ(format string, a ...any) (Script, error) {
	return Script{Kind: Report, Text: fmt.Sprintf(format, a...)}, nil
}

// one devolve os argumentos de drill de uma coluna.
func one(col string) func(Row) []any { return func(r Row) []any { return []any{r[col]} } }
func two(c1, c2 string) func(Row) []any {
	return func(r Row) []any { return []any{r[c1], r[c2]} }
}

// nil1 é o argumento "sem filtro" das telas que aceitam filtro opcional.
var nil1 = []any{nil}
var nil2 = []any{nil, nil}

// v monta uma tela.
func v(id, title, sql string, actions ...*Action) *View {
	return &View{ID: id, Title: title, SQL: sql, Actions: actions}
}

func top(n int, sql string) string {
	return fmt.Sprintf("select * from (%s) where rownum <= %d", sql, n)
}

// strList transforma "a, b ,c" em 'A','B','C' (maiúsculo, para nomes).
func strList(s string, upper bool) string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if upper && !strings.HasPrefix(p, `"`) {
			p = strings.ToUpper(p)
		}
		out = append(out, Lit(strings.Trim(p, `"`)))
	}
	return strings.Join(out, ",")
}

// identList transforma "a, b" em A, B (identificadores).
func identList(s string) string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, IdentIn(p))
		}
	}
	return strings.Join(out, ", ")
}

func iff(c bool, a, b string) string {
	if c {
		return a
	}
	return b
}

// plsql envolve comandos num bloco anônimo.
func plsql(body string) string { return "BEGIN\n  " + strings.TrimSpace(body) + "\nEND;" }

// pending envolve as chamadas do Resource Manager na pending area.
func pending(body string) string {
	return "BEGIN\n  DBMS_RESOURCE_MANAGER.CLEAR_PENDING_AREA;\n  DBMS_RESOURCE_MANAGER.CREATE_PENDING_AREA;\n  " +
		strings.TrimSpace(body) + "\n  DBMS_RESOURCE_MANAGER.VALIDATE_PENDING_AREA;\n  DBMS_RESOURCE_MANAGER.SUBMIT_PENDING_AREA;\nEND;"
}

// named devolve "nome => valor" só para os campos preenchidos.
func named(pairs ...string) string {
	var out []string
	for i := 0; i+1 < len(pairs); i += 2 {
		if strings.TrimSpace(pairs[i+1]) != "" {
			out = append(out, pairs[i]+" => "+pairs[i+1])
		}
	}
	return strings.Join(out, ", ")
}

func litOrEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return Lit(strings.TrimSpace(s))
}

func numOrEmpty(name, s string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", nil
	}
	return Num(name, s)
}
