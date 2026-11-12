// Navegador de objetos: categorias da árvore, abas de detalhe por tipo e ações
// de cada tipo (o "clique direito" do SQL Developer). by • DBA BRABO
package catalog

import (
	"errors"
	"fmt"
	"strings"
)

// Category é uma pasta do navegador (Tabelas, Views...).
type Category struct {
	Title string
	Types []string // OBJECT_TYPE do ALL_OBJECTS
	SQL   string   // lista própria (:1 = owner); vazio = ALL_OBJECTS pelos tipos
}

// Categories devolve as pastas do navegador na ordem do SQL Developer.
func Categories() []Category {
	return []Category{
		{Title: L("Tabelas", "Tables"), Types: []string{"TABLE"}, SQL: `select table_name object_name, 'TABLE' object_type,
		   case when temporary = 'Y' then 'GTT' when partitioned = 'YES' then 'PART' when iot_type is not null then 'IOT' end info
		   from all_tables where owner = :1 and nested = 'NO' and secondary = 'N'
		    and (owner, table_name) not in (select owner, mview_name from all_mviews where owner = :1)
		    and table_name not like 'BIN$%' order by table_name`},
		{Title: "Views", Types: []string{"VIEW"}},
		{Title: L("Índices", "Indexes"), Types: []string{"INDEX"}, SQL: `select index_name object_name, 'INDEX' object_type, table_name || ' ' || lower(index_type) info
		   from all_indexes where owner = :1 order by index_name`},
		{Title: "Packages", Types: []string{"PACKAGE"}},
		{Title: "Procedures", Types: []string{"PROCEDURE"}},
		{Title: L("Funções", "Functions"), Types: []string{"FUNCTION"}},
		{Title: L("Gatilhos", "Triggers"), Types: []string{"TRIGGER"}},
		{Title: L("Tipos", "Types"), Types: []string{"TYPE"}},
		{Title: L("Sequências", "Sequences"), Types: []string{"SEQUENCE"}},
		{Title: L("Sinônimos", "Synonyms"), Types: []string{"SYNONYM"}},
		{Title: L("Views materializadas", "Materialized views"), Types: []string{"MATERIALIZED VIEW"}},
		{Title: "Database links", Types: []string{"DATABASE LINK"}, SQL: `select db_link object_name, 'DATABASE LINK' object_type, host info
		   from all_db_links where owner = :1 order by db_link`},
		{Title: L("Jobs do scheduler", "Scheduler jobs"), Types: []string{"JOB"}},
		{Title: L("Filas (AQ)", "Queues (AQ)"), Types: []string{"QUEUE"}},
		{Title: L("Lixeira", "Recycle bin"), Types: []string{"RECYCLEBIN"}, SQL: `select object_name, 'RECYCLEBIN' object_type, original_name || ' (' || type || ')' info
		   from dba_recyclebin where owner = :1 order by droptime desc`},
	}
}

// CategorySQL devolve a consulta da pasta (:1 = owner).
func (c Category) CategorySQL() string {
	if c.SQL != "" {
		return c.SQL
	}
	in := strList(strings.Join(c.Types, ","), true)
	return `select object_name, object_type, case when status <> 'VALID' then status end info
	   from all_objects where owner = :1 and object_type in (` + in + `) and object_name not like 'BIN$%'
	    and nvl(subobject_name, 'x') = 'x' order by object_name`
}

// Detail é uma aba do visualizador de objeto.
type Detail struct {
	Title string
	SQL   string // :1 owner, :2 nome
	Text  bool   // primeira coluna em texto (DDL, código)
	Data  bool   // aba de dados (grade editável por ROWID)
}

func ddlType(typ string) string {
	switch typ {
	case "PACKAGE BODY":
		return "PACKAGE_BODY"
	case "TYPE BODY":
		return "TYPE_BODY"
	case "MATERIALIZED VIEW":
		return "MATERIALIZED_VIEW"
	case "DATABASE LINK":
		return "DB_LINK"
	case "JOB":
		return "PROCOBJ"
	case "JAVA SOURCE":
		return "JAVA_SOURCE"
	}
	return typ
}

// DDLQuery monta a consulta do DDL (SQL terminator e formatação ligados).
func DDLQuery(typ string) string {
	t := ddlType(typ)
	extra := ""
	if t == "TABLE" {
		// índices, comentários e grants junto, como o SQL Developer mostra
		extra = ` union all select dbms_metadata.get_dependent_ddl('COMMENT', :2, :1) from dual where exists (select 1 from all_col_comments where owner = :1 and table_name = :2 and comments is not null union select 1 from all_tab_comments where owner = :1 and table_name = :2 and comments is not null)
		 union all select dbms_metadata.get_ddl('INDEX', index_name, owner) from all_indexes where table_owner = :1 and table_name = :2 and index_type <> 'LOB' and (owner, index_name) not in (select owner, constraint_name from all_constraints where owner = :1 and table_name = :2 and constraint_type in ('P','U'))`
	}
	return `select dbms_metadata.get_ddl('` + t + `', :2, :1) from dual` + extra
}

const ddlSetup = `BEGIN
  DBMS_METADATA.SET_TRANSFORM_PARAM(DBMS_METADATA.SESSION_TRANSFORM, 'SQLTERMINATOR', TRUE);
  DBMS_METADATA.SET_TRANSFORM_PARAM(DBMS_METADATA.SESSION_TRANSFORM, 'PRETTY', TRUE);
  DBMS_METADATA.SET_TRANSFORM_PARAM(DBMS_METADATA.SESSION_TRANSFORM, 'SEGMENT_ATTRIBUTES', TRUE);
END;`

// DDLSetup é o bloco que deixa o DBMS_METADATA no formato de script.
func DDLSetup() string { return ddlSetup }

var colsSQL = `select column_id "#", column_name, data_type ||
        case when data_type in ('VARCHAR2','NVARCHAR2','CHAR','NCHAR','RAW') then '(' || decode(char_used, 'C', char_length || ' CHAR', data_length) || ')'
             when data_type = 'NUMBER' and data_precision is not null then '(' || data_precision || nvl2(nullif(data_scale, 0), ',' || data_scale, '') || ')'
        end data_type,
        nullable, data_default, (select comments from all_col_comments c where c.owner = t.owner and c.table_name = t.table_name and c.column_name = t.column_name) comments
   from all_tab_columns t where owner = :1 and table_name = :2 order by column_id`

var depsSQL = `select 'USA' direction, referenced_owner owner, referenced_name name, referenced_type type from all_dependencies where owner = :1 and name = :2
 union all
 select 'USADO POR', owner, name, type from all_dependencies where referenced_owner = :1 and referenced_name = :2
 order by 1, 2, 3`

var grantsSQL = `select grantee, privilege, grantable, grantor from all_tab_privs where table_schema = :1 and table_name = :2 order by grantee, privilege`

func detailsSQL(view, ownerCol, nameCol string) string {
	return "select * from " + view + " where " + ownerCol + " = :1 and " + nameCol + " = :2"
}

// Details devolve as abas do visualizador para o tipo.
func Details(typ string) []Detail {
	ddl := Detail{Title: "DDL", Text: true, SQL: DDLQuery(typ)}
	deps := Detail{Title: L("Dependências", "Dependencies"), SQL: depsSQL}
	grants := Detail{Title: "Grants", SQL: grantsSQL}
	code := Detail{Title: L("Código", "Code"), Text: true, SQL: `select text from all_source where owner = :1 and name = :2 and type = '` + typ + `' order by line`}
	errs := Detail{Title: L("Erros", "Errors"), SQL: `select line, position, attribute, text from all_errors where owner = :1 and name = :2 order by type, sequence`}
	switch typ {
	case "TABLE":
		return []Detail{
			{Title: L("Colunas", "Columns"), SQL: colsSQL},
			{Title: L("Dados", "Data"), Data: true},
			{Title: "Constraints", SQL: `select c.constraint_name, c.constraint_type type, c.status, c.validated,
			        (select listagg(column_name, ', ') within group (order by position) from all_cons_columns cc where cc.owner = c.owner and cc.constraint_name = c.constraint_name) columns,
			        c.r_owner, (select table_name from all_constraints r where r.owner = c.r_owner and r.constraint_name = c.r_constraint_name) r_table,
			        c.delete_rule, c.search_condition_vc condition
			   from all_constraints c where c.owner = :1 and c.table_name = :2 order by decode(c.constraint_type, 'P', 1, 'U', 2, 'R', 3, 4), c.constraint_name`},
			{Title: L("Índices", "Indexes"), SQL: `select i.index_name, i.index_type, i.uniqueness, i.status, i.tablespace_name,
			        (select listagg(column_name, ', ') within group (order by column_position) from all_ind_columns c where c.index_owner = i.owner and c.index_name = i.index_name) columns,
			        i.blevel, i.leaf_blocks, i.distinct_keys, i.clustering_factor, i.last_analyzed
			   from all_indexes i where i.table_owner = :1 and i.table_name = :2 order by i.index_name`},
			{Title: L("Estatísticas", "Statistics"), SQL: `select num_rows, blocks, empty_blocks, avg_row_len, sample_size, last_analyzed, stale_stats, stattype_locked
			   from all_tab_statistics where owner = :1 and table_name = :2 and object_type = 'TABLE'`},
			{Title: L("Estatísticas de colunas", "Column statistics"), SQL: `select column_name, num_distinct, num_nulls, density, histogram, num_buckets, last_analyzed
			   from all_tab_col_statistics where owner = :1 and table_name = :2 order by column_name`},
			{Title: L("Gatilhos", "Triggers"), SQL: `select trigger_name, trigger_type, triggering_event, status, when_clause from all_triggers where table_owner = :1 and table_name = :2 order by trigger_name`},
			{Title: L("Partições", "Partitions"), SQL: `select partition_name, partition_position pos, high_value, tablespace_name, num_rows, last_analyzed, compression
			   from all_tab_partitions where table_owner = :1 and table_name = :2 order by partition_position`},
			{Title: L("Segmentos", "Segments"), SQL: `select segment_name, partition_name, segment_type, tablespace_name, round(bytes/1048576, 1) mb, extents
			   from dba_segments where owner = :1 and (segment_name = :2 or segment_name in (select index_name from all_indexes where table_owner = :1 and table_name = :2)
			    or segment_name in (select segment_name from all_lobs where owner = :1 and table_name = :2)) order by bytes desc`},
			grants, deps,
			{Title: L("Detalhes", "Details"), SQL: detailsSQL("all_tables", "owner", "table_name")},
			ddl,
		}
	case "VIEW":
		return []Detail{{Title: L("Colunas", "Columns"), SQL: colsSQL}, {Title: L("Dados", "Data"), Data: true}, {Title: "SQL", Text: true, SQL: `select text_vc from all_views where owner = :1 and view_name = :2`},
			grants, deps, {Title: L("Detalhes", "Details"), SQL: detailsSQL("all_views", "owner", "view_name")}, ddl}
	case "MATERIALIZED VIEW":
		return []Detail{{Title: L("Colunas", "Columns"), SQL: colsSQL}, {Title: L("Dados", "Data"), Data: true},
			{Title: L("Detalhes", "Details"), SQL: detailsSQL("all_mviews", "owner", "mview_name")},
			{Title: L("Refresh", "Refresh"), SQL: `select * from all_mview_refresh_times where owner = :1 and name = :2`}, grants, deps, ddl}
	case "INDEX":
		return []Detail{
			{Title: L("Colunas", "Columns"), SQL: `select column_position "#", column_name, descend from all_ind_columns where index_owner = :1 and index_name = :2 order by column_position`},
			{Title: L("Detalhes", "Details"), SQL: detailsSQL("all_indexes", "owner", "index_name")},
			{Title: L("Partições", "Partitions"), SQL: `select partition_name, partition_position pos, status, tablespace_name, num_rows, last_analyzed from all_ind_partitions where index_owner = :1 and index_name = :2 order by partition_position`},
			ddl}
	case "PACKAGE", "TYPE":
		body := typ + " BODY"
		return []Detail{code,
			{Title: L("Corpo", "Body"), Text: true, SQL: `select text from all_source where owner = :1 and name = :2 and type = '` + body + `' order by line`},
			errs, grants, deps,
			{Title: L("Subprogramas", "Subprograms"), SQL: `select procedure_name, overload, subprogram_id, pipelined, deterministic from all_procedures where owner = :1 and object_name = :2 and procedure_name is not null order by subprogram_id`},
			ddl}
	case "PROCEDURE", "FUNCTION", "TRIGGER", "JAVA SOURCE":
		d := []Detail{code, errs, grants, deps}
		if typ == "TRIGGER" {
			d = append(d, Detail{Title: L("Detalhes", "Details"), SQL: detailsSQL("all_triggers", "owner", "trigger_name")})
		} else {
			d = append(d, Detail{Title: L("Argumentos", "Arguments"), SQL: `select position, argument_name, data_type, in_out, defaulted from all_arguments where owner = :1 and object_name = :2 and package_name is null order by overload, sequence`})
		}
		return append(d, ddl)
	case "SEQUENCE":
		return []Detail{{Title: L("Detalhes", "Details"), SQL: detailsSQL("all_sequences", "sequence_owner", "sequence_name")}, grants, deps, ddl}
	case "SYNONYM":
		return []Detail{{Title: L("Detalhes", "Details"), SQL: detailsSQL("all_synonyms", "owner", "synonym_name")}, deps, ddl}
	case "DATABASE LINK":
		return []Detail{{Title: L("Detalhes", "Details"), SQL: detailsSQL("all_db_links", "owner", "db_link")}, ddl}
	case "JOB":
		return []Detail{{Title: L("Detalhes", "Details"), SQL: detailsSQL("all_scheduler_jobs", "owner", "job_name")},
			{Title: L("Execuções", "Runs"), SQL: `select log_date, status, error#, run_duration, additional_info from all_scheduler_job_run_details where owner = :1 and job_name = :2 order by log_date desc`}, ddl}
	case "RECYCLEBIN":
		return []Detail{{Title: L("Detalhes", "Details"), SQL: `select * from dba_recyclebin where owner = :1 and object_name = :2`}}
	}
	return []Detail{{Title: L("Detalhes", "Details"), SQL: `select * from all_objects where owner = :1 and object_name = :2`}, deps, ddl}
}

// linkAsOwner testa (test=true) ou remove um database link. Link privado só
// o dono usa ou remove; se o dono é outro, roda numa procedure temporária no
// esquema dele (definer rights) e apaga a procedure no fim.
func linkAsOwner(owner, link string, test bool) string {
	q := `"` + strings.ReplaceAll(link, `"`, `""`) + `"`
	stmt := "DROP DATABASE LINK " + q
	if owner == "PUBLIC" {
		stmt = "DROP PUBLIC DATABASE LINK " + q
	}
	if test {
		stmt = "SELECT 'OK: ' || SYS_CONTEXT('USERENV', 'DB_NAME') || ' " + L("em", "at") + " ' || TO_CHAR(SYSDATE, 'YYYY-MM-DD HH24:MI:SS') FROM DUAL@" + q
	}
	run := "EXECUTE IMMEDIATE " + Lit(stmt)
	if test {
		run += " INTO r"
	}
	tmp := `"` + strings.ReplaceAll(owner, `"`, `""`) + `".ORABRABO_TMP_LINK`
	proc := "CREATE OR REPLACE PROCEDURE " + tmp + " (r OUT VARCHAR2) AS BEGIN r := NULL; " + run + "; END;"
	return `DECLARE
  r VARCHAR2(4000);
BEGIN
  IF ` + Lit(owner) + ` IN ('PUBLIC', SYS_CONTEXT('USERENV', 'SESSION_USER')) THEN
    ` + run + `;
  ELSE
    EXECUTE IMMEDIATE ` + Lit(proc) + `;
    BEGIN
      EXECUTE IMMEDIATE ` + Lit("BEGIN "+tmp+"(:r); END;") + ` USING OUT r;
    EXCEPTION WHEN OTHERS THEN
      EXECUTE IMMEDIATE ` + Lit("DROP PROCEDURE "+tmp) + `;
      RAISE;
    END;
    EXECUTE IMMEDIATE ` + Lit("DROP PROCEDURE "+tmp) + `;
  END IF;
  IF r IS NOT NULL THEN DBMS_OUTPUT.PUT_LINE(r); END IF;
END;
/`
}

// ObjRow monta a linha usada pelas ações de objeto.
func ObjRow(owner, name, typ string) Row {
	return Row{"OWNER": owner, "OBJECT_NAME": name, "OBJECT_TYPE": typ}
}

func objQ(r Row) string { return Q2(r["OWNER"], r["OBJECT_NAME"]) }

// ObjectActions devolve as ações de um tipo de objeto.
func ObjectActions(typ string) []*Action {
	drop := func(extra ...Field) *Action {
		return danger(act('X', L("Remover (DROP)", "Drop"), true, extra, func(r Row, x Vals) (Script, error) {
			t := r["OBJECT_TYPE"]
			s := "DROP " + t + " " + objQ(r)
			switch t {
			case "TABLE":
				s += iff(Yes(x["CASCADE"]), " CASCADE CONSTRAINTS", "") + iff(Yes(x["PURGE"]), " PURGE", "")
			case "TYPE":
				s += iff(Yes(x["FORCE"]), " FORCE", "")
			case "DATABASE LINK":
				return sqlS("%s", linkAsOwner(r["OWNER"], r["OBJECT_NAME"], false))
			case "JOB":
				s = "BEGIN DBMS_SCHEDULER.DROP_JOB(" + Lit(r["OWNER"]+"."+r["OBJECT_NAME"]) + ", force => TRUE); END;"
				return sqlS("%s", s)
			case "QUEUE":
				s = "BEGIN DBMS_AQADM.STOP_QUEUE(" + Lit(r["OWNER"]+"."+r["OBJECT_NAME"]) + "); DBMS_AQADM.DROP_QUEUE(" + Lit(r["OWNER"]+"."+r["OBJECT_NAME"]) + "); END;"
				return sqlS("%s", s)
			}
			return sqlS("%s;", s)
		}), "OBJECT_NAME")
	}
	compile := act('c', L("Compilar", "Compile"), true, []Field{fB("DEBUG", "DEBUG", false)}, func(r Row, x Vals) (Script, error) {
		s, err := compileStmt(r["OWNER"], r["OBJECT_NAME"], r["OBJECT_TYPE"])
		if err != nil {
			return Script{}, err
		}
		if Yes(x["DEBUG"]) {
			s = strings.Replace(s, " COMPILE", " COMPILE DEBUG", 1)
		}
		return sqlS("%s;", s)
	})
	grant := act('g', L("Conceder acesso", "Grant access"), true, []Field{
		fR("PRIV", L("Privilégio(s)", "Privilege(s)"), map[bool]string{true: "EXECUTE", false: "SELECT"}[typ == "PACKAGE" || typ == "PROCEDURE" || typ == "FUNCTION" || typ == "TYPE"]),
		fR("TO", L("Para (usuário/role)", "To (user/role)"), ""), fB("GRANT", "WITH GRANT OPTION", false),
	}, func(r Row, x Vals) (Script, error) {
		return sqlS("GRANT %s ON %s TO %s%s;", strings.ToUpper(x["PRIV"]), objQ(r), identList(x["TO"]), iff(Yes(x["GRANT"]), " WITH GRANT OPTION", ""))
	})
	synonym := act('y', L("Criar sinônimo", "Create synonym"), true, []Field{fT("NAME", L("Nome (vazio = mesmo)", "Name (empty = same)"), ""), fB("PUBLIC", "PUBLIC", false)},
		func(r Row, x Vals) (Script, error) {
			n := strings.TrimSpace(x["NAME"])
			if n == "" {
				n = r["OBJECT_NAME"]
			} else {
				n = strings.ToUpper(n)
			}
			return sqlS("CREATE OR REPLACE %sSYNONYM %s FOR %s;", iff(Yes(x["PUBLIC"]), "PUBLIC ", ""), Ident(n), objQ(r))
		})
	switch typ {
	case "TABLE":
		return []*Action{
			act('s', L("Coletar estatísticas", "Gather statistics"), true, []Field{fB("CASCADE", L("Índices junto", "Indexes too"), true)}, func(r Row, x Vals) (Script, error) {
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_STATS.GATHER_TABLE_STATS(ownname => %s, tabname => %s, cascade => %s);", Lit(r["OWNER"]), Lit(r["OBJECT_NAME"]), iff(Yes(x["CASCADE"]), "TRUE", "FALSE"))))
			}),
			rep('n', L("Contar linhas", "Count rows"), true, nil, func(r Row, _ Vals) (Script, error) {
				return reportQ("select 'Linhas / rows: ' || count(*) from %s", objQ(r))
			}),
			act('a', L("Adicionar coluna", "Add column"), true, []Field{fR("COL", L("Coluna", "Column"), ""), fR("TYPE", L("Tipo", "Type"), "VARCHAR2(100)"),
				fT("DEFAULT", L("Default (opcional)", "Default (optional)"), ""), fB("NOTNULL", "NOT NULL", false)},
				func(r Row, x Vals) (Script, error) {
					s := "ALTER TABLE " + objQ(r) + " ADD (" + IdentIn(x["COL"]) + " " + strings.ToUpper(strings.TrimSpace(x["TYPE"]))
					if d := strings.TrimSpace(x["DEFAULT"]); d != "" {
						s += " DEFAULT " + d
					}
					return sqlS("%s%s);", s, iff(Yes(x["NOTNULL"]), " NOT NULL", ""))
				}),
			act('m', L("Renomear", "Rename"), true, []Field{fR("NEW", L("Novo nome", "New name"), "")}, func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER TABLE %s RENAME TO %s;", objQ(r), IdentIn(x["NEW"]))
			}),
			act('o', L("Comentário", "Comment"), true, []Field{fT("TEXT", L("Comentário", "Comment"), "")}, func(r Row, x Vals) (Script, error) {
				return sqlS("COMMENT ON TABLE %s IS %s;", objQ(r), Lit(x["TEXT"]))
			}),
			act('t', L("Gatilhos ON/OFF", "Triggers ON/OFF"), true, []Field{fC("MODE", "", "ENABLE", "ENABLE", "DISABLE")}, func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER TABLE %s %s ALL TRIGGERS;", objQ(r), x["MODE"])
			}),
			act('k', L("Constraints ON/OFF", "Constraints ON/OFF"), true, []Field{fC("MODE", "", "ENABLE", "ENABLE", "DISABLE"), fR("NAME", "Constraint", "")}, func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER TABLE %s %s CONSTRAINT %s;", objQ(r), x["MODE"], IdentIn(x["NAME"]))
			}),
			act('h', "Shrink", true, []Field{fB("CASCADE", "CASCADE", true)}, func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER TABLE %s ENABLE ROW MOVEMENT;\nALTER TABLE %s SHRINK SPACE%s;", objQ(r), objQ(r), iff(Yes(x["CASCADE"]), " CASCADE", ""))
			}),
			act('v', L("Mover (MOVE ONLINE)", "Move (MOVE ONLINE)"), true, []Field{fT("TS", L("Tablespace (vazio = mesma)", "Tablespace (empty = same)"), ""), fB("ONLINE", "ONLINE", true)},
				func(r Row, x Vals) (Script, error) {
					ts := ""
					if t := strings.TrimSpace(x["TS"]); t != "" {
						ts = " TABLESPACE " + IdentIn(t)
					}
					return sqlS("ALTER TABLE %s MOVE%s%s;", objQ(r), ts, iff(Yes(x["ONLINE"]), " ONLINE", ""))
				}),
			act('l', L("Travar a tabela (LOCK)", "Lock table"), true, []Field{fC("MODE", "", "EXCLUSIVE", "ROW SHARE", "ROW EXCLUSIVE", "SHARE", "EXCLUSIVE"), fB("NOWAIT", "NOWAIT", true)},
				func(r Row, x Vals) (Script, error) {
					return sqlS("LOCK TABLE %s IN %s MODE%s;", objQ(r), x["MODE"], iff(Yes(x["NOWAIT"]), " NOWAIT", ""))
				}),
			danger(act('T', "Truncate", true, []Field{fB("STORAGE", "DROP STORAGE", true)}, func(r Row, x Vals) (Script, error) {
				return sqlS("TRUNCATE TABLE %s%s;", objQ(r), iff(Yes(x["STORAGE"]), " DROP STORAGE", " REUSE STORAGE"))
			}), "OBJECT_NAME"),
			grant, synonym,
			drop(fB("CASCADE", "CASCADE CONSTRAINTS", false), fB("PURGE", L("PURGE (não vai para a lixeira)", "PURGE (skip recycle bin)"), false)),
		}
	case "VIEW":
		return []*Action{compile, grant, synonym, drop()}
	case "MATERIALIZED VIEW":
		return []*Action{
			act('r', "Refresh", true, []Field{fC("METHOD", L("Método", "Method"), "?", "?", "C", "F")}, func(r Row, x Vals) (Script, error) {
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_MVIEW.REFRESH(%s, method => %s, atomic_refresh => FALSE);", Lit(r["OWNER"]+"."+r["OBJECT_NAME"]), Lit(x["METHOD"]))))
			}),
			compile, grant, synonym, drop(),
		}
	case "INDEX":
		return []*Action{
			act('r', "Rebuild", true, []Field{fT("TS", L("Tablespace (vazio = mesma)", "Tablespace (empty = same)"), ""), fB("ONLINE", "ONLINE", true)}, func(r Row, x Vals) (Script, error) {
				ts := ""
				if t := strings.TrimSpace(x["TS"]); t != "" {
					ts = " TABLESPACE " + IdentIn(t)
				}
				return sqlS("ALTER INDEX %s REBUILD%s%s;", objQ(r), ts, iff(Yes(x["ONLINE"]), " ONLINE", ""))
			}),
			act('u', L("Tornar inutilizável", "Make unusable"), true, nil, func(r Row, _ Vals) (Script, error) { return sqlS("ALTER INDEX %s UNUSABLE;", objQ(r)) }),
			act('v', L("Visível / invisível", "Visible / invisible"), true, []Field{fC("MODE", "", "INVISIBLE", "VISIBLE", "INVISIBLE")}, func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER INDEX %s %s;", objQ(r), x["MODE"])
			}),
			act('s', L("Coletar estatísticas", "Gather statistics"), true, nil, func(r Row, _ Vals) (Script, error) {
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_STATS.GATHER_INDEX_STATS(ownname => %s, indname => %s);", Lit(r["OWNER"]), Lit(r["OBJECT_NAME"]))))
			}),
			act('m', L("Renomear", "Rename"), true, []Field{fR("NEW", L("Novo nome", "New name"), "")}, func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER INDEX %s RENAME TO %s;", objQ(r), IdentIn(x["NEW"]))
			}),
			drop(),
		}
	case "PACKAGE", "PACKAGE BODY", "PROCEDURE", "FUNCTION", "TYPE", "JAVA SOURCE":
		return []*Action{compile, grant, synonym, drop(fB("FORCE", L("FORCE (tipo com dependentes)", "FORCE (type with dependents)"), false))}
	case "TRIGGER":
		return []*Action{
			act('e', L("Habilitar", "Enable"), true, nil, func(r Row, _ Vals) (Script, error) { return sqlS("ALTER TRIGGER %s ENABLE;", objQ(r)) }),
			act('d', L("Desabilitar", "Disable"), true, nil, func(r Row, _ Vals) (Script, error) { return sqlS("ALTER TRIGGER %s DISABLE;", objQ(r)) }),
			compile, drop(),
		}
	case "SEQUENCE":
		return []*Action{
			act('e', L("Alterar", "Alter"), true, []Field{fT("INC", "INCREMENT BY", ""), fT("CACHE", "CACHE", ""), fT("MAX", "MAXVALUE", "")}, func(r Row, x Vals) (Script, error) {
				s := ""
				for _, k := range []struct{ f, kw string }{{"INC", "INCREMENT BY"}, {"CACHE", "CACHE"}, {"MAX", "MAXVALUE"}} {
					if v := strings.TrimSpace(x[k.f]); v != "" {
						n, err := Num(k.f, v)
						if err != nil {
							return Script{}, err
						}
						s += " " + k.kw + " " + n
					}
				}
				if s == "" {
					return Script{}, errors.New(L("nada para alterar", "nothing to change"))
				}
				return sqlS("ALTER SEQUENCE %s%s;", objQ(r), s)
			}),
			act('r', L("Reiniciar (18c+)", "Restart (18c+)"), true, []Field{fR("START", "START WITH", "1")}, func(r Row, x Vals) (Script, error) {
				n, err := Num("START", x["START"])
				if err != nil {
					return Script{}, err
				}
				return sqlS("ALTER SEQUENCE %s RESTART START WITH %s;", objQ(r), n)
			}),
			grant, synonym, drop(),
		}
	case "SYNONYM":
		return []*Action{compile, drop()}
	case "DATABASE LINK":
		return []*Action{
			rep('t', L("Testar o link", "Test the link"), true, nil, func(r Row, _ Vals) (Script, error) {
				return reportQ("%s", linkAsOwner(r["OWNER"], r["OBJECT_NAME"], true))
			}),
			drop(),
		}
	case "JOB":
		return []*Action{
			act('r', L("Executar agora", "Run now"), true, nil, func(r Row, _ Vals) (Script, error) {
				return sqlS("%s", plsql("DBMS_SCHEDULER.RUN_JOB("+Lit(r["OWNER"]+"."+r["OBJECT_NAME"])+", use_current_session => FALSE);"))
			}),
			act('e', L("Habilitar", "Enable"), true, nil, func(r Row, _ Vals) (Script, error) {
				return sqlS("%s", plsql("DBMS_SCHEDULER.ENABLE("+Lit(r["OWNER"]+"."+r["OBJECT_NAME"])+");"))
			}),
			act('d', L("Desabilitar", "Disable"), true, nil, func(r Row, _ Vals) (Script, error) {
				return sqlS("%s", plsql("DBMS_SCHEDULER.DISABLE("+Lit(r["OWNER"]+"."+r["OBJECT_NAME"])+", force => TRUE);"))
			}),
			drop(),
		}
	case "RECYCLEBIN":
		return []*Action{
			act('r', L("Restaurar", "Restore"), true, []Field{fT("NEW", L("Renomear para (opcional)", "Rename to (optional)"), "")}, func(r Row, x Vals) (Script, error) {
				s := "FLASHBACK TABLE " + objQ(r) + " TO BEFORE DROP"
				if n := strings.TrimSpace(x["NEW"]); n != "" {
					s += " RENAME TO " + IdentIn(n)
				}
				return sqlS("%s;", s)
			}),
			danger(act('p', L("Expurgar", "Purge"), true, nil, func(r Row, _ Vals) (Script, error) { return sqlS("PURGE TABLE %s;", objQ(r)) }), "OBJECT_NAME"),
		}
	}
	return []*Action{grant, drop()}
}
