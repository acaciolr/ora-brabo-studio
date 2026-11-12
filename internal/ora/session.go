// Sessão de uma worksheet: conexão dedicada, transação aberta (sem autocommit,
// como no SQL Developer), DBMS_OUTPUT, explain, autotrace e comandos do cliente.
// by • DBA BRABO
package ora

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	go_ora "github.com/sijms/go-ora/v2"
)

// Cell é um valor já formatado para a grade.
type Cell struct {
	V    string
	Null bool
}

// Result é o resultado de um comando.
type Result struct {
	Stmt     Stmt
	Cols     []string
	Types    []string
	Rows     [][]Cell
	More     bool // havia mais linhas além do limite
	Affected int64
	Msg      string
	Err      error
	Elapsed  time.Duration
	Output   []string // DBMS_OUTPUT
	Text     string   // saída em texto (DESC, SHOW, plano, relatório)
}

// Session é uma conexão dedicada (uma por worksheet).
type Session struct {
	mu         sync.Mutex
	DB         *sql.DB
	conn       *sql.Conn
	tx         *sql.Tx
	AutoCommit bool
	ServerOut  bool
	Pending    bool // há DML sem commit
	FetchLimit int
	User       string
	cancel     context.CancelFunc
}

// NewSession abre uma sessão dedicada no pool.
func NewSession(db *sql.DB) (*Session, error) {
	c, err := db.Conn(context.Background())
	if err != nil {
		return nil, err
	}
	s := &Session{DB: db, conn: c, ServerOut: true, FetchLimit: 500}
	_ = c.QueryRowContext(context.Background(), "select user from dual").Scan(&s.User)
	if err := s.enableOutput(context.Background()); err != nil {
		s.ServerOut = false
	}
	return s, nil
}

// Close devolve a sessão (faz rollback do que estiver pendente).
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tx != nil {
		_ = s.tx.Rollback()
		s.tx = nil
	}
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
}

// Cancel interrompe o comando em execução.
func (s *Session) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

type execer interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

// ex devolve onde executar: a transação aberta (ou abre uma) ou a conexão (autocommit).
func (s *Session) ex(_ context.Context) (execer, error) {
	if s.conn == nil {
		return nil, errors.New("sessão fechada / session closed")
	}
	if s.AutoCommit {
		return s.conn, nil
	}
	if s.tx == nil {
		// contexto próprio: o do comando é cancelado no fim e levaria a transação junto
		tx, err := s.conn.BeginTx(context.Background(), nil)
		if err != nil {
			return nil, err
		}
		s.tx = tx
	}
	return s.tx, nil
}

func (s *Session) enableOutput(ctx context.Context) error {
	var e execer = s.conn
	if s.tx != nil {
		e = s.tx
	}
	_, err := e.ExecContext(ctx, "BEGIN DBMS_OUTPUT.ENABLE(NULL); END;")
	return err
}

// Commit grava a transação.
func (s *Session) Commit() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Pending = false
	if s.tx == nil {
		return nil
	}
	err := s.tx.Commit()
	s.tx = nil
	return err
}

// Rollback desfaz a transação.
func (s *Session) Rollback() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Pending = false
	if s.tx == nil {
		return nil
	}
	err := s.tx.Rollback()
	s.tx = nil
	return err
}

// Run executa um comando. binds: valores das :variaveis na ordem de Binds().
func (s *Session) Run(st Stmt, binds []any) Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	defer func() { cancel(); s.cancel = nil }()
	t0 := time.Now()
	r := s.run(ctx, st, binds)
	r.Stmt = st
	r.Elapsed = time.Since(t0)
	if s.ServerOut && (st.Kind != KindClient || isExec(st.Text)) {
		r.Output = s.readOutput(context.Background())
	}
	return r
}

func isExec(text string) bool {
	f := strings.Fields(text)
	return len(f) > 0 && (strings.EqualFold(f[0], "EXEC") || strings.EqualFold(f[0], "EXECUTE"))
}

func (s *Session) run(ctx context.Context, st Stmt, binds []any) Result {
	switch st.Kind {
	case KindClient:
		return s.client(ctx, st)
	case KindTx:
		up := strings.ToUpper(strings.Fields(st.Text)[0])
		if strings.HasPrefix(up, "COMMIT") {
			if err := s.commitLocked(); err != nil {
				return Result{Err: err}
			}
			return Result{Msg: L("Commit concluído.", "Commit complete.")}
		}
		if strings.HasPrefix(up, "ROLLBACK") && !regexp.MustCompile(`(?i)\bto\b`).MatchString(st.Text) {
			if err := s.rollbackLocked(); err != nil {
				return Result{Err: err}
			}
			return Result{Msg: L("Rollback concluído.", "Rollback complete.")}
		}
	}
	e, err := s.ex(ctx)
	if err != nil {
		return Result{Err: err}
	}
	switch st.Kind {
	case KindQuery:
		return s.query(ctx, e, st.Text, binds, s.FetchLimit)
	case KindPLSQL:
		if _, err := e.ExecContext(ctx, st.Text, binds...); err != nil {
			if !isCompileWarning(err) {
				return Result{Err: err}
			}
		}
		return s.afterPLSQL(ctx, e, st.Text)
	default:
		res, err := e.ExecContext(ctx, st.Text, binds...)
		if err != nil {
			return Result{Err: err}
		}
		n, _ := res.RowsAffected()
		if st.Kind == KindDML {
			s.Pending = true
			return Result{Affected: n, Msg: fmt.Sprintf(L("%d linha(s) afetada(s).", "%d row(s) affected."), n)}
		}
		return Result{Msg: ddlMessage(st.Text)}
	}
}

func (s *Session) commitLocked() error {
	s.Pending = false
	if s.tx == nil {
		return nil
	}
	err := s.tx.Commit()
	s.tx = nil
	return err
}

func (s *Session) rollbackLocked() error {
	s.Pending = false
	if s.tx == nil {
		return nil
	}
	err := s.tx.Rollback()
	s.tx = nil
	return err
}

func isCompileWarning(err error) bool {
	m := err.Error()
	return strings.Contains(m, "ORA-24344") || strings.Contains(m, "ORA-24333")
}

var rePLCreate = regexp.MustCompile(`(?is)^\s*create\s+(or\s+replace\s+)?((editionable|noneditionable)\s+)?(package\s+body|type\s+body|procedure|function|package|trigger|type|library|java\s+source)\s+(("[^"]+"|[\w$#]+)\s*\.\s*)?("[^"]+"|[\w$#]+)`)

// afterPLSQL confere ALL_ERRORS depois de um CREATE de código.
func (s *Session) afterPLSQL(ctx context.Context, e execer, text string) Result {
	m := rePLCreate.FindStringSubmatch(text)
	if m == nil {
		return Result{Msg: L("Procedimento PL/SQL concluído com sucesso.", "PL/SQL procedure successfully completed.")}
	}
	typ := strings.ToUpper(strings.Join(strings.Fields(m[4]), " "))
	owner := unq(m[6])
	name := unq(m[7])
	q := `select line, position, text, attribute from all_errors
	       where owner = nvl(:1, sys_context('userenv','current_schema')) and name = :2 and type = :3
	       order by sequence`
	rows, err := e.QueryContext(ctx, q, nullIfEmpty(owner), name, typ)
	if err != nil {
		return Result{Msg: typ + " " + name + L(" criado.", " created.")}
	}
	defer rows.Close()
	var lines []string
	nerr := 0
	for rows.Next() {
		var line, pos int
		var txt, attr string
		if rows.Scan(&line, &pos, &txt, &attr) == nil {
			lines = append(lines, fmt.Sprintf("%d/%d  %s: %s", line, pos, attr, strings.TrimSpace(txt)))
			if attr == "ERROR" {
				nerr++
			}
		}
	}
	if len(lines) == 0 {
		return Result{Msg: fmt.Sprintf(L("%s %s %s.", "%s %s compiled."), noun(typ), name, verbPT("compilado", typ))}
	}
	r := Result{Text: strings.Join(lines, "\n")}
	if nerr > 0 {
		r.Err = fmt.Errorf(L("%s %s %s com erros (%d).", "%s %s compiled with errors (%d)."), noun(typ), name, verbPT("compilado", typ), nerr)
	} else {
		r.Msg = fmt.Sprintf(L("%s %s %s com avisos.", "%s %s compiled with warnings."), noun(typ), name, verbPT("compilado", typ))
	}
	return r
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

func unq(s string) string {
	if strings.HasPrefix(s, `"`) {
		return strings.Trim(s, `"`)
	}
	return strings.ToUpper(s)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

var reDDLVerb = regexp.MustCompile(`(?is)^\s*(create|alter|drop|truncate|rename|grant|revoke|comment|analyze|audit|noaudit|purge|flashback)\s+(or\s+replace\s+)?(global\s+temporary\s+|unique\s+|bitmap\s+|public\s+|private\s+)?(\w+)?`)

func ddlMessage(text string) string {
	m := reDDLVerb.FindStringSubmatch(stripLeadingComments(text))
	if m == nil {
		return L("Comando executado.", "Statement executed.")
	}
	verb := strings.ToLower(m[1])
	obj := strings.ToUpper(m[4])
	pt := map[string]string{"create": "criado", "alter": "alterado", "drop": "removido",
		"truncate": "truncado", "rename": "renomeado", "comment": "comentado", "analyze": "analisado",
		"purge": "expurgado", "flashback": "recuperado"}
	en := map[string]string{"create": "created", "alter": "altered", "drop": "dropped", "truncate": "truncated",
		"rename": "renamed", "comment": "commented", "analyze": "analyzed", "purge": "purged", "flashback": "complete"}
	switch verb {
	case "grant":
		return L("Grant concluído.", "Grant succeeded.")
	case "revoke":
		return L("Revoke concluído.", "Revoke succeeded.")
	case "audit", "noaudit":
		return L("Auditoria alterada.", "Audit succeeded.")
	}
	if obj == "" {
		return L("Comando executado.", "Statement executed.")
	}
	return noun(obj) + " " + L(verbPT(pt[verb], obj), en[verb]) + "."
}

// nomes dos objetos em PT e gênero (true = feminino)
var nounsPT = map[string]struct {
	n string
	f bool
}{
	"TABLE": {"Tabela", true}, "VIEW": {"View", true}, "INDEX": {"Índice", false}, "PROCEDURE": {"Procedure", true},
	"FUNCTION": {"Função", true}, "PACKAGE": {"Package", false}, "PACKAGE BODY": {"Package body", false}, "TRIGGER": {"Gatilho", false},
	"SEQUENCE": {"Sequência", true}, "SYNONYM": {"Sinônimo", false}, "USER": {"Usuário", false}, "ROLE": {"Role", true},
	"TABLESPACE": {"Tablespace", false}, "TYPE": {"Tipo", false}, "TYPE BODY": {"Type body", false}, "DATABASE": {"Banco", false},
	"SYSTEM": {"Sistema", false}, "SESSION": {"Sessão", true}, "PROFILE": {"Profile", false}, "DIRECTORY": {"Diretório", false},
	"MATERIALIZED": {"View materializada", true}, "PLUGGABLE": {"PDB", false}, "CLUSTER": {"Cluster", false},
	"CONTEXT": {"Contexto", false}, "LIBRARY": {"Biblioteca", true}, "JAVA": {"Java", false}, "SCHEMA": {"Schema", false},
}

func noun(obj string) string {
	if L("pt", "en") == "en" {
		return titleCase(obj)
	}
	if x, ok := nounsPT[strings.ToUpper(obj)]; ok {
		return x.n
	}
	return titleCase(obj)
}

// verbPT põe o particípio no gênero do objeto ("criado" → "criada").
func verbPT(v, obj string) string {
	if x, ok := nounsPT[strings.ToUpper(obj)]; ok && x.f && strings.HasSuffix(v, "o") {
		return v[:len(v)-1] + "a"
	}
	return v
}

// query executa um SELECT e traz até limit linhas.
func (s *Session) query(ctx context.Context, e execer, q string, binds []any, limit int) Result {
	rows, err := e.QueryContext(ctx, q, binds...)
	if err != nil {
		return Result{Err: err}
	}
	defer rows.Close()
	return Collect(rows, limit)
}

// Collect lê as linhas de um *sql.Rows já formatadas.
func Collect(rows *sql.Rows, limit int) Result {
	var r Result
	cols, err := rows.Columns()
	if err != nil {
		r.Err = err
		return r
	}
	r.Cols = cols
	if ct, err := rows.ColumnTypes(); err == nil {
		for _, t := range ct {
			r.Types = append(r.Types, t.DatabaseTypeName())
		}
	}
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	for rows.Next() {
		if limit > 0 && len(r.Rows) >= limit {
			r.More = true
			break
		}
		if err := rows.Scan(ptrs...); err != nil {
			r.Err = err
			return r
		}
		row := make([]Cell, len(cols))
		for i, v := range vals {
			row[i] = Format(v)
		}
		r.Rows = append(r.Rows, row)
	}
	if err := rows.Err(); err != nil {
		r.Err = err
	}
	if r.More {
		r.Msg = fmt.Sprintf(L("%d linhas buscadas (há mais).", "%d rows fetched (more available)."), len(r.Rows))
	} else {
		r.Msg = fmt.Sprintf(L("%d linha(s) selecionada(s).", "%d row(s) selected."), len(r.Rows))
	}
	return r
}

// Format converte um valor do driver em texto.
func Format(v any) Cell {
	switch x := v.(type) {
	case nil:
		return Cell{Null: true}
	case string:
		return Cell{V: x}
	case []byte:
		return Cell{V: strings.ToUpper(hex.EncodeToString(x))}
	case time.Time:
		if x.Nanosecond() != 0 {
			return Cell{V: x.Format("2006-01-02 15:04:05.000000")}
		}
		return Cell{V: x.Format("2006-01-02 15:04:05")}
	case float64:
		return Cell{V: strconv.FormatFloat(x, 'f', -1, 64)}
	case float32:
		return Cell{V: strconv.FormatFloat(float64(x), 'f', -1, 32)}
	case int64:
		return Cell{V: strconv.FormatInt(x, 10)}
	case bool:
		if x {
			return Cell{V: "TRUE"}
		}
		return Cell{V: "FALSE"}
	case fmt.Stringer:
		return Cell{V: x.String()}
	}
	return Cell{V: fmt.Sprint(v)}
}

// readOutput lê o buffer do DBMS_OUTPUT.
func (s *Session) readOutput(ctx context.Context) []string {
	if s.conn == nil {
		return nil
	}
	var e execer = s.conn
	if s.tx != nil {
		e = s.tx
	}
	var out []string
	for i := 0; i < 100000; i++ {
		var line string
		var status int64
		_, err := e.ExecContext(ctx, "BEGIN DBMS_OUTPUT.GET_LINE(:1, :2); END;",
			go_ora.Out{Dest: &line, Size: 32767}, go_ora.Out{Dest: &status})
		if err != nil || status != 0 {
			break
		}
		out = append(out, line)
	}
	return out
}

// QueryText roda uma consulta e junta a primeira coluna em texto (plano, relatórios).
func (s *Session) QueryText(q string, args ...any) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, err := s.ex(context.Background())
	if err != nil {
		return "", err
	}
	return queryText(context.Background(), e, q, args...)
}

func queryText(ctx context.Context, e execer, q string, args ...any) (string, error) {
	rows, err := e.QueryContext(ctx, q, args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var v sql.NullString
		if err := rows.Scan(&v); err != nil {
			return "", err
		}
		b.WriteString(v.String)
		b.WriteByte('\n')
	}
	return b.String(), rows.Err()
}

// Explain devolve o plano estimado (EXPLAIN PLAN + DBMS_XPLAN.DISPLAY).
func (s *Session) Explain(st Stmt) Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx := context.Background()
	e, err := s.ex(ctx)
	if err != nil {
		return Result{Err: err}
	}
	id := fmt.Sprintf("OBS%d", time.Now().UnixNano()%1e9)
	if _, err := e.ExecContext(ctx, "EXPLAIN PLAN SET STATEMENT_ID = '"+id+"' FOR "+st.Text); err != nil {
		return Result{Err: err, Stmt: st}
	}
	txt, err := queryText(ctx, e, "SELECT plan_table_output FROM TABLE(DBMS_XPLAN.DISPLAY('PLAN_TABLE', :1, 'ALL'))", id)
	_, _ = e.ExecContext(ctx, "DELETE FROM plan_table WHERE statement_id = :1", id)
	if err != nil {
		return Result{Err: err, Stmt: st}
	}
	return Result{Stmt: st, Text: txt, Msg: L("Plano de execução estimado.", "Estimated execution plan.")}
}

// estatísticas da sessão usadas no autotrace
var autotraceStats = []string{"recursive calls", "db block gets", "consistent gets", "physical reads",
	"redo size", "bytes sent via SQL*Net to client", "bytes received via SQL*Net from client",
	"SQL*Net roundtrips to/from client", "sorts (memory)", "sorts (disk)"}

func (s *Session) myStats(ctx context.Context, e execer) (map[string]int64, error) {
	q := `select n.name, s.value from v$mystat s join v$statname n on n.statistic# = s.statistic#
	       where n.name in ('` + strings.Join(autotraceStats, "','") + `')`
	rows, err := e.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]int64{}
	for rows.Next() {
		var n string
		var v int64
		if rows.Scan(&n, &v) == nil {
			m[n] = v
		}
	}
	return m, rows.Err()
}

// Autotrace executa o comando, mostra o plano real (ALLSTATS LAST) e as estatísticas.
func (s *Session) Autotrace(st Stmt, binds []any) Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx := context.Background()
	e, err := s.ex(ctx)
	if err != nil {
		return Result{Err: err}
	}
	t0 := time.Now()
	before, err := s.myStats(ctx, e)
	if err != nil {
		return Result{Stmt: st, Err: fmt.Errorf(L("autotrace precisa de SELECT em V$MYSTAT/V$STATNAME (role SELECT_CATALOG_ROLE): %v",
			"autotrace needs SELECT on V$MYSTAT/V$STATNAME (SELECT_CATALOG_ROLE): %v"), err)}
	}
	_, _ = e.ExecContext(ctx, "ALTER SESSION SET statistics_level = ALL")
	// marca o comando para achar o cursor dele no V$SQL (o prev_sql_id da
	// sessão aponta para os comandos internos do driver)
	tag := fmt.Sprintf("/* orabrabo-autotrace %d */ ", time.Now().UnixNano())
	text := tag + st.Text
	var r Result
	if st.Kind == KindQuery {
		r = s.query(ctx, e, text, binds, 0) // busca tudo, como o autotrace
	} else {
		res, err := e.ExecContext(ctx, text, binds...)
		if err != nil {
			r.Err = err
		} else {
			r.Affected, _ = res.RowsAffected()
			s.Pending = true
		}
	}
	r.Stmt = st
	var sqlID string
	var child int64
	idErr := e.QueryRowContext(ctx, `select sql_id, child_number from v$sql
	  where sql_text like :1 and sql_text not like '%v$sql%'
	  order by last_active_time desc fetch first 1 rows only`, strings.TrimSpace(tag)+"%").Scan(&sqlID, &child)
	_, _ = e.ExecContext(ctx, "ALTER SESSION SET statistics_level = TYPICAL")
	var b strings.Builder
	if idErr != nil {
		b.WriteString(L("Plano indisponível: ", "Plan unavailable: ") + idErr.Error() + "\n")
	}
	if sqlID != "" {
		plan, err := queryText(ctx, e, "SELECT plan_table_output FROM TABLE(DBMS_XPLAN.DISPLAY_CURSOR(:1, :2, 'ALLSTATS LAST'))", sqlID, child)
		if err != nil {
			b.WriteString(L("Plano indisponível: ", "Plan unavailable: ") + err.Error() + "\n")
		} else {
			b.WriteString(plan)
		}
	}
	after, err := s.myStats(ctx, e)
	if err == nil {
		b.WriteString("\n" + L("Estatísticas", "Statistics") + "\n----------------------------------------------------------\n")
		for _, n := range autotraceStats {
			fmt.Fprintf(&b, "%12d  %s\n", after[n]-before[n], n)
		}
		fmt.Fprintf(&b, "%12d  %s\n", len(r.Rows), L("linhas processadas", "rows processed"))
	}
	r.Text = b.String()
	r.Elapsed = time.Since(t0)
	if r.Err == nil {
		r.Msg = L("Autotrace concluído.", "Autotrace complete.")
	}
	r.Rows = nil // a grade do autotrace fica vazia; o foco é o plano
	r.Cols = nil
	return r
}

// ---- comandos do cliente (subconjunto do SQL*Plus) ----

func (s *Session) client(ctx context.Context, st Stmt) Result {
	f := strings.Fields(st.Text)
	cmd := strings.ToUpper(f[0])
	arg := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(st.Text[len(f[0]):]), ";"))
	switch {
	case cmd == "DESC" || cmd == "DESCRIBE":
		e, err := s.ex(ctx)
		if err != nil {
			return Result{Err: err}
		}
		t, err := Describe(ctx, e, arg)
		if err != nil {
			return Result{Err: err}
		}
		return Result{Text: t}
	case cmd == "SET":
		a := strings.ToUpper(arg)
		switch {
		case strings.HasPrefix(a, "SERVEROUT") && strings.Contains(a, "ON"):
			s.ServerOut = true
			if err := s.enableOutput(ctx); err != nil {
				return Result{Err: err}
			}
			return Result{Msg: "serveroutput ON"}
		case strings.HasPrefix(a, "SERVEROUT"):
			s.ServerOut = false
			if e, err := s.ex(ctx); err == nil {
				_, _ = e.ExecContext(ctx, "BEGIN DBMS_OUTPUT.DISABLE; END;")
			}
			return Result{Msg: "serveroutput OFF"}
		case strings.HasPrefix(a, "AUTOCOMMIT"):
			on := strings.Contains(a, "ON")
			if on && s.tx != nil {
				if err := s.commitLocked(); err != nil {
					return Result{Err: err}
				}
			}
			s.AutoCommit = on
			return Result{Msg: "autocommit " + map[bool]string{true: "ON", false: "OFF"}[on]}
		}
		return Result{Msg: L("SET ignorado: ", "SET ignored: ") + arg}
	case cmd == "SHOW":
		return s.show(ctx, arg)
	case cmd == "PROMPT":
		return Result{Text: arg}
	case cmd == "EXEC" || cmd == "EXECUTE":
		blk := Stmt{Text: "BEGIN " + strings.TrimSuffix(arg, ";") + "; END;", Kind: KindPLSQL}
		return s.run(ctx, blk, nil)
	case cmd == "REM" || cmd == "REMARK" || cmd == "CLEAR" || cmd == "COL" || cmd == "COLUMN" ||
		cmd == "DEFINE" || cmd == "UNDEFINE" || cmd == "WHENEVER" || cmd == "SPOOL":
		return Result{Msg: L("Comando do SQL*Plus ignorado: ", "SQL*Plus command ignored: ") + cmd}
	}
	return Result{Err: fmt.Errorf(L("comando do cliente não suportado: %s", "unsupported client command: %s"), cmd)}
}

func (s *Session) show(ctx context.Context, arg string) Result {
	f := strings.Fields(strings.ToUpper(arg))
	if len(f) == 0 {
		return Result{Err: errors.New("SHOW ?")}
	}
	e, err := s.ex(ctx)
	if err != nil {
		return Result{Err: err}
	}
	switch {
	case f[0] == "USER":
		var u string
		_ = e.QueryRowContext(ctx, "select user from dual").Scan(&u)
		return Result{Text: "USER " + L("é", "is") + " \"" + u + "\""}
	case strings.HasPrefix(f[0], "ERR"):
		q := `select type||' '||name, line||'/'||position, text from user_errors order by type, name, sequence`
		var args []any
		if len(f) >= 2 {
			name := f[len(f)-1]
			q = `select type||' '||name, line||'/'||position, text from user_errors where name = :1 order by sequence`
			args = append(args, strings.Trim(name, `"`))
		}
		rows, err := e.QueryContext(ctx, q, args...)
		if err != nil {
			return Result{Err: err}
		}
		defer rows.Close()
		var b strings.Builder
		for rows.Next() {
			var o, p, t string
			if rows.Scan(&o, &p, &t) == nil {
				fmt.Fprintf(&b, "%-30s %-9s %s\n", o, p, strings.TrimSpace(t))
			}
		}
		if b.Len() == 0 {
			return Result{Text: L("Sem erros.", "No errors.")}
		}
		return Result{Text: b.String()}
	case strings.HasPrefix(f[0], "PARAM") || strings.HasPrefix(f[0], "SGA") || f[0] == "CON_NAME" || f[0] == "PDBS":
		var q string
		var args []any
		switch {
		case strings.HasPrefix(f[0], "SGA"):
			q = "select name, value from v$sga"
		case f[0] == "CON_NAME":
			q = "select sys_context('userenv','con_name') con_name from dual"
		case f[0] == "PDBS":
			q = "select con_id, name, open_mode, restricted from v$pdbs order by con_id"
		default:
			q = "select name, type, display_value from v$parameter order by name"
			if len(f) > 1 {
				q = "select name, type, display_value from v$parameter where name like '%'||lower(:1)||'%' order by name"
				args = append(args, f[1])
			}
		}
		r := s.query(ctx, e, q, args, 0)
		return r
	}
	return Result{Err: fmt.Errorf("SHOW %s: %s", arg, L("não suportado", "not supported"))}
}

// Describe monta o DESC de uma tabela/view (resolve sinônimo privado e público).
func Describe(ctx context.Context, e execer, obj string) (string, error) {
	owner, name := SplitName(obj)
	if owner == "" {
		var n int
		_ = e.QueryRowContext(ctx, "select count(*) from user_tab_columns where table_name = :1", name).Scan(&n)
		if n > 0 {
			_ = e.QueryRowContext(ctx, "select user from dual").Scan(&owner)
		} else {
			var o, t sql.NullString
			_ = e.QueryRowContext(ctx, `select table_owner, table_name from (
			      select table_owner, table_name from all_synonyms
			       where synonym_name = :1 and owner in (user, 'PUBLIC')
			       order by decode(owner, 'PUBLIC', 2, 1)) where rownum = 1`, name).Scan(&o, &t)
			if o.Valid {
				owner, name = o.String, t.String
			}
		}
	}
	q := `select column_name, decode(nullable,'N','NOT NULL',''),
	        data_type || case
	          when data_type in ('VARCHAR2','NVARCHAR2','CHAR','NCHAR','RAW','UROWID')
	            then '('||decode(char_used,'C',char_length||' CHAR',data_length)||')'
	          when data_type = 'NUMBER' and data_precision is not null
	            then '('||data_precision||nvl2(nullif(data_scale,0),','||data_scale,'')||')'
	          when data_type = 'FLOAT' then '('||data_precision||')'
	          else '' end
	      from all_tab_columns
	     where owner = :1 and table_name = :2
	     order by column_id`
	rows, err := e.QueryContext(ctx, q, owner, name)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var b strings.Builder
	fmt.Fprintf(&b, "%-32s %-9s %s\n%s %s %s\n", L("Nome", "Name"), "Null?", L("Tipo", "Type"),
		strings.Repeat("-", 32), strings.Repeat("-", 9), strings.Repeat("-", 28))
	n := 0
	for rows.Next() {
		var c, nn, t sql.NullString
		if rows.Scan(&c, &nn, &t) == nil {
			fmt.Fprintf(&b, "%-32s %-9s %s\n", c.String, nn.String, t.String)
			n++
		}
	}
	if n == 0 {
		return "", fmt.Errorf(L("objeto %s não existe ou sem acesso", "object %s does not exist or no access"), obj)
	}
	return b.String(), nil
}

// SplitName separa OWNER.NOME respeitando aspas; sem aspas vira maiúsculo.
func SplitName(s string) (owner, name string) {
	s = strings.TrimSpace(s)
	parts := []string{}
	cur := strings.Builder{}
	inq := false
	for _, r := range s {
		switch {
		case r == '"':
			inq = !inq
			cur.WriteRune(r)
		case r == '.' && !inq:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	parts = append(parts, cur.String())
	if len(parts) >= 2 {
		return unq(parts[0]), unq(parts[1])
	}
	return "", unq(parts[0])
}
