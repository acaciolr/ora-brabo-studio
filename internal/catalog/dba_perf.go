// Aba DBA · Performance: AWR (snapshots, relatórios, baselines, templates,
// configuração), ADDM, ASH, Statspack e SQL Tuning Advisor. AWR/ASH/ADDM pedem
// o Diagnostics Pack; SQL Tuning e SQL Monitor pedem o Tuning Pack.
// by • DBA BRABO
package catalog

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const dbidInst = "(select dbid from v$database), (select instance_number from v$instance)"

func snapRange(x Vals) (string, string, error) {
	b, err := Num("BEGIN", x["BEGIN"])
	if err != nil {
		return "", "", err
	}
	e, err := Num("END", x["END"])
	if err != nil {
		return "", "", err
	}
	return b, e, nil
}

func awrReport(b, e string, html bool) (Script, error) {
	fn := "awr_report_text"
	if html {
		fn = "awr_report_html"
	}
	return reportQ("select output from table(dbms_workload_repository.%s(%s, %s, %s))", fn, dbidInst, b, e)
}

func perfNode() *Node {
	rangeFields := []Field{fR("BEGIN", L("Snapshot inicial", "Begin snapshot"), "{PREV}"), fR("END", L("Snapshot final", "End snapshot"), "{SNAP_ID}")}
	snaps := v("pf.snaps", L("Snapshots AWR", "AWR snapshots"), top(500, `select snap_id, snap_id - 1 prev, instance_number inst, begin_interval_time, end_interval_time,
	        snap_level, error_count
	   from dba_hist_snapshot where dbid = (select dbid from v$database) order by snap_id desc`),
		act('n', L("Tirar snapshot agora", "Take snapshot now"), false, nil, func(Row, Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_WORKLOAD_REPOSITORY.CREATE_SNAPSHOT;"))
		}),
		rep('r', L("Relatório AWR (texto)", "AWR report (text)"), true, rangeFields, func(_ Row, x Vals) (Script, error) {
			b, e, err := snapRange(x)
			if err != nil {
				return Script{}, err
			}
			return awrReport(b, e, false)
		}),
		rep('h', L("Relatório AWR (HTML, salve com Ctrl+S)", "AWR report (HTML, save with Ctrl+S)"), true, rangeFields, func(_ Row, x Vals) (Script, error) {
			b, e, err := snapRange(x)
			if err != nil {
				return Script{}, err
			}
			return awrReport(b, e, true)
		}),
		rep('d', L("Comparar dois períodos (AWR diff)", "Compare two periods (AWR diff)"), true, []Field{
			fR("B1", L("Período 1: início", "Period 1: begin"), ""), fR("E1", L("Período 1: fim", "Period 1: end"), ""),
			fR("B2", L("Período 2: início", "Period 2: begin"), "{PREV}"), fR("E2", L("Período 2: fim", "Period 2: end"), "{SNAP_ID}"),
		}, func(_ Row, x Vals) (Script, error) {
			var n [4]string
			for i, k := range []string{"B1", "E1", "B2", "E2"} {
				s, err := Num(k, x[k])
				if err != nil {
					return Script{}, err
				}
				n[i] = s
			}
			return reportQ("select output from table(dbms_workload_repository.awr_diff_report_text(%s, %s, %s, %s, %s, %s))",
				dbidInst, n[0], n[1], dbidInst, n[2], n[3])
		}),
		rep('s', L("Relatório de um SQL (awrsqrpt)", "SQL report (awrsqrpt)"), true, append(rangeFields, fR("SQLID", "SQL_ID", "")), func(_ Row, x Vals) (Script, error) {
			b, e, err := snapRange(x)
			if err != nil {
				return Script{}, err
			}
			return reportQ("select output from table(dbms_workload_repository.awr_sql_report_text(%s, %s, %s, %s))", dbidInst, b, e, Lit(strings.TrimSpace(x["SQLID"])))
		}),
		act('a', L("Rodar ADDM neste intervalo", "Run ADDM on this range"), true, rangeFields, func(_ Row, x Vals) (Script, error) {
			b, e, err := snapRange(x)
			if err != nil {
				return Script{}, err
			}
			t := fmt.Sprintf("OBS_ADDM_%s_%s", b, e)
			return Script{Kind: Report, Text: fmt.Sprintf("DECLARE\n  t VARCHAR2(100) := %s;\nBEGIN\n  BEGIN DBMS_ADDM.DELETE(t); EXCEPTION WHEN OTHERS THEN NULL; END;\n  DBMS_ADDM.ANALYZE_DB(t, %s, %s);\nEND;\n/\nselect dbms_addm.get_report(%s) from dual", Lit(t), b, e, Lit(t))}, nil
		}),
		act('b', L("Criar baseline deste intervalo", "Create baseline from this range"), true, append(rangeFields,
			fR("NAME", L("Nome", "Name"), "BASE_{SNAP_ID}"), fT("EXP", L("Expira em (dias; vazio = nunca)", "Expires in (days; empty = never)"), "")),
			func(_ Row, x Vals) (Script, error) {
				b, e, err := snapRange(x)
				if err != nil {
					return Script{}, err
				}
				exp, err := numOrEmpty("EXP", x["EXP"])
				if err != nil {
					return Script{}, err
				}
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_WORKLOAD_REPOSITORY.CREATE_BASELINE(%s);",
					named("start_snap_id", b, "end_snap_id", e, "baseline_name", Lit(x["NAME"]), "expiration", exp))))
			}),
		danger(act('x', L("Apagar intervalo de snapshots", "Drop snapshot range"), true, []Field{fR("LOW", L("De", "From"), "{SNAP_ID}"), fR("HIGH", L("Até", "To"), "{SNAP_ID}")},
			func(_ Row, x Vals) (Script, error) {
				a, err := Num("LOW", x["LOW"])
				if err != nil {
					return Script{}, err
				}
				b, err := Num("HIGH", x["HIGH"])
				if err != nil {
					return Script{}, err
				}
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_WORKLOAD_REPOSITORY.DROP_SNAPSHOT_RANGE(low_snap_id => %s, high_snap_id => %s);", a, b)))
			}), "SNAP_ID"),
	)
	snaps.Pack = "diag"

	baselines := v("pf.baselines", "Baselines", `select baseline_id, baseline_name, baseline_type, start_snap_id, end_snap_id,
	        start_snap_time, end_snap_time, expiration, creation_time
	   from dba_hist_baseline order by baseline_id`,
		act('n', L("Criar", "Create"), false, []Field{fR("NAME", L("Nome", "Name"), ""), fR("BEGIN", L("Snapshot inicial", "Begin snapshot"), ""), fR("END", L("Snapshot final", "End snapshot"), ""),
			fT("EXP", L("Expira em (dias; vazio = nunca)", "Expires in (days; empty = never)"), "")},
			func(_ Row, x Vals) (Script, error) {
				b, e, err := snapRange(x)
				if err != nil {
					return Script{}, err
				}
				exp, err := numOrEmpty("EXP", x["EXP"])
				if err != nil {
					return Script{}, err
				}
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_WORKLOAD_REPOSITORY.CREATE_BASELINE(%s);",
					named("start_snap_id", b, "end_snap_id", e, "baseline_name", Lit(x["NAME"]), "expiration", exp))))
			}),
		act('m', L("Renomear", "Rename"), true, []Field{fR("NEW", L("Novo nome", "New name"), "")}, func(r Row, x Vals) (Script, error) {
			return sqlS("%s", plsql(fmt.Sprintf("DBMS_WORKLOAD_REPOSITORY.RENAME_BASELINE(old_baseline_name => %s, new_baseline_name => %s);", Lit(r["BASELINE_NAME"]), Lit(x["NEW"]))))
		}),
		act('w', L("Janela móvel do sistema (dias)", "System moving window (days)"), false, []Field{fR("DAYS", L("Dias", "Days"), "8")}, func(_ Row, x Vals) (Script, error) {
			n, err := Num("DAYS", x["DAYS"])
			if err != nil {
				return Script{}, err
			}
			return sqlS("%s", plsql("DBMS_WORKLOAD_REPOSITORY.MODIFY_BASELINE_WINDOW_SIZE(window_size => "+n+");"))
		}),
		rep('r', L("Relatório AWR da baseline", "Baseline AWR report"), true, nil, func(r Row, _ Vals) (Script, error) {
			return awrReport(r["START_SNAP_ID"], r["END_SNAP_ID"], false)
		}),
		danger(act('x', L("Remover", "Drop"), true, []Field{fB("CASCADE", L("Apagar os snapshots junto", "Drop its snapshots too"), false)},
			func(r Row, x Vals) (Script, error) {
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_WORKLOAD_REPOSITORY.DROP_BASELINE(baseline_name => %s, cascade => %s);", Lit(r["BASELINE_NAME"]), iff(Yes(x["CASCADE"]), "TRUE", "FALSE"))))
			}), "BASELINE_NAME"),
	)
	baselines.Pack = "diag"

	templates := v("pf.templates", L("Templates de baseline", "Baseline templates"), `select template_name, template_type, baseline_name_prefix, start_time, end_time,
	        day_of_week, hour_in_day, duration, expiration, repeat_interval
	   from dba_hist_baseline_template order by template_name`,
		act('n', L("Criar (única vez)", "Create (single)"), false, []Field{
			fR("NAME", L("Nome do template", "Template name"), ""), fR("BASE", L("Nome da baseline", "Baseline name"), ""),
			fR("START", L("Início (AAAA-MM-DD HH24:MI)", "Start (YYYY-MM-DD HH24:MI)"), time.Now().Add(24*time.Hour).Format("2006-01-02 15:04")),
			fR("END", L("Fim (AAAA-MM-DD HH24:MI)", "End (YYYY-MM-DD HH24:MI)"), time.Now().Add(26*time.Hour).Format("2006-01-02 15:04")),
			fT("EXP", L("Expira em (dias)", "Expires in (days)"), "30"),
		}, func(_ Row, x Vals) (Script, error) {
			exp, err := numOrEmpty("EXP", x["EXP"])
			if err != nil {
				return Script{}, err
			}
			return sqlS("%s", plsql(fmt.Sprintf("DBMS_WORKLOAD_REPOSITORY.CREATE_BASELINE_TEMPLATE(%s);", named(
				"start_time", "TO_DATE("+Lit(x["START"])+", 'YYYY-MM-DD HH24:MI')",
				"end_time", "TO_DATE("+Lit(x["END"])+", 'YYYY-MM-DD HH24:MI')",
				"baseline_name", Lit(x["BASE"]), "template_name", Lit(x["NAME"]), "expiration", exp))))
		}),
		act('p', L("Criar (repetitivo)", "Create (repeating)"), false, []Field{
			fR("NAME", L("Nome do template", "Template name"), ""), fR("PREFIX", L("Prefixo das baselines", "Baseline prefix"), "SEMANAL_"),
			fC("DAY", L("Dia da semana", "Day of week"), "MONDAY", "ALL", "MONDAY", "TUESDAY", "WEDNESDAY", "THURSDAY", "FRIDAY", "SATURDAY", "SUNDAY"),
			fR("HOUR", L("Hora (0-23)", "Hour (0-23)"), "9"), fR("DUR", L("Duração (horas)", "Duration (hours)"), "3"),
			fR("START", L("Começa em (AAAA-MM-DD)", "Starts on (YYYY-MM-DD)"), time.Now().Format("2006-01-02")),
			fR("END", L("Termina em (AAAA-MM-DD)", "Ends on (YYYY-MM-DD)"), time.Now().AddDate(1, 0, 0).Format("2006-01-02")),
			fT("EXP", L("Cada baseline expira em (dias)", "Each baseline expires in (days)"), "30"),
		}, func(_ Row, x Vals) (Script, error) {
			h, err := Num("HOUR", x["HOUR"])
			if err != nil {
				return Script{}, err
			}
			d, err := Num("DUR", x["DUR"])
			if err != nil {
				return Script{}, err
			}
			exp, err := numOrEmpty("EXP", x["EXP"])
			if err != nil {
				return Script{}, err
			}
			return sqlS("%s", plsql(fmt.Sprintf("DBMS_WORKLOAD_REPOSITORY.CREATE_BASELINE_TEMPLATE(%s);", named(
				"day_of_week", Lit(x["DAY"]), "hour_in_day", h, "duration", d,
				"start_time", "TO_DATE("+Lit(x["START"])+", 'YYYY-MM-DD')", "end_time", "TO_DATE("+Lit(x["END"])+", 'YYYY-MM-DD')",
				"baseline_name_prefix", Lit(x["PREFIX"]), "template_name", Lit(x["NAME"]), "expiration", exp))))
		}),
		danger(act('x', L("Remover", "Drop"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_WORKLOAD_REPOSITORY.DROP_BASELINE_TEMPLATE(template_name => "+Lit(r["TEMPLATE_NAME"])+");"))
		}), "TEMPLATE_NAME"),
	)
	templates.Pack = "diag"

	awrCfg := v("pf.awrcfg", L("Configuração do AWR", "AWR settings"), `select dbid, snap_interval, retention, topnsql from dba_hist_wr_control`,
		act('e', L("Alterar", "Change"), false, []Field{
			fT("INTERVAL", L("Intervalo (minutos)", "Interval (minutes)"), "60"), fT("RETENTION", L("Retenção (dias)", "Retention (days)"), "8"),
			fT("TOPN", L("Top N SQL (vazio = mantém)", "Top N SQL (empty = keep)"), ""),
		}, func(_ Row, x Vals) (Script, error) {
			iv, err := numOrEmpty("INTERVAL", x["INTERVAL"])
			if err != nil {
				return Script{}, err
			}
			ret := ""
			if r := strings.TrimSpace(x["RETENTION"]); r != "" {
				d, err := Num("RETENTION", r)
				if err != nil {
					return Script{}, err
				}
				ret = d + " * 1440"
			}
			tn, err := numOrEmpty("TOPN", x["TOPN"])
			if err != nil {
				return Script{}, err
			}
			args := named("retention", ret, "interval", iv, "topnsql", tn)
			if args == "" {
				return Script{}, errors.New(L("nada para alterar", "nothing to change"))
			}
			return sqlS("%s", plsql("DBMS_WORKLOAD_REPOSITORY.MODIFY_SNAPSHOT_SETTINGS("+args+");"))
		}),
	)
	awrCfg.Pack = "diag"

	findings := v("pf.findings", L("Achados do ADDM", "ADDM findings"), `select f.finding_id, f.type, f.impact, f.message, f.more_info
	   from dba_advisor_findings f where f.task_name = :1 order by f.impact desc nulls last`)
	addm := v("pf.addm", L("Tarefas do ADDM", "ADDM tasks"), top(300, `select t.task_name, t.owner, t.status, t.created, t.execution_end, t.description,
	        (select count(*) from dba_advisor_findings f where f.task_name = t.task_name and f.owner = t.owner) findings
	   from dba_advisor_tasks t where t.advisor_name = 'ADDM' order by t.created desc`),
		rep('r', L("Relatório", "Report"), true, nil, func(r Row, _ Vals) (Script, error) {
			return reportQ("select dbms_addm.get_report(%s) from dual", Lit(r["TASK_NAME"]))
		}),
		act('n', L("Rodar ADDM (banco)", "Run ADDM (database)"), false, []Field{fR("BEGIN", L("Snapshot inicial", "Begin snapshot"), ""), fR("END", L("Snapshot final", "End snapshot"), "")},
			func(_ Row, x Vals) (Script, error) {
				b, e, err := snapRange(x)
				if err != nil {
					return Script{}, err
				}
				t := fmt.Sprintf("OBS_ADDM_%s_%s", b, e)
				return Script{Kind: Report, Text: fmt.Sprintf("DECLARE\n  t VARCHAR2(100) := %s;\nBEGIN\n  BEGIN DBMS_ADDM.DELETE(t); EXCEPTION WHEN OTHERS THEN NULL; END;\n  DBMS_ADDM.ANALYZE_DB(t, %s, %s);\nEND;\n/\nselect dbms_addm.get_report(%s) from dual", Lit(t), b, e, Lit(t))}, nil
			}),
		act('p', L("Parâmetro padrão do ADDM", "ADDM default parameter"), false, []Field{
			fC("PARAM", L("Parâmetro", "Parameter"), "DBIO_EXPECTED", "DBIO_EXPECTED", "DB_ELAPSED_TIME", "HISTORY_TABLE"), fR("VALUE", L("Valor", "Value"), "10000"),
		}, func(_ Row, x Vals) (Script, error) {
			val := strings.TrimSpace(x["VALUE"])
			if _, err := Num("VALUE", val); err != nil {
				val = Lit(val)
			}
			return sqlS("%s", plsql(fmt.Sprintf("DBMS_ADVISOR.SET_DEFAULT_TASK_PARAMETER('ADDM', %s, %s);", Lit(x["PARAM"]), val)))
		}),
		danger(act('x', L("Remover tarefa", "Delete task"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_ADDM.DELETE("+Lit(r["TASK_NAME"])+");"))
		}), "TASK_NAME"),
		drill('v', L("Achados", "Findings"), findings, one("TASK_NAME")),
	)
	addm.Pack = "diag"

	ashMin := []Field{fR("MIN", L("Últimos minutos", "Last minutes"), "30")}
	ashRep := rep('r', L("Relatório ASH", "ASH report"), false, ashMin, func(_ Row, x Vals) (Script, error) {
		n, err := Num("MIN", x["MIN"])
		if err != nil {
			return Script{}, err
		}
		return reportQ("select output from table(dbms_workload_repository.ash_report_text(%s, sysdate - %s/1440, sysdate))", dbidInst, n)
	})
	ashEv := v("pf.ash.events", L("ASH: eventos (30 min)", "ASH: events (30 min)"), `select nvl(event, 'ON CPU') event, nvl(wait_class, 'CPU') wait_class, count(*) samples,
	        round(ratio_to_report(count(*)) over () * 100, 1) pct
	   from v$active_session_history where sample_time > systimestamp - interval '30' minute
	  group by event, wait_class order by 3 desc`, ashRep)
	ashEv.Pack = "diag"
	ashSQL := v("pf.ash.sql", L("ASH: SQL (30 min)", "ASH: SQL (30 min)"), top(50, `select sql_id, count(*) samples, round(ratio_to_report(count(*)) over () * 100, 1) pct,
	        sum(case when session_state = 'ON CPU' then 1 else 0 end) cpu,
	        max(sql_opname) op, count(distinct session_id) sessions
	   from v$active_session_history where sample_time > systimestamp - interval '30' minute and sql_id is not null
	  group by sql_id order by 2 desc`), ashRep,
		rep('p', L("Plano (cursor)", "Plan (cursor)"), true, nil, func(r Row, _ Vals) (Script, error) {
			return reportQ("select plan_table_output from table(dbms_xplan.display_cursor(%s, null, 'ALL'))", Lit(r["SQL_ID"]))
		}))
	ashSQL.Pack = "diag"

	stSnaps := v("pf.statspack", "Statspack", top(300, `select snap_id, snap_time, snap_level, instance_number inst from perfstat.stats$snapshot order by snap_id desc`),
		act('n', L("Tirar snapshot", "Take snapshot"), false, []Field{fC("LEVEL", L("Nível", "Level"), "7", "5", "6", "7", "10")},
			func(_ Row, x Vals) (Script, error) {
				return sqlS("%s", plsql("PERFSTAT.STATSPACK.SNAP(i_snap_level => "+x["LEVEL"]+");"))
			}),
		danger(act('x', L("Apagar intervalo", "Purge range"), true, []Field{fR("LOW", L("De", "From"), "{SNAP_ID}"), fR("HIGH", L("Até", "To"), "{SNAP_ID}")},
			func(_ Row, x Vals) (Script, error) {
				a, err := Num("LOW", x["LOW"])
				if err != nil {
					return Script{}, err
				}
				b, err := Num("HIGH", x["HIGH"])
				if err != nil {
					return Script{}, err
				}
				return sqlS("%s", plsql(fmt.Sprintf("PERFSTAT.STATSPACK.PURGE(i_begin_snap => %s, i_end_snap => %s);", a, b)))
			}), "SNAP_ID"),
	)
	stSnaps.Help = L("Alternativa sem licença ao AWR. Instale com @?/rdbms/admin/spcreate.sql; o relatório sai com @?/rdbms/admin/spreport.sql.",
		"License-free alternative to AWR. Install with @?/rdbms/admin/spcreate.sql; the report comes from @?/rdbms/admin/spreport.sql.")
	stSnaps.Requires = "PERFSTAT.STATS$SNAPSHOT"
	stSnaps.Missing = L("Statspack não instalado: como SYSDBA, rode @?/rdbms/admin/spcreate.sql (cria o usuário PERFSTAT).",
		"Statspack not installed: as SYSDBA, run @?/rdbms/admin/spcreate.sql (creates the PERFSTAT user).")

	sqlmon := v("pf.sqlmon", "SQL Monitor", top(200, `select sql_id, status, username, sql_exec_start, round(elapsed_time/1e6, 1) ela_s,
	        round(cpu_time/1e6, 1) cpu_s, buffer_gets, disk_reads, px_servers_allocated px, substr(sql_text, 1, 120) sql_text, sql_exec_id
	   from v$sql_monitor order by sql_exec_start desc`),
		rep('r', L("Relatório", "Report"), true, nil, func(r Row, _ Vals) (Script, error) {
			return reportQ("select dbms_sqltune.report_sql_monitor(sql_id => %s, sql_exec_id => %s, type => 'TEXT') from dual", Lit(r["SQL_ID"]), r["SQL_EXEC_ID"])
		}),
	)
	sqlmon.Pack = "tuning"

	tasks := v("pf.sqltune", L("Tarefas do SQL Tuning Advisor", "SQL Tuning Advisor tasks"), top(200, `select task_name, owner, status, created, execution_end, description
	   from dba_advisor_tasks where advisor_name = 'SQL Tuning Advisor' order by created desc`),
		rep('r', L("Relatório", "Report"), true, nil, func(r Row, _ Vals) (Script, error) {
			return reportQ("select dbms_sqltune.report_tuning_task(%s) from dual", Lit(r["TASK_NAME"]))
		}),
		danger(act('x', L("Remover", "Drop"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_SQLTUNE.DROP_TUNING_TASK("+Lit(r["TASK_NAME"])+");"))
		}), "TASK_NAME"),
	)
	tasks.Pack = "tuning"

	spm := v("pf.spm", "SQL Plan Baselines (SPM)", top(500, `select sql_handle, plan_name, enabled, accepted, fixed, reproduced, origin, created,
	        last_executed, substr(sql_text, 1, 120) sql_text
	   from dba_sql_plan_baselines order by created desc`),
		act('e', L("Habilitar / desabilitar", "Enable / disable"), true, []Field{fC("V", "ENABLED", "NO", "YES", "NO")}, func(r Row, x Vals) (Script, error) {
			return sqlS("DECLARE\n  n PLS_INTEGER;\nBEGIN\n  n := DBMS_SPM.ALTER_SQL_PLAN_BASELINE(sql_handle => %s, plan_name => %s, attribute_name => 'ENABLED', attribute_value => %s);\nEND;", Lit(r["SQL_HANDLE"]), Lit(r["PLAN_NAME"]), Lit(x["V"]))
		}),
		act('f', L("Fixar / soltar", "Fix / unfix"), true, []Field{fC("V", "FIXED", "YES", "YES", "NO")}, func(r Row, x Vals) (Script, error) {
			return sqlS("DECLARE\n  n PLS_INTEGER;\nBEGIN\n  n := DBMS_SPM.ALTER_SQL_PLAN_BASELINE(sql_handle => %s, plan_name => %s, attribute_name => 'FIXED', attribute_value => %s);\nEND;", Lit(r["SQL_HANDLE"]), Lit(r["PLAN_NAME"]), Lit(x["V"]))
		}),
		rep('p', L("Plano da baseline", "Baseline plan"), true, nil, func(r Row, _ Vals) (Script, error) {
			return reportQ("select plan_table_output from table(dbms_xplan.display_sql_plan_baseline(sql_handle => %s, plan_name => %s))", Lit(r["SQL_HANDLE"]), Lit(r["PLAN_NAME"]))
		}),
		act('v', L("Evoluir (verificar planos novos)", "Evolve (verify new plans)"), true, nil, func(r Row, _ Vals) (Script, error) {
			return Script{Kind: Report, Text: fmt.Sprintf("select dbms_spm.evolve_sql_plan_baseline(sql_handle => %s) from dual", Lit(r["SQL_HANDLE"]))}, nil
		}),
		danger(act('x', L("Remover", "Drop"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("DECLARE\n  n PLS_INTEGER;\nBEGIN\n  n := DBMS_SPM.DROP_SQL_PLAN_BASELINE(sql_handle => %s, plan_name => %s);\nEND;", Lit(r["SQL_HANDLE"]), Lit(r["PLAN_NAME"]))
		}), "PLAN_NAME"),
	)

	statsJobs := v("pf.stats", L("Estatísticas: tabelas desatualizadas", "Statistics: stale tables"), top(500, `select owner, table_name, partition_name, object_type, num_rows, last_analyzed, stale_stats, stattype_locked
	   from dba_tab_statistics where (stale_stats = 'YES' or last_analyzed is null) and owner not in
	        (select username from dba_users where oracle_maintained = 'Y')
	  order by owner, table_name`),
		act('g', L("Coletar", "Gather"), true, []Field{fT("PCT", L("Amostra (vazio = AUTO)", "Sample (empty = AUTO)"), ""), fB("CASCADE", L("Índices junto", "Indexes too"), true)},
			func(r Row, x Vals) (Script, error) {
				est := "DBMS_STATS.AUTO_SAMPLE_SIZE"
				if p := strings.TrimSpace(x["PCT"]); p != "" {
					n, err := Num("PCT", p)
					if err != nil {
						return Script{}, err
					}
					est = n
				}
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_STATS.GATHER_TABLE_STATS(ownname => %s, tabname => %s, estimate_percent => %s, cascade => %s);",
					Lit(r["OWNER"]), Lit(r["TABLE_NAME"]), est, iff(Yes(x["CASCADE"]), "TRUE", "FALSE"))))
			}),
		act('s', L("Coletar o schema inteiro", "Gather the whole schema"), true, []Field{fC("OPT", L("Opções", "Options"), "GATHER AUTO", "GATHER AUTO", "GATHER STALE", "GATHER EMPTY", "GATHER")},
			func(r Row, x Vals) (Script, error) {
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_STATS.GATHER_SCHEMA_STATS(ownname => %s, options => %s);", Lit(r["OWNER"]), Lit(x["OPT"]))))
			}),
		act('l', L("Travar / destravar estatísticas", "Lock / unlock statistics"), true, []Field{fC("OP", "", "LOCK", "LOCK", "UNLOCK")},
			func(r Row, x Vals) (Script, error) {
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_STATS.%s_TABLE_STATS(ownname => %s, tabname => %s);", x["OP"], Lit(r["OWNER"]), Lit(r["TABLE_NAME"]))))
			}),
		act('d', L("Coletar estatísticas do dicionário e fixas", "Gather dictionary and fixed stats"), false, nil, func(Row, Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_STATS.GATHER_DICTIONARY_STATS;\n  DBMS_STATS.GATHER_FIXED_OBJECTS_STATS;"))
		}),
	)

	return &Node{Title: "Performance", Children: []*Node{
		{Title: L("Snapshots AWR", "AWR snapshots"), View: snaps},
		{Title: "Baselines", View: baselines},
		{Title: L("Templates de baseline", "Baseline templates"), View: templates},
		{Title: L("Configuração do AWR", "AWR settings"), View: awrCfg},
		{Title: "ADDM", View: addm},
		{Title: "ASH", Children: []*Node{
			{Title: L("Eventos", "Events"), View: ashEv},
			{Title: "SQL", View: ashSQL},
		}},
		{Title: "Statspack", View: stSnaps},
		{Title: "SQL Monitor", View: sqlmon},
		{Title: "SQL Tuning Advisor", View: tasks},
		{Title: "SQL Plan Baselines", View: spm},
		{Title: L("Estatísticas do otimizador", "Optimizer statistics"), View: statsJobs},
	}}
}
