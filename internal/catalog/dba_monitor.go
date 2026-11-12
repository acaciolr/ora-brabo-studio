// Aba DBA · Monitoramento: sessões, bloqueios, locks, top SQL, long ops, esperas,
// métricas, alert log, objetos inválidos e Data Guard. by • DBA BRABO
package catalog

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func sessKey(r Row) string {
	k := r["SID"] + "," + r["SERIAL#"]
	if i := r["INST_ID"]; i != "" {
		k += ",@" + i
	}
	return k
}

func killAct() *Action {
	return danger(act('k', L("Matar sessão (KILL)", "Kill session"), true, []Field{fB("IMMEDIATE", "IMMEDIATE", true)},
		func(r Row, x Vals) (Script, error) {
			return sqlS("ALTER SYSTEM KILL SESSION %s%s;", Lit(sessKey(r)), iff(Yes(x["IMMEDIATE"]), " IMMEDIATE", ""))
		}), "SID")
}

func sqlTextRep(k rune) *Action {
	return rep(k, L("Texto do SQL", "SQL text"), true, nil, func(r Row, _ Vals) (Script, error) {
		if r["SQL_ID"] == "" {
			return Script{}, errors.New(L("sem SQL_ID", "no SQL_ID"))
		}
		return reportQ("select sql_fulltext from v$sqlarea where sql_id = %s", Lit(r["SQL_ID"]))
	})
}

func planRep(k rune) *Action {
	return rep(k, L("Plano (cursor)", "Plan (cursor)"), true, nil, func(r Row, _ Vals) (Script, error) {
		if r["SQL_ID"] == "" {
			return Script{}, errors.New(L("sem SQL_ID", "no SQL_ID"))
		}
		return reportQ("select plan_table_output from table(dbms_xplan.display_cursor(%s, null, 'ALL +PEEKED_BINDS'))", Lit(r["SQL_ID"]))
	})
}

func compileStmt(owner, name, typ string) (string, error) {
	obj := Q2(owner, name)
	switch typ {
	case "PACKAGE BODY":
		return "ALTER PACKAGE " + obj + " COMPILE BODY", nil
	case "TYPE BODY":
		return "ALTER TYPE " + obj + " COMPILE BODY", nil
	case "SYNONYM":
		if owner == "PUBLIC" {
			return "ALTER PUBLIC SYNONYM " + Ident(name) + " COMPILE", nil
		}
		return "ALTER SYNONYM " + obj + " COMPILE", nil
	case "PACKAGE", "PROCEDURE", "FUNCTION", "TRIGGER", "VIEW", "TYPE", "MATERIALIZED VIEW", "JAVA SOURCE", "JAVA CLASS", "DIMENSION", "LIBRARY":
		return "ALTER " + typ + " " + obj + " COMPILE", nil
	}
	return "", fmt.Errorf(L("não sei compilar %s", "cannot compile %s"), typ)
}

func monitorNode() *Node {
	waits := v("mo.sesswaits", L("Esperas da sessão", "Session waits"), `select event, wait_class, total_waits, round(time_waited_micro/1e6, 1) secs, max_wait
	   from gv$session_event where sid = :1 and inst_id = nvl(:2, inst_id) and wait_class <> 'Idle' order by time_waited_micro desc`)
	cursors := v("mo.sesscursors", L("Cursores abertos", "Open cursors"), `select sql_id, cursor_type, sql_text from gv$open_cursor where sid = :1 and inst_id = nvl(:2, inst_id) order by sql_id`)
	sesStats := v("mo.sessstats", L("Estatísticas da sessão", "Session statistics"), `select n.name, s.value from gv$sesstat s join v$statname n on n.statistic# = s.statistic#
	   where s.sid = :1 and s.inst_id = nvl(:2, s.inst_id) and s.value > 0 order by s.value desc`)

	sessActs := func() []*Action {
		return []*Action{
			killAct(),
			act('d', L("Desconectar (DISCONNECT)", "Disconnect"), true, []Field{fC("WHEN", L("Quando", "When"), "IMMEDIATE", "IMMEDIATE", "POST_TRANSACTION")},
				func(r Row, x Vals) (Script, error) {
					return sqlS("ALTER SYSTEM DISCONNECT SESSION %s %s;", Lit(r["SID"]+","+r["SERIAL#"]), x["WHEN"])
				}),
			act('c', L("Cancelar o SQL em execução (18c+)", "Cancel running SQL (18c+)"), true, nil, func(r Row, _ Vals) (Script, error) {
				return sqlS("ALTER SYSTEM CANCEL SQL %s;", Lit(sessKey(r)))
			}),
			sqlTextRep('s'), planRep('p'),
			act('t', L("Ligar trace (10046 com binds e esperas)", "Enable trace (10046 with binds and waits)"), true, nil, func(r Row, _ Vals) (Script, error) {
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_MONITOR.SESSION_TRACE_ENABLE(session_id => %s, serial_num => %s, waits => TRUE, binds => TRUE);", r["SID"], r["SERIAL#"])))
			}),
			act('u', L("Desligar trace", "Disable trace"), true, nil, func(r Row, _ Vals) (Script, error) {
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_MONITOR.SESSION_TRACE_DISABLE(session_id => %s, serial_num => %s);", r["SID"], r["SERIAL#"])))
			}),
			drill('w', L("Esperas", "Waits"), waits, two("SID", "INST_ID")),
			drill('o', L("Cursores abertos", "Open cursors"), cursors, two("SID", "INST_ID")),
			drill('e', L("Estatísticas", "Statistics"), sesStats, two("SID", "INST_ID")),
		}
	}
	sessSQL := `select s.inst_id, s.sid, s.serial#, s.username, s.status, s.osuser, s.machine, s.program, s.module,
	        s.sql_id, s.event, s.wait_class, s.last_call_et et, s.blocking_session blk, s.logon_time, s.service_name, s.type
	   from gv$session s %s order by s.status, s.last_call_et desc`
	sessions := v("mo.sessions", L("Sessões de usuário", "User sessions"), fmt.Sprintf(sessSQL, "where s.type = 'USER'"), sessActs()...)
	sessions.Refresh = 5
	sessions.KeyCol = "SID"
	active := v("mo.active", L("Sessões ativas", "Active sessions"), fmt.Sprintf(sessSQL, "where s.type = 'USER' and s.status = 'ACTIVE' and s.wait_class <> 'Idle'"), sessActs()...)
	active.Refresh = 3
	active.KeyCol = "SID"
	allSess := v("mo.allsessions", L("Todas as sessões (com background)", "All sessions (with background)"), fmt.Sprintf(sessSQL, ""), sessActs()...)
	allSess.KeyCol = "SID"

	blockers := v("mo.blockers", L("Bloqueios (árvore)", "Blocking (tree)"), `with w as (
	   select inst_id, sid, serial#, username, blocking_instance, blocking_session, event, last_call_et, sql_id, row_wait_obj#, status
	     from gv$session
	    where blocking_session is not null
	       or (inst_id, sid) in (select blocking_instance, blocking_session from gv$session where blocking_session is not null))
	 select lpad(' ', 2 * (level - 1)) || sid tree, inst_id, sid, serial#, username, status, event, last_call_et et, sql_id,
	        (select owner || '.' || object_name from dba_objects where object_id = w.row_wait_obj# and w.row_wait_obj# > 0) object
	   from w start with blocking_session is null
	 connect by prior sid = blocking_session and prior inst_id = blocking_instance`,
		killAct(), sqlTextRep('s'), planRep('p'))
	blockers.Refresh = 5
	blockers.Help = L("A raiz de cada árvore é quem segura o lock.", "The root of each tree is the session holding the lock.")

	locks := v("mo.locks", L("Objetos travados (DML)", "Locked objects (DML)"), `select l.inst_id, l.session_id sid, s.serial#, s.username, o.owner, o.object_name, o.object_type,
	        decode(l.locked_mode, 0, 'None', 1, 'Null', 2, 'Row-S', 3, 'Row-X', 4, 'Share', 5, 'S/Row-X', 6, 'Exclusive') lock_mode,
	        s.status, s.sql_id, s.last_call_et et
	   from gv$locked_object l
	   join dba_objects o on o.object_id = l.object_id
	   join gv$session s on s.sid = l.session_id and s.inst_id = l.inst_id
	  order by o.owner, o.object_name`, killAct(), sqlTextRep('s'))

	topSQL := v("mo.topsql", "Top SQL", top(100, `select sql_id, plan_hash_value phv, executions execs, round(elapsed_time/1e6, 1) ela_s,
	        round(elapsed_time/nullif(executions, 0)/1e3, 1) ms_exec, round(cpu_time/1e6, 1) cpu_s, buffer_gets, disk_reads,
	        rows_processed, substr(sql_text, 1, 200) sql_text
	   from v$sqlstats order by elapsed_time desc`),
		sqlTextRep('s'), planRep('p'),
		pack(rep('h', L("Planos no histórico (AWR)", "Plans in history (AWR)"), true, nil, func(r Row, _ Vals) (Script, error) {
			return reportQ("select plan_table_output from table(dbms_xplan.display_awr(%s))", Lit(r["SQL_ID"]))
		}), "diag"),
		pack(act('t', "SQL Tuning Advisor", true, []Field{fR("SEC", L("Limite (segundos)", "Time limit (seconds)"), "60")}, func(r Row, x Vals) (Script, error) {
			n, err := Num("SEC", x["SEC"])
			if err != nil {
				return Script{}, err
			}
			t := fmt.Sprintf("OBS_%s_%d", r["SQL_ID"], time.Now().Unix()%100000)
			return Script{Kind: Report, Text: fmt.Sprintf("DECLARE\n  t VARCHAR2(64);\nBEGIN\n  t := DBMS_SQLTUNE.CREATE_TUNING_TASK(sql_id => %s, time_limit => %s, task_name => %s);\n  DBMS_SQLTUNE.EXECUTE_TUNING_TASK(t);\nEND;\n/\nselect dbms_sqltune.report_tuning_task(%s) from dual",
				Lit(r["SQL_ID"]), n, Lit(t), Lit(t))}, nil
		}), "tuning"),
		pack(rep('m', L("Relatório do SQL Monitor", "SQL Monitor report"), true, nil, func(r Row, _ Vals) (Script, error) {
			return reportQ("select dbms_sqltune.report_sql_monitor(sql_id => %s, type => 'TEXT') from dual", Lit(r["SQL_ID"]))
		}), "tuning"),
		act('b', L("Criar SQL plan baseline (SPM)", "Create SQL plan baseline (SPM)"), true, []Field{fB("FIXED", L("Fixo", "Fixed"), false)}, func(r Row, x Vals) (Script, error) {
			return sqlS("DECLARE\n  n PLS_INTEGER;\nBEGIN\n  n := DBMS_SPM.LOAD_PLANS_FROM_CURSOR_CACHE(sql_id => %s, plan_hash_value => %s, fixed => %s);\n  DBMS_OUTPUT.PUT_LINE(n || ' plano(s) carregado(s)');\nEND;",
				Lit(r["SQL_ID"]), r["PHV"], Lit(iff(Yes(x["FIXED"]), "YES", "NO")))
		}),
		act('x', L("Tirar do shared pool (PURGE)", "Purge from shared pool"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("DECLARE\n  n VARCHAR2(100);\nBEGIN\n  SELECT address || ',' || hash_value INTO n FROM v$sqlarea WHERE sql_id = %s;\n  SYS.DBMS_SHARED_POOL.PURGE(n, 'C');\nEND;", Lit(r["SQL_ID"]))
		}),
	)
	topSQL.KeyCol = "SQL_ID"

	longops := v("mo.longops", L("Operações longas", "Long operations"), `select inst_id, sid, serial#, opname, target, sofar, totalwork,
	        round(sofar * 100 / nullif(totalwork, 0), 1) pct, elapsed_seconds ela_s, time_remaining rem_s, message
	   from gv$session_longops where sofar < totalwork order by start_time desc`, killAct())
	longops.Refresh = 5

	events := v("mo.events", L("Eventos de espera (desde o startup)", "Wait events (since startup)"), top(60, `select event, wait_class, total_waits, round(time_waited_micro/1e6) secs,
	        round(time_waited_micro/nullif(total_waits, 0)/1000, 2) avg_ms
	   from v$system_event where wait_class <> 'Idle' order by time_waited_micro desc`))
	metrics := v("mo.metrics", L("Métricas (último minuto)", "Metrics (last minute)"), `select metric_name, round(value, 2) value, metric_unit
	   from v$sysmetric where group_id = 2 order by metric_name`)
	metrics.Refresh = 15

	alert := v("mo.alert", L("Alert log (últimas 500)", "Alert log (last 500)"), top(500, `select originating_timestamp ts, message_level lvl, message_text
	   from v$diag_alert_ext order by originating_timestamp desc`))
	alertErr := v("mo.alert.err", L("Alert log: só erros", "Alert log: errors only"), top(500, `select originating_timestamp ts, message_level lvl, message_text
	   from v$diag_alert_ext where message_text like '%ORA-%' or message_text like '%TNS-%' or message_text like '%Error%'
	  order by originating_timestamp desc`))
	incidents := v("mo.incidents", L("Incidentes (ADR)", "Incidents (ADR)"), `select incident_id, problem_key, create_time, status, flood_controlled
	   from v$diag_incident order by create_time desc`)

	errs := v("mo.errors", L("Erros de compilação", "Compilation errors"), `select owner, name, type, line, position, attribute, text
	   from dba_errors where owner = :1 and name = :2 order by sequence`)
	invalid := v("mo.invalid", L("Objetos inválidos", "Invalid objects"), `select owner, object_name, object_type, status, last_ddl_time
	   from dba_objects where status = 'INVALID' order by owner, object_type, object_name`,
		act('c', L("Compilar", "Compile"), true, nil, func(r Row, _ Vals) (Script, error) {
			s, err := compileStmt(r["OWNER"], r["OBJECT_NAME"], r["OBJECT_TYPE"])
			if err != nil {
				return Script{}, err
			}
			return sqlS("%s;", s)
		}),
		act('s', L("Recompilar o schema (UTL_RECOMP)", "Recompile schema (UTL_RECOMP)"), true, []Field{fR("SCHEMA", "Schema", "{OWNER}"), fN("PAR", L("Paralelo (0 = serial)", "Parallel (0 = serial)"), "0")},
			func(_ Row, x Vals) (Script, error) {
				n, err := Num("PAR", x["PAR"])
				if err != nil {
					return Script{}, err
				}
				if n == "0" {
					return sqlS("%s", plsql("SYS.UTL_RECOMP.RECOMP_SERIAL("+Lit(strings.ToUpper(x["SCHEMA"]))+");"))
				}
				return sqlS("%s", plsql(fmt.Sprintf("SYS.UTL_RECOMP.RECOMP_PARALLEL(%s, %s);", n, Lit(strings.ToUpper(x["SCHEMA"])))))
			}),
		act('a', L("Recompilar tudo (utlrp)", "Recompile all (utlrp)"), false, nil, func(Row, Vals) (Script, error) {
			return sqlS("%s", plsql("SYS.UTL_RECOMP.RECOMP_PARALLEL(0);"))
		}),
		drill('v', L("Erros", "Errors"), errs, two("OWNER", "OBJECT_NAME")),
	)

	dgStats := v("mo.dg", "Data Guard", `select name, value, unit, time_computed from v$dataguard_stats order by name`)
	dgDest := v("mo.dgdest", L("Data Guard: destinos", "Data Guard: destinations"), `select dest_id, status, type, database_mode, recovery_mode, protection_mode,
	        destination, gap_status, error
	   from v$archive_dest_status where status <> 'INACTIVE' order by dest_id`)
	dgProc := v("mo.dgproc", L("Data Guard: processos", "Data Guard: processes"), `select name, pid, role, action, client_role, group#, thread#, sequence#, block#
	   from v$dataguard_process order by name`)

	tsAlert := v("mo.tsfull", L("Tablespaces acima de 85%", "Tablespaces above 85%"), `select tablespace_name, round(used_percent, 1) pct_used,
	        round(used_space * (select block_size from dba_tablespaces t where t.tablespace_name = m.tablespace_name)/1048576) used_mb,
	        round(tablespace_size * (select block_size from dba_tablespaces t where t.tablespace_name = m.tablespace_name)/1048576) max_mb
	   from dba_tablespace_usage_metrics m where used_percent > 85 order by used_percent desc`)

	return &Node{Title: L("Monitoramento", "Monitoring"), Children: []*Node{
		{Title: L("Sessões ativas", "Active sessions"), View: active},
		{Title: L("Sessões de usuário", "User sessions"), View: sessions},
		{Title: L("Todas as sessões", "All sessions"), View: allSess},
		{Title: L("Bloqueios", "Blocking"), View: blockers},
		{Title: L("Objetos travados", "Locked objects"), View: locks},
		{Title: "Top SQL", View: topSQL},
		{Title: L("Operações longas", "Long operations"), View: longops},
		{Title: L("Eventos de espera", "Wait events"), View: events},
		{Title: L("Métricas", "Metrics"), View: metrics},
		{Title: L("Tablespaces cheias", "Full tablespaces"), View: tsAlert},
		{Title: "Alert log", View: alert},
		{Title: L("Alert log: erros", "Alert log: errors"), View: alertErr},
		{Title: L("Incidentes", "Incidents"), View: incidents},
		{Title: L("Objetos inválidos", "Invalid objects"), View: invalid},
		{Title: "Data Guard", Children: []*Node{
			{Title: L("Estatísticas", "Statistics"), View: dgStats},
			{Title: L("Destinos", "Destinations"), View: dgDest},
			{Title: L("Processos", "Processes"), View: dgProc},
		}},
	}}
}

// DBA monta a árvore inteira da aba DBA (chamar depois de definir o idioma).
func DBA() *Node {
	return &Node{Title: "DBA", Children: []*Node{
		databaseNode(), storageNode(), securityNode(), backupNode(), datapumpNode(),
		perfNode(), rsrcNode(), schedNode(), monitorNode(),
	}}
}
