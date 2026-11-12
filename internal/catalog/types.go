// Catálogo declarativo das telas (aba DBA e ações de objetos): cada tela é uma
// consulta de lista + ações que montam o comando. O comando sempre aparece antes
// de rodar, editável, e vai para o histórico.
// by • DBA BRABO
package catalog

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Row é a linha selecionada na lista (colunas em maiúsculo).
type Row map[string]string

// Vals são os valores preenchidos no formulário da ação.
type Vals map[string]string

// FieldKind é o tipo do campo do formulário.
type FieldKind int

const (
	Text FieldKind = iota
	Password
	Bool
	Choice
	Number
	Multi // texto com várias linhas
)

// Field é um campo do formulário de uma ação.
type Field struct {
	Name     string
	Label    string
	Kind     FieldKind
	Default  string // pode usar {COLUNA} da linha selecionada
	Choices  []string
	Required bool
	Hint     string
}

// ScriptKind diz como o comando gerado roda.
type ScriptKind int

const (
	SQL    ScriptKind = iota // um ou mais comandos SQL / PL/SQL (dividido como no SQL*Plus)
	RMAN                     // script RMAN, enviado ao comando RMAN da conexão
	Report                   // consulta que devolve texto (1ª coluna), aberta numa aba de texto
	Grid                     // consulta aberta numa aba de grade
)

// Script é o comando que uma ação gera.
type Script struct {
	Kind ScriptKind
	Text string
}

// Action é uma ação de uma tela.
type Action struct {
	Key     rune   // atalho na tela
	Name    string // rótulo
	NeedRow bool   // exige linha selecionada
	Fields  []Field
	Danger  bool   // pede confirmação digitando o nome
	Confirm string // coluna cujo valor deve ser digitado na confirmação (Danger)
	Build   func(r Row, v Vals) (Script, error)
	// Drill abre outra tela filtrada pela linha (ex.: privilégios do usuário)
	Drill     *View
	DrillArgs func(r Row) []any
	DrillName func(r Row) string
	Pack      string // "diag" ou "tuning": exige licença do pack
	Auto      bool   // roda sem pré-visualizar (só leitura: relatórios)
}

// View é uma tela de lista.
type View struct {
	ID      string
	Title   string
	SQL     string // consulta da lista; :1.. vêm de Args (drill)
	Args    []any
	Actions []*Action
	Pack    string // "diag"/"tuning": AWR, ASH, ADDM, SQL Tuning
	Needs   string // "cdb", "sysdba"
	Refresh int    // segundos de atualização automática sugerida (0 = manual)
	Special string // "instance" (painel), "topactivity", "text" (relatório)
	Help    string
	KeyCol  string // coluna que identifica a linha (para manter a seleção ao atualizar)
	// Requires: objeto de que a tela depende (ex.: PERFSTAT.STATS$SNAPSHOT,
	// DBA_TABLESPACE_SETS). Se não existir, a tela avisa em vez de dar erro.
	Requires string
	Missing  string // o que fazer quando falta (ex.: instalar o Statspack)
}

// RequiresSQL devolve a consulta que conta se o objeto exigido existe.
func (v *View) RequiresSQL() string {
	if v.Requires == "" {
		return ""
	}
	owner, name, ok := strings.Cut(strings.ToUpper(v.Requires), ".")
	if !ok {
		return "select count(*) from all_objects where object_name = " + Lit(owner) + " and owner in ('SYS', 'PUBLIC')"
	}
	return "select count(*) from all_objects where owner = " + Lit(owner) + " and object_name = " + Lit(name)
}

// Node é um nó da árvore.
type Node struct {
	Title    string
	View     *View
	Children []*Node
}

// Clone devolve uma cópia da tela com novos argumentos (usado no drill).
func (v *View) Clone(title string, args []any) *View {
	c := *v
	c.Title = title
	c.Args = args
	return &c
}

// FindAction acha a ação pelo atalho.
func (v *View) FindAction(k rune) *Action {
	for _, a := range v.Actions {
		if a.Key == k {
			return a
		}
	}
	return nil
}

// ---- auxiliares para montar comandos ----

var reSimpleIdent = regexp.MustCompile(`^[A-Z][A-Z0-9_$#]*$`)

// Ident devolve o identificador pronto para o SQL: sem aspas se for simples e
// maiúsculo; com aspas (escapando ") nos demais casos.
func Ident(s string) string {
	if reSimpleIdent.MatchString(s) {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// IdentIn aceita o que o usuário digitou: sem aspas vira maiúsculo.
func IdentIn(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) && len(s) > 1 {
		return Ident(s[1 : len(s)-1])
	}
	return Ident(strings.ToUpper(s))
}

// Q2 monta OWNER.NOME.
func Q2(owner, name string) string {
	if owner == "" {
		return Ident(name)
	}
	return Ident(owner) + "." + Ident(name)
}

// Lit devolve um literal de texto SQL.
func Lit(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// Num confere um número inteiro.
func Num(name, s string) (string, error) {
	s = strings.TrimSpace(s)
	if _, err := strconv.ParseInt(s, 10, 64); err != nil {
		return "", fmt.Errorf("%s: %q não é número inteiro", name, s)
	}
	return s, nil
}

var reSize = regexp.MustCompile(`(?i)^\d+(\.\d+)?\s*[KMGTP]?$`)

// Size confere um tamanho (100M, 2G, 512K, 1048576).
func Size(name, s string) (string, error) {
	s = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	if !reSize.MatchString(s) || strings.Contains(s, ".") {
		return "", fmt.Errorf("%s: %q não é tamanho válido (ex.: 100M, 2G)", name, s)
	}
	return s, nil
}

// Req confere campo obrigatório.
func Req(name, s string) (string, error) {
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("%s: obrigatório", name)
	}
	return strings.TrimSpace(s), nil
}

// Yes diz se um campo Bool está ligado.
func Yes(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "s", "sim", "y", "yes", "true", "on", "x":
		return true
	}
	return false
}

// Fill troca {COLUNA} pelo valor da linha.
func Fill(tpl string, r Row) string {
	if !strings.Contains(tpl, "{") {
		return tpl
	}
	return regexp.MustCompile(`\{([A-Z0-9_#$]+)\}`).ReplaceAllStringFunc(tpl, func(m string) string {
		return r[m[1:len(m)-1]]
	})
}

// sqlS é um atalho para Script SQL.
func sqlS(format string, a ...any) (Script, error) {
	return Script{Kind: SQL, Text: fmt.Sprintf(format, a...)}, nil
}

func rmanS(format string, a ...any) (Script, error) {
	return Script{Kind: RMAN, Text: fmt.Sprintf(format, a...)}, nil
}

// Walk percorre a árvore.
func Walk(n *Node, f func(path []string, n *Node)) {
	var rec func(p []string, n *Node)
	rec = func(p []string, n *Node) {
		p = append(p, n.Title)
		f(p, n)
		for _, c := range n.Children {
			rec(append([]string(nil), p...), c)
		}
	}
	rec(nil, n)
}
