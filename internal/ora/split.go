// Divisor de script no estilo SQL*Plus: SQL termina em ';', PL/SQL em '/' sozinho
// na linha, comandos do cliente (DESC, SET, SHOW, PROMPT, EXEC) terminam na linha.
// by • DBA BRABO
package ora

import (
	"regexp"
	"strings"
)

// Kind diz como um comando deve ser executado.
type Kind int

const (
	KindQuery  Kind = iota // SELECT / WITH: devolve linhas
	KindDML                // INSERT/UPDATE/DELETE/MERGE: linhas afetadas
	KindDDL                // CREATE/ALTER/DROP/GRANT... (exceto PL/SQL)
	KindPLSQL              // bloco anônimo ou CREATE de código PL/SQL
	KindClient             // comando do cliente (DESC, SET SERVEROUTPUT, SHOW...)
	KindTx                 // COMMIT / ROLLBACK / SAVEPOINT
)

// Stmt é um comando do script com a posição (em bytes) no texto original.
type Stmt struct {
	Text  string
	Kind  Kind
	Start int
	End   int
	Line  int // linha (1-based) onde começa
}

var (
	rePLSQLStart = regexp.MustCompile(`(?is)^\s*(declare|begin|create\s+(or\s+replace\s+)?((editionable|noneditionable)\s+)?(procedure|function|package|trigger|type|library|java)\b)`)
	reClient     = regexp.MustCompile(`(?i)^\s*(desc|describe|set|show|prompt|exec|execute|rem|remark|spool|clear|col|column|define|undefine|whenever|@)\b`)
	reQuery      = regexp.MustCompile(`(?is)^\s*(\(\s*)*(select|with)\b`)
	reDML        = regexp.MustCompile(`(?i)^\s*(insert|update|delete|merge|lock\s+table|call)\b`)
	reTx         = regexp.MustCompile(`(?i)^\s*(commit|rollback|savepoint)\b`)
)

// Classify descobre o tipo de um comando já isolado.
func Classify(s string) Kind {
	t := stripLeadingComments(s)
	switch {
	case rePLSQLStart.MatchString(t):
		return KindPLSQL
	case reClient.MatchString(t):
		return KindClient
	case reQuery.MatchString(t):
		return KindQuery
	case reDML.MatchString(t):
		return KindDML
	case reTx.MatchString(t):
		return KindTx
	}
	return KindDDL
}

func stripLeadingComments(s string) string {
	for {
		t := strings.TrimLeft(s, " \t\r\n")
		switch {
		case strings.HasPrefix(t, "--"):
			i := strings.IndexByte(t, '\n')
			if i < 0 {
				return ""
			}
			s = t[i+1:]
		case strings.HasPrefix(t, "/*"):
			i := strings.Index(t[2:], "*/")
			if i < 0 {
				return ""
			}
			s = t[i+4:]
		default:
			return t
		}
	}
}

// Split divide o script em comandos.
func Split(script string) []Stmt {
	var out []Stmt
	n := len(script)
	i := 0
	for i < n {
		// pula espaço em branco entre comandos
		for i < n && strings.ContainsRune(" \t\r\n", rune(script[i])) {
			i++
		}
		if i >= n {
			break
		}
		start := i
		// linha atual até o fim para decidir o tipo
		head := script[start:]
		// '/' sozinho numa linha sem comando antes: ignora (re-executar no SQL*Plus)
		if lineIs(head, "/") {
			i = nextLine(script, i)
			continue
		}
		body := stripLeadingComments(head)
		if body == "" {
			break // só comentários até o fim
		}
		kind := Classify(head)
		var end, next int
		switch kind {
		case KindPLSQL:
			end, next = scanUntilSlash(script, i)
		case KindClient:
			// termina no fim da linha (aceita ';' final)
			off := len(head) - len(body)
			le := strings.IndexByte(script[i+off:], '\n')
			if le < 0 {
				end, next = n, n
			} else {
				end, next = i+off+le, i+off+le+1
			}
		default:
			end, next = scanUntilSemicolon(script, i)
		}
		text := strings.TrimSpace(script[start:end])
		if kind != KindPLSQL {
			text = strings.TrimSpace(strings.TrimSuffix(text, ";"))
		} else {
			text = strings.TrimRight(text, " \t\r\n")
		}
		if stripLeadingComments(text) != "" {
			out = append(out, Stmt{Text: text, Kind: Classify(text), Start: start, End: end,
				Line: strings.Count(script[:start], "\n") + 1})
		}
		i = next
	}
	return out
}

func lineIs(s, want string) bool {
	le := strings.IndexByte(s, '\n')
	if le < 0 {
		le = len(s)
	}
	return strings.TrimSpace(s[:le]) == want
}

func nextLine(s string, i int) int {
	le := strings.IndexByte(s[i:], '\n')
	if le < 0 {
		return len(s)
	}
	return i + le + 1
}

// scanUntilSemicolon acha o ';' que fecha o comando, fora de aspas e comentários.
// Uma linha só com '/' também fecha. Uma linha em branco também (como no SQL*Plus
// com SQLBLANKLINES OFF), para o F9 não juntar comandos sem ';'.
func scanUntilSemicolon(s string, i int) (end, next int) {
	n := len(s)
	lineStart := i
	for i < n {
		c := s[i]
		switch {
		case c == '\'' || (c == '"'):
			i = skipQuoted(s, i)
			continue
		case (c == 'q' || c == 'Q') && i+2 < n && s[i+1] == '\'':
			i = skipQQuote(s, i)
			continue
		case c == '-' && i+1 < n && s[i+1] == '-':
			le := strings.IndexByte(s[i:], '\n')
			if le < 0 {
				return n, n
			}
			i += le
			continue
		case c == '/' && i+1 < n && s[i+1] == '*':
			ce := strings.Index(s[i+2:], "*/")
			if ce < 0 {
				return n, n
			}
			i += ce + 4
			continue
		case c == ';':
			return i + 1, i + 1
		case c == '\n':
			i++
			// linha seguinte é '/' sozinho ou em branco: fecha aqui
			if i < n && lineIs(s[i:], "/") {
				return i - 1, nextLine(s, i)
			}
			if i < n && lineIs(s[i:], "") && i > lineStart {
				return i - 1, nextLine(s, i)
			}
			lineStart = i
			continue
		}
		i++
	}
	return n, n
}

// scanUntilSlash: PL/SQL vai até a linha com '/' sozinho (ou fim do texto).
func scanUntilSlash(s string, i int) (end, next int) {
	n := len(s)
	for i < n {
		c := s[i]
		switch {
		case c == '\'':
			i = skipQuoted(s, i)
			continue
		case (c == 'q' || c == 'Q') && i+2 < n && s[i+1] == '\'':
			i = skipQQuote(s, i)
			continue
		case c == '-' && i+1 < n && s[i+1] == '-':
			le := strings.IndexByte(s[i:], '\n')
			if le < 0 {
				return n, n
			}
			i += le
			continue
		case c == '/' && i+1 < n && s[i+1] == '*':
			ce := strings.Index(s[i+2:], "*/")
			if ce < 0 {
				return n, n
			}
			i += ce + 4
			continue
		case c == '\n':
			i++
			if i < n && lineIs(s[i:], "/") {
				return i - 1, nextLine(s, i)
			}
			continue
		}
		i++
	}
	return n, n
}

func skipQuoted(s string, i int) int {
	q := s[i]
	i++
	for i < len(s) {
		if s[i] == q {
			if i+1 < len(s) && s[i+1] == q { // '' escapado
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return len(s)
}

// q'[...]' , q'{...}', q'<...>', q'(...)' ou q'X...X'
func skipQQuote(s string, i int) int {
	open := s[i+2]
	close := open
	switch open {
	case '[':
		close = ']'
	case '{':
		close = '}'
	case '<':
		close = '>'
	case '(':
		close = ')'
	}
	j := strings.Index(s[i+3:], string(close)+"'")
	if j < 0 {
		return len(s)
	}
	return i + 3 + j + 2
}

// AtCursor devolve o comando que contém a posição (offset em bytes). Se o cursor
// estiver entre comandos, pega o anterior mais próximo; se antes do primeiro, o primeiro.
func AtCursor(script string, pos int) (Stmt, bool) {
	st := Split(script)
	if len(st) == 0 {
		return Stmt{}, false
	}
	best := -1
	for k, s := range st {
		if pos >= s.Start && pos <= s.End+1 {
			return s, true
		}
		if s.Start <= pos {
			best = k
		}
	}
	if best < 0 {
		best = 0
	}
	return st[best], true
}

var reBind = regexp.MustCompile(`:([A-Za-z][A-Za-z0-9_$#]*)`)

// Binds lista as variáveis :nome do comando (fora de aspas e comentários), sem repetir.
// Em trigger (:new/:old) e PL/SQL não procura.
func Binds(s Stmt) []string {
	if s.Kind == KindPLSQL || s.Kind == KindClient {
		return nil
	}
	clean := maskLiterals(s.Text)
	seen := map[string]bool{}
	var out []string
	for _, m := range reBind.FindAllStringSubmatchIndex(clean, -1) {
		if m[0] > 0 && clean[m[0]-1] == ':' { // ::
			continue
		}
		name := clean[m[2]:m[3]]
		u := strings.ToUpper(name)
		if !seen[u] {
			seen[u] = true
			out = append(out, name)
		}
	}
	return out
}

// maskLiterals troca o conteúdo de aspas e comentários por espaços (mesmo tamanho).
func maskLiterals(s string) string {
	b := []byte(s)
	n := len(b)
	for i := 0; i < n; {
		c := b[i]
		var j int
		switch {
		case c == '\'' || c == '"':
			j = skipQuoted(s, i)
		case (c == 'q' || c == 'Q') && i+2 < n && b[i+1] == '\'':
			j = skipQQuote(s, i)
		case c == '-' && i+1 < n && b[i+1] == '-':
			le := strings.IndexByte(s[i:], '\n')
			if le < 0 {
				j = n
			} else {
				j = i + le
			}
		case c == '/' && i+1 < n && b[i+1] == '*':
			ce := strings.Index(s[i+2:], "*/")
			if ce < 0 {
				j = n
			} else {
				j = i + ce + 4
			}
		default:
			i++
			continue
		}
		for k := i; k < j && k < n; k++ {
			if b[k] != '\n' {
				b[k] = ' '
			}
		}
		i = j
	}
	return string(b)
}
