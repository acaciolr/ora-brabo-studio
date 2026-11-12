// Aba DBA · Resource Manager (planos, diretivas, grupos, mapeamentos, CDB) e
// Scheduler (jobs, log, programs, schedules, janelas, chains, credenciais,
// DBMS_JOB e autotasks). by • DBA BRABO
package catalog

import (
	"errors"
	"fmt"
	"strings"
)

var mapAttrs = []string{"ORACLE_USER", "SERVICE_NAME", "CLIENT_OS_USER", "CLIENT_PROGRAM", "CLIENT_MACHINE",
	"MODULE_NAME", "MODULE_NAME_ACTION", "SERVICE_MODULE", "SERVICE_MODULE_ACTION", "CLIENT_ID"}

func directiveArgs(x Vals) (string, error) {
	var parts []string
	for _, k := range []struct{ f, p string }{{"MGMT_P1", "mgmt_p1"}, {"UTIL", "utilization_limit"}, {"PX", "parallel_degree_limit_p1"},
		{"ACTIVE", "active_sess_pool_p1"}, {"SWITCH_TIME", "switch_time"}} {
		if s := strings.TrimSpace(x[k.f]); s != "" {
			n, err := Num(k.f, s)
			if err != nil {
				return "", err
			}
			parts = append(parts, k.p+" => "+n)
		}
	}
	if g := strings.TrimSpace(x["SWITCH_GROUP"]); g != "" {
		parts = append(parts, "switch_group => "+Lit(strings.ToUpper(g)))
	}
	return strings.Join(parts, ", "), nil
}

func directiveFields() []Field {
	return []Field{
		fT("MGMT_P1", L("CPU nível 1 (%)", "CPU level 1 (%)"), ""), fT("UTIL", L("Limite de uso de CPU (%)", "CPU utilization limit (%)"), ""),
		fT("PX", L("Grau paralelo máximo", "Max parallel degree"), ""), fT("ACTIVE", L("Sessões ativas máximas", "Max active sessions"), ""),
		fT("SWITCH_GROUP", L("Trocar para o grupo (ex.: CANCEL_SQL)", "Switch to group (e.g. CANCEL_SQL)"), ""),
		fT("SWITCH_TIME", L("Depois de (segundos)", "After (seconds)"), ""),
	}
}

func rsrcNode() *Node {
	dirs := v("rm.directives", L("Diretivas do plano", "Plan directives"), `select plan, group_or_subplan, type, mgmt_p1, mgmt_p2, utilization_limit,
	        parallel_degree_limit_p1 px_limit, active_sess_pool_p1 active_max, switch_group, switch_time, comments
	   from dba_rsrc_plan_directives where plan = nvl(:1, plan) order by plan, mgmt_p1 desc nulls last`,
		act('n', L("Adicionar diretiva", "Add directive"), false, append([]Field{
			fR("PLAN", L("Plano", "Plan"), "{PLAN}"), fR("GROUP", L("Grupo ou subplano", "Group or subplan"), ""), fT("COMMENT", L("Comentário", "Comment"), ""),
		}, directiveFields()...), func(_ Row, x Vals) (Script, error) {
			a, err := directiveArgs(x)
			if err != nil {
				return Script{}, err
			}
			call := fmt.Sprintf("DBMS_RESOURCE_MANAGER.CREATE_PLAN_DIRECTIVE(plan => %s, group_or_subplan => %s, comment => %s%s);",
				Lit(strings.ToUpper(x["PLAN"])), Lit(strings.ToUpper(x["GROUP"])), Lit(x["COMMENT"]), iff(a != "", ", "+a, ""))
			return sqlS("%s", pending(call))
		}),
		act('e', L("Alterar diretiva", "Change directive"), true, directiveFields(), func(r Row, x Vals) (Script, error) {
			a, err := directiveArgs(x)
			if err != nil {
				return Script{}, err
			}
			if a == "" {
				return Script{}, errors.New(L("nada para alterar", "nothing to change"))
			}
			a = strings.NewReplacer("mgmt_p1 =>", "new_mgmt_p1 =>", "utilization_limit =>", "new_utilization_limit =>",
				"parallel_degree_limit_p1 =>", "new_parallel_degree_limit_p1 =>", "active_sess_pool_p1 =>", "new_active_sess_pool_p1 =>",
				"switch_time =>", "new_switch_time =>", "switch_group =>", "new_switch_group =>").Replace(a)
			return sqlS("%s", pending(fmt.Sprintf("DBMS_RESOURCE_MANAGER.UPDATE_PLAN_DIRECTIVE(plan => %s, group_or_subplan => %s, %s);", Lit(r["PLAN"]), Lit(r["GROUP_OR_SUBPLAN"]), a)))
		}),
		danger(act('x', L("Remover diretiva", "Delete directive"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", pending(fmt.Sprintf("DBMS_RESOURCE_MANAGER.DELETE_PLAN_DIRECTIVE(plan => %s, group_or_subplan => %s);", Lit(r["PLAN"]), Lit(r["GROUP_OR_SUBPLAN"]))))
		}), "GROUP_OR_SUBPLAN"),
	)
	dirs.Args = nil1
	plans := v("rm.plans", L("Planos", "Plans"), `select plan, num_plan_directives directives, cpu_method, mgmt_method, status, mandatory, comments,
	        case when upper(plan) = (select upper(name) from v$rsrc_plan where is_top_plan = 'TRUE' and rownum = 1) then 'ACTIVE' end active
	   from dba_rsrc_plans order by plan`,
		act('a', L("Ativar", "Activate"), true, []Field{fB("FORCE", L("FORCE (janelas do scheduler não trocam)", "FORCE (scheduler windows won't switch it)"), false)},
			func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER SYSTEM SET resource_manager_plan = %s SCOPE = BOTH;", Lit(iff(Yes(x["FORCE"]), "FORCE:", "")+r["PLAN"]))
			}),
		act('d', L("Desativar (nenhum plano)", "Deactivate (no plan)"), false, nil, func(Row, Vals) (Script, error) {
			return sqlS("ALTER SYSTEM SET resource_manager_plan = '' SCOPE = BOTH;")
		}),
		act('n', L("Criar plano", "Create plan"), false, []Field{fR("PLAN", L("Nome", "Name"), ""), fT("COMMENT", L("Comentário", "Comment"), ""),
			fR("OTHER", L("CPU de OTHER_GROUPS (%)", "OTHER_GROUPS CPU (%)"), "100")},
			func(_ Row, x Vals) (Script, error) {
				o, err := Num("OTHER", x["OTHER"])
				if err != nil {
					return Script{}, err
				}
				p := Lit(strings.ToUpper(x["PLAN"]))
				return sqlS("%s", pending(fmt.Sprintf("DBMS_RESOURCE_MANAGER.CREATE_PLAN(plan => %s, comment => %s);\n  DBMS_RESOURCE_MANAGER.CREATE_PLAN_DIRECTIVE(plan => %s, group_or_subplan => 'OTHER_GROUPS', comment => 'demais sessoes', mgmt_p1 => %s);",
					p, Lit(x["COMMENT"]), p, o)))
			}),
		danger(act('x', L("Remover plano", "Delete plan"), true, []Field{fB("CASCADE", L("Com grupos e subplanos", "With groups and subplans"), false)},
			func(r Row, x Vals) (Script, error) {
				if Yes(x["CASCADE"]) {
					return sqlS("%s", pending("DBMS_RESOURCE_MANAGER.DELETE_PLAN_CASCADE(plan => "+Lit(r["PLAN"])+");"))
				}
				return sqlS("%s", pending("DBMS_RESOURCE_MANAGER.DELETE_PLAN(plan => "+Lit(r["PLAN"])+");"))
			}), "PLAN"),
		drill('v', L("Diretivas", "Directives"), dirs, one("PLAN")),
	)
	plans.KeyCol = "PLAN"
	groups := v("rm.groups", L("Grupos de consumidores", "Consumer groups"), `select consumer_group, cpu_method, mgmt_method, status, mandatory, comments from dba_rsrc_consumer_groups order by 1`,
		act('n', L("Criar grupo", "Create group"), false, []Field{fR("GROUP", L("Nome", "Name"), ""), fT("COMMENT", L("Comentário", "Comment"), "")},
			func(_ Row, x Vals) (Script, error) {
				return sqlS("%s", pending(fmt.Sprintf("DBMS_RESOURCE_MANAGER.CREATE_CONSUMER_GROUP(consumer_group => %s, comment => %s);", Lit(strings.ToUpper(x["GROUP"])), Lit(x["COMMENT"]))))
			}),
		act('g', L("Permitir troca para o grupo", "Grant switch to group"), true, []Field{fR("TO", L("Usuário/role", "User/role"), "")},
			func(r Row, x Vals) (Script, error) {
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_RESOURCE_MANAGER_PRIVS.GRANT_SWITCH_CONSUMER_GROUP(grantee_name => %s, consumer_group => %s, grant_option => FALSE);", Lit(strings.ToUpper(x["TO"])), Lit(r["CONSUMER_GROUP"]))))
			}),
		danger(act('x', L("Remover grupo", "Delete group"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", pending("DBMS_RESOURCE_MANAGER.DELETE_CONSUMER_GROUP(consumer_group => "+Lit(r["CONSUMER_GROUP"])+");"))
		}), "CONSUMER_GROUP"),
	)
	mappings := v("rm.mappings", L("Mapeamentos", "Mappings"), `select attribute, value, consumer_group, status from dba_rsrc_group_mappings order by 1, 2`,
		act('n', L("Mapear", "Map"), false, []Field{
			fC("ATTR", L("Atributo", "Attribute"), "ORACLE_USER", mapAttrs...), fR("VALUE", L("Valor", "Value"), ""), fR("GROUP", L("Grupo", "Group"), ""),
		}, func(_ Row, x Vals) (Script, error) {
			return sqlS("%s", pending(fmt.Sprintf("DBMS_RESOURCE_MANAGER.SET_CONSUMER_GROUP_MAPPING(attribute => %s, value => %s, consumer_group => %s);",
				Lit(x["ATTR"]), Lit(strings.ToUpper(x["VALUE"])), Lit(strings.ToUpper(x["GROUP"])))))
		}),
		danger(act('x', L("Remover mapeamento", "Remove mapping"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", pending(fmt.Sprintf("DBMS_RESOURCE_MANAGER.SET_CONSUMER_GROUP_MAPPING(attribute => %s, value => %s, consumer_group => NULL);", Lit(r["ATTRIBUTE"]), Lit(r["VALUE"]))))
		}), "VALUE"),
	)
	prioF := []Field{}
	prioDefault := map[string]string{"EXPLICIT": "1", "SERVICE_MODULE_ACTION": "2", "SERVICE_MODULE": "3", "MODULE_NAME_ACTION": "4", "MODULE_NAME": "5",
		"SERVICE_NAME": "6", "ORACLE_USER": "7", "CLIENT_PROGRAM": "8", "CLIENT_OS_USER": "9", "CLIENT_MACHINE": "10", "CLIENT_ID": "11"}
	prioOrder := []string{"EXPLICIT", "SERVICE_MODULE_ACTION", "SERVICE_MODULE", "MODULE_NAME_ACTION", "MODULE_NAME", "SERVICE_NAME", "ORACLE_USER", "CLIENT_PROGRAM", "CLIENT_OS_USER", "CLIENT_MACHINE", "CLIENT_ID"}
	for _, k := range prioOrder {
		prioF = append(prioF, fR(k, k, prioDefault[k]))
	}
	prio := v("rm.priorities", L("Prioridades dos mapeamentos", "Mapping priorities"), `select attribute, priority, status from dba_rsrc_mapping_priority order by priority`,
		act('e', L("Alterar prioridades", "Change priorities"), false, prioF, func(_ Row, x Vals) (Script, error) {
			var a []string
			for _, k := range prioOrder {
				n, err := Num(k, x[k])
				if err != nil {
					return Script{}, err
				}
				a = append(a, strings.ToLower(k)+" => "+n)
			}
			return sqlS("%s", pending("DBMS_RESOURCE_MANAGER.SET_CONSUMER_GROUP_MAPPING_PRI(\n    "+strings.Join(a, ",\n    ")+");"))
		}),
	)

	cdbDirs := v("rm.cdbdirs", L("Diretivas do plano CDB", "CDB plan directives"), `select plan, pluggable_database, directive_type, shares, utilization_limit, parallel_server_limit, comments
	   from dba_cdb_rsrc_plan_directives where plan = nvl(:1, plan) order by plan, pluggable_database`,
		act('n', L("Adicionar diretiva de PDB", "Add PDB directive"), false, []Field{
			fR("PLAN", L("Plano", "Plan"), "{PLAN}"), fR("PDB", "PDB", ""), fT("SHARES", "Shares", "1"),
			fT("UTIL", L("Limite de CPU (%)", "CPU limit (%)"), "100"), fT("PX", L("Limite de PX (%)", "PX limit (%)"), "100"),
		}, func(_ Row, x Vals) (Script, error) {
			var a []string
			for _, k := range []struct{ f, p string }{{"SHARES", "shares"}, {"UTIL", "utilization_limit"}, {"PX", "parallel_server_limit"}} {
				if s := strings.TrimSpace(x[k.f]); s != "" {
					n, err := Num(k.f, s)
					if err != nil {
						return Script{}, err
					}
					a = append(a, k.p+" => "+n)
				}
			}
			return sqlS("%s", pending(fmt.Sprintf("DBMS_RESOURCE_MANAGER.CREATE_CDB_PLAN_DIRECTIVE(plan => %s, pluggable_database => %s, %s);",
				Lit(strings.ToUpper(x["PLAN"])), Lit(strings.ToUpper(x["PDB"])), strings.Join(a, ", "))))
		}),
		danger(act('x', L("Remover diretiva", "Delete directive"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", pending(fmt.Sprintf("DBMS_RESOURCE_MANAGER.DELETE_CDB_PLAN_DIRECTIVE(plan => %s, pluggable_database => %s);", Lit(r["PLAN"]), Lit(r["PLUGGABLE_DATABASE"]))))
		}), "PLUGGABLE_DATABASE"),
	)
	cdbDirs.Args = nil1
	cdbDirs.Needs = "cdb"
	cdbPlans := v("rm.cdbplans", L("Planos do CDB", "CDB plans"), `select plan, status, mandatory, comments from dba_cdb_rsrc_plans order by plan`,
		act('n', L("Criar plano CDB", "Create CDB plan"), false, []Field{fR("PLAN", L("Nome", "Name"), ""), fT("COMMENT", L("Comentário", "Comment"), "")},
			func(_ Row, x Vals) (Script, error) {
				return sqlS("%s", pending(fmt.Sprintf("DBMS_RESOURCE_MANAGER.CREATE_CDB_PLAN(plan => %s, comment => %s);", Lit(strings.ToUpper(x["PLAN"])), Lit(x["COMMENT"]))))
			}),
		act('a', L("Ativar no CDB$ROOT", "Activate in CDB$ROOT"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER SYSTEM SET resource_manager_plan = %s SCOPE = BOTH;", Lit(r["PLAN"]))
		}),
		danger(act('x', L("Remover plano CDB", "Delete CDB plan"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", pending("DBMS_RESOURCE_MANAGER.DELETE_CDB_PLAN(plan => "+Lit(r["PLAN"])+");"))
		}), "PLAN"),
		drill('v', L("Diretivas", "Directives"), cdbDirs, one("PLAN")),
	)
	cdbPlans.Needs = "cdb"

	return &Node{Title: "Resource Manager", Children: []*Node{
		{Title: L("Plano em uso e consumo", "Active plan and usage"), View: v("rm.current", L("Plano em uso", "Active plan"),
			`select g.name consumer_group, g.active_sessions, g.execution_waiters, g.requests, round(g.cpu_wait_time/1000) cpu_wait_s,
			        round(g.consumed_cpu_time/1000) cpu_s, (select max(name) from v$rsrc_plan where is_top_plan = 'TRUE') plan
			   from v$rsrc_consumer_group g order by g.consumed_cpu_time desc`)},
		{Title: L("Planos", "Plans"), View: plans},
		{Title: L("Diretivas", "Directives"), View: dirs},
		{Title: L("Grupos de consumidores", "Consumer groups"), View: groups},
		{Title: L("Mapeamentos", "Mappings"), View: mappings},
		{Title: L("Prioridades dos mapeamentos", "Mapping priorities"), View: prio},
		{Title: L("Planos do CDB", "CDB plans"), View: cdbPlans},
	}}
}

func schedName(r Row, nameCol string) string { return Lit(Q2(r["OWNER"], r[nameCol])) }

func schedNode() *Node {
	runs := v("sc.runs", L("Execuções (log)", "Runs (log)"), top(500, `select log_id, log_date, owner, job_name, status, error#, run_duration,
	        actual_start_date, additional_info, errors, output
	   from dba_scheduler_job_run_details where owner = nvl(:1, owner) and job_name = nvl(:2, job_name)
	  order by log_date desc`),
		act('p', L("Limpar o log", "Purge the log"), false, []Field{fT("DAYS", L("Manter últimos (dias; vazio = tudo)", "Keep last (days; empty = all)"), "30")},
			func(_ Row, x Vals) (Script, error) {
				d, err := numOrEmpty("DAYS", x["DAYS"])
				if err != nil {
					return Script{}, err
				}
				if d == "" {
					return sqlS("%s", plsql("DBMS_SCHEDULER.PURGE_LOG;"))
				}
				return sqlS("%s", plsql("DBMS_SCHEDULER.PURGE_LOG(log_history => "+d+");"))
			}),
	)
	runs.Args = nil2
	jobs := v("sc.jobs", "Jobs", `select owner, job_name, job_type, enabled, state, schedule_type, repeat_interval, last_start_date,
	        next_run_date, run_count, failure_count, job_class, comments
	   from dba_scheduler_jobs order by owner, job_name`,
		act('r', L("Executar agora", "Run now"), true, []Field{fB("CURRENT", L("Na sessão atual (espera terminar)", "In current session (waits)"), false)},
			func(r Row, x Vals) (Script, error) {
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_SCHEDULER.RUN_JOB(%s, use_current_session => %s);", schedName(r, "JOB_NAME"), iff(Yes(x["CURRENT"]), "TRUE", "FALSE"))))
			}),
		act('e', L("Habilitar", "Enable"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_SCHEDULER.ENABLE("+schedName(r, "JOB_NAME")+");"))
		}),
		act('d', L("Desabilitar", "Disable"), true, []Field{fB("FORCE", "FORCE", false)}, func(r Row, x Vals) (Script, error) {
			return sqlS("%s", plsql(fmt.Sprintf("DBMS_SCHEDULER.DISABLE(%s, force => %s);", schedName(r, "JOB_NAME"), iff(Yes(x["FORCE"]), "TRUE", "FALSE"))))
		}),
		act('s', L("Parar", "Stop"), true, []Field{fB("FORCE", "FORCE", false)}, func(r Row, x Vals) (Script, error) {
			return sqlS("%s", plsql(fmt.Sprintf("DBMS_SCHEDULER.STOP_JOB(%s, force => %s);", schedName(r, "JOB_NAME"), iff(Yes(x["FORCE"]), "TRUE", "FALSE"))))
		}),
		act('i', L("Alterar a repetição", "Change repeat interval"), true, []Field{fR("RI", "repeat_interval", "{REPEAT_INTERVAL}")},
			func(r Row, x Vals) (Script, error) {
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_SCHEDULER.SET_ATTRIBUTE(%s, 'repeat_interval', %s);", schedName(r, "JOB_NAME"), Lit(x["RI"]))))
			}),
		act('n', L("Criar job", "Create job"), false, []Field{
			fR("NAME", L("Nome (OWNER.NOME ou NOME)", "Name (OWNER.NAME or NAME)"), ""),
			fC("TYPE", L("Tipo", "Type"), "PLSQL_BLOCK", "PLSQL_BLOCK", "STORED_PROCEDURE", "EXECUTABLE"),
			fM("ACTION", L("Ação", "Action"), "BEGIN\n  NULL;\nEND;"),
			fT("RI", L("Repetição (vazio = uma vez)", "Repeat (empty = once)"), "FREQ=DAILY; BYHOUR=2; BYMINUTE=0"),
			fB("ENABLED", L("Habilitado", "Enabled"), true), fT("COMMENT", L("Comentário", "Comment"), ""),
		}, func(_ Row, x Vals) (Script, error) {
			return sqlS("%s", plsql(fmt.Sprintf("DBMS_SCHEDULER.CREATE_JOB(%s);", named(
				"job_name", Lit(strings.ToUpper(strings.TrimSpace(x["NAME"]))), "job_type", Lit(x["TYPE"]),
				"job_action", Lit(x["ACTION"]), "start_date", "SYSTIMESTAMP",
				"repeat_interval", litOrEmpty(x["RI"]), "enabled", iff(Yes(x["ENABLED"]), "TRUE", "FALSE"),
				"comments", litOrEmpty(x["COMMENT"])))))
		}),
		danger(act('x', L("Remover", "Drop"), true, []Field{fB("FORCE", "FORCE", false)}, func(r Row, x Vals) (Script, error) {
			return sqlS("%s", plsql(fmt.Sprintf("DBMS_SCHEDULER.DROP_JOB(%s, force => %s);", schedName(r, "JOB_NAME"), iff(Yes(x["FORCE"]), "TRUE", "FALSE"))))
		}), "JOB_NAME"),
		rep('D', "DDL", true, nil, func(r Row, _ Vals) (Script, error) {
			return reportQ("select dbms_metadata.get_ddl('PROCOBJ', %s, %s) from dual", Lit(r["JOB_NAME"]), Lit(r["OWNER"]))
		}),
		drill('l', L("Execuções", "Runs"), runs, two("OWNER", "JOB_NAME")),
	)
	jobs.KeyCol = "JOB_NAME"

	enDis := func(col string) []*Action {
		return []*Action{
			act('e', L("Habilitar", "Enable"), true, nil, func(r Row, _ Vals) (Script, error) {
				return sqlS("%s", plsql("DBMS_SCHEDULER.ENABLE("+schedName(r, col)+");"))
			}),
			act('d', L("Desabilitar", "Disable"), true, nil, func(r Row, _ Vals) (Script, error) {
				return sqlS("%s", plsql("DBMS_SCHEDULER.DISABLE("+schedName(r, col)+", force => TRUE);"))
			}),
		}
	}
	programs := v("sc.programs", "Programs", `select owner, program_name, program_type, program_action, number_of_arguments args, enabled, comments
	   from dba_scheduler_programs order by owner, program_name`,
		append(enDis("PROGRAM_NAME"), danger(act('x', L("Remover", "Drop"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_SCHEDULER.DROP_PROGRAM("+schedName(r, "PROGRAM_NAME")+", force => TRUE);"))
		}), "PROGRAM_NAME"))...)
	schedules := v("sc.schedules", "Schedules", `select owner, schedule_name, schedule_type, start_date, repeat_interval, end_date, comments
	   from dba_scheduler_schedules order by owner, schedule_name`,
		act('n', L("Criar", "Create"), false, []Field{fR("NAME", L("Nome", "Name"), ""), fR("RI", "repeat_interval", "FREQ=DAILY; BYHOUR=2"), fT("COMMENT", L("Comentário", "Comment"), "")},
			func(_ Row, x Vals) (Script, error) {
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_SCHEDULER.CREATE_SCHEDULE(schedule_name => %s, start_date => SYSTIMESTAMP, repeat_interval => %s%s);",
					Lit(strings.ToUpper(x["NAME"])), Lit(x["RI"]), iff(strings.TrimSpace(x["COMMENT"]) != "", ", comments => "+Lit(x["COMMENT"]), ""))))
			}),
		danger(act('x', L("Remover", "Drop"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_SCHEDULER.DROP_SCHEDULE("+schedName(r, "SCHEDULE_NAME")+", force => TRUE);"))
		}), "SCHEDULE_NAME"),
	)
	winName := func(r Row) string { return Lit("SYS." + r["WINDOW_NAME"]) }
	windows := v("sc.windows", L("Janelas", "Windows"), `select window_name, resource_plan, schedule_type, repeat_interval, duration, enabled, active, next_start_date
	   from dba_scheduler_windows order by window_name`,
		act('o', L("Abrir agora", "Open now"), true, []Field{fT("MIN", L("Duração (minutos; vazio = a da janela)", "Duration (minutes; empty = window's)"), "")},
			func(r Row, x Vals) (Script, error) {
				dur := "NULL"
				if m := strings.TrimSpace(x["MIN"]); m != "" {
					n, err := Num("MIN", m)
					if err != nil {
						return Script{}, err
					}
					dur = "NUMTODSINTERVAL(" + n + ", 'MINUTE')"
				}
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_SCHEDULER.OPEN_WINDOW(%s, duration => %s, force => TRUE);", winName(r), dur)))
			}),
		act('c', L("Fechar", "Close"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_SCHEDULER.CLOSE_WINDOW("+winName(r)+");"))
		}),
		act('e', L("Habilitar", "Enable"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_SCHEDULER.ENABLE("+winName(r)+");"))
		}),
		act('d', L("Desabilitar", "Disable"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_SCHEDULER.DISABLE("+winName(r)+", force => TRUE);"))
		}),
		act('p', L("Trocar o plano da janela", "Change window plan"), true, []Field{fT("PLAN", L("Plano (vazio = nenhum)", "Plan (empty = none)"), "{RESOURCE_PLAN}")},
			func(r Row, x Vals) (Script, error) {
				p := strings.TrimSpace(x["PLAN"])
				val := "NULL"
				if p != "" {
					val = Lit(strings.ToUpper(p))
				}
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_SCHEDULER.SET_ATTRIBUTE(%s, 'resource_plan', %s);", winName(r), val)))
			}),
	)
	chains := v("sc.chains", "Chains", `select owner, chain_name, rule_set_name, number_of_rules rules, number_of_steps steps, enabled, comments
	   from dba_scheduler_chains order by owner, chain_name`,
		append(enDis("CHAIN_NAME"), danger(act('x', L("Remover", "Drop"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_SCHEDULER.DROP_CHAIN("+schedName(r, "CHAIN_NAME")+", force => TRUE);"))
		}), "CHAIN_NAME"))...)
	creds := v("sc.credentials", L("Credenciais", "Credentials"), `select owner, credential_name, username, windows_domain, enabled, comments from dba_credentials order by owner, credential_name`,
		act('n', L("Criar", "Create"), false, []Field{fR("NAME", L("Nome", "Name"), ""), fR("USER", L("Usuário do SO", "OS user"), "oracle"), fP("PWD", L("Senha", "Password"))},
			func(_ Row, x Vals) (Script, error) {
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_CREDENTIAL.CREATE_CREDENTIAL(credential_name => %s, username => %s, password => %s);",
					Lit(strings.ToUpper(x["NAME"])), Lit(x["USER"]), Lit(x["PWD"]))))
			}),
		danger(act('x', L("Remover", "Drop"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_CREDENTIAL.DROP_CREDENTIAL("+schedName(r, "CREDENTIAL_NAME")+", force => TRUE);"))
		}), "CREDENTIAL_NAME"),
	)
	oldJobs := v("sc.dbmsjob", "DBMS_JOB", `select job, log_user, schema_user, last_date, next_date, broken, interval, failures, what from dba_jobs order by job`,
		act('b', L("Quebrar / consertar (broken)", "Break / fix (broken)"), true, []Field{fC("B", "BROKEN", "TRUE", "TRUE", "FALSE")},
			func(r Row, x Vals) (Script, error) {
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_JOB.BROKEN(%s, %s);\n  COMMIT;", r["JOB"], x["B"])))
			}),
		act('r', L("Executar agora", "Run now"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_JOB.RUN("+r["JOB"]+");"))
		}),
	)
	oldJobs.Help = L("DBMS_JOB só aceita BROKEN/RUN do dono do job (log_user).", "DBMS_JOB only accepts BROKEN/RUN from the job owner (log_user).")
	autotask := v("sc.autotask", L("Tarefas automáticas", "Automated tasks"), `select client_name, status, consumer_group, window_group, attributes from dba_autotask_client order by 1`,
		act('e', L("Habilitar", "Enable"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql(fmt.Sprintf("DBMS_AUTO_TASK_ADMIN.ENABLE(client_name => %s, operation => NULL, window_name => NULL);", Lit(r["CLIENT_NAME"]))))
		}),
		act('d', L("Desabilitar", "Disable"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql(fmt.Sprintf("DBMS_AUTO_TASK_ADMIN.DISABLE(client_name => %s, operation => NULL, window_name => NULL);", Lit(r["CLIENT_NAME"]))))
		}),
	)

	return &Node{Title: "Scheduler", Children: []*Node{
		{Title: "Jobs", View: jobs},
		{Title: L("Execuções", "Runs"), View: runs},
		{Title: "Programs", View: programs},
		{Title: "Schedules", View: schedules},
		{Title: L("Janelas", "Windows"), View: windows},
		{Title: "Chains", View: chains},
		{Title: L("Credenciais", "Credentials"), View: creds},
		{Title: "DBMS_JOB", View: oldJobs},
		{Title: L("Tarefas automáticas", "Automated tasks"), View: autotask},
	}}
}
