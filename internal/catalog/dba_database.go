// Aba DBA · Banco de dados: instância, parâmetros, banco, serviços, PDBs,
// componentes e uso de recursos (licenciamento). by • DBA BRABO
package catalog

import (
	"fmt"
	"strings"
)

func paramValue(r Row, val string) string {
	val = strings.TrimSpace(val)
	// TYPE 2 = texto: põe aspas se o usuário não pôs
	if r["TYPE"] == "2" && val != "" && !strings.ContainsAny(val, "'\"") {
		return Lit(val)
	}
	return val
}

func paramsView(id, title, where string) *View {
	sid := fT("SID", L("SID (RAC, vazio = todas: *)", "SID (RAC, empty = all: *)"), "")
	vw := v(id, title, `select name, display_value value, type, isdefault, ismodified,
	       issys_modifiable sys_mod, isses_modifiable ses_mod, description
	  from v$parameter `+where+` order by name`,
		act('e', L("Alterar (ALTER SYSTEM)", "Change (ALTER SYSTEM)"), true, []Field{
			fT("VALUE", L("Valor", "Value"), "{VALUE}"),
			fC("SCOPE", "SCOPE", "BOTH", "BOTH", "SPFILE", "MEMORY"),
			sid, fT("COMMENT", L("Comentário", "Comment"), ""),
		}, func(r Row, x Vals) (Script, error) {
			s := "ALTER SYSTEM SET " + r["NAME"] + " = " + paramValue(r, x["VALUE"])
			if c := strings.TrimSpace(x["COMMENT"]); c != "" {
				s += " COMMENT = " + Lit(c)
			}
			s += " SCOPE = " + x["SCOPE"]
			if t := strings.TrimSpace(x["SID"]); t != "" {
				s += " SID = " + Lit(t)
			}
			return sqlS("%s;", s)
		}),
		act('r', L("Voltar ao padrão (RESET)", "Reset to default"), true, []Field{
			fC("SCOPE", "SCOPE", "SPFILE", "SPFILE", "BOTH", "MEMORY"), sid,
		}, func(r Row, x Vals) (Script, error) {
			s := "ALTER SYSTEM RESET " + r["NAME"] + " SCOPE = " + x["SCOPE"]
			if t := strings.TrimSpace(x["SID"]); t != "" {
				s += " SID = " + Lit(t)
			}
			return sqlS("%s;", s)
		}),
		act('s', L("Alterar só na sessão", "Change in session"), true, []Field{fT("VALUE", L("Valor", "Value"), "{VALUE}")},
			func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER SESSION SET %s = %s;", r["NAME"], paramValue(r, x["VALUE"]))
			}),
		act('p', L("Criar PFILE a partir do SPFILE", "Create PFILE from SPFILE"), false, []Field{
			fT("FILE", L("Arquivo (vazio = padrão)", "File (empty = default)"), "/tmp/init_obs.ora"),
		}, func(r Row, x Vals) (Script, error) {
			if f := strings.TrimSpace(x["FILE"]); f != "" {
				return sqlS("CREATE PFILE = %s FROM SPFILE;", Lit(f))
			}
			return sqlS("CREATE PFILE FROM SPFILE;")
		}),
		act('m', L("Criar SPFILE a partir da memória", "Create SPFILE from memory"), false, []Field{
			fT("FILE", L("Arquivo (vazio = padrão)", "File (empty = default)"), ""),
		}, func(r Row, x Vals) (Script, error) {
			if f := strings.TrimSpace(x["FILE"]); f != "" {
				return sqlS("CREATE SPFILE = %s FROM MEMORY;", Lit(f))
			}
			return sqlS("CREATE SPFILE FROM MEMORY;")
		}),
	)
	vw.KeyCol = "NAME"
	return vw
}

func archivelogScript(on bool) (Script, error) {
	mode := "NOARCHIVELOG"
	if on {
		mode = "ARCHIVELOG"
	}
	return rmanS("SHUTDOWN IMMEDIATE;\nSTARTUP MOUNT;\nSQL 'ALTER DATABASE %s';\nALTER DATABASE OPEN;", mode)
}

func databaseNode() *Node {
	db := v("db.database", L("Banco", "Database"), `select name, dbid, db_unique_name, created, log_mode, open_mode,
	        database_role, protection_mode, force_logging, flashback_on, supplemental_log_data_min supl_log,
	        platform_name, cdb, current_scn, controlfile_type
	   from v$database`,
		danger(act('A', L("Ligar ARCHIVELOG (reinicia o banco, RMAN)", "Enable ARCHIVELOG (restarts the database, RMAN)"), false, nil,
			func(Row, Vals) (Script, error) { return archivelogScript(true) }), ""),
		danger(act('N', L("Desligar ARCHIVELOG (reinicia o banco, RMAN)", "Disable ARCHIVELOG (restarts the database, RMAN)"), false, nil,
			func(Row, Vals) (Script, error) { return archivelogScript(false) }), ""),
		act('f', L("Flashback Database ON/OFF", "Flashback Database ON/OFF"), false, []Field{fC("MODE", "", "ON", "ON", "OFF")},
			func(_ Row, x Vals) (Script, error) { return sqlS("ALTER DATABASE FLASHBACK %s;", x["MODE"]) }),
		act('l', L("Force logging ON/OFF", "Force logging ON/OFF"), false, []Field{fC("MODE", "", "ON", "ON", "OFF")},
			func(_ Row, x Vals) (Script, error) {
				return sqlS("ALTER DATABASE %sFORCE LOGGING;", iff(x["MODE"] == "OFF", "NO ", ""))
			}),
		act('g', L("Supplemental logging (mínimo)", "Supplemental logging (minimal)"), false, []Field{fC("MODE", "", "ADD", "ADD", "DROP")},
			func(_ Row, x Vals) (Script, error) {
				return sqlS("ALTER DATABASE %s SUPPLEMENTAL LOG DATA;", x["MODE"])
			}),
		act('d', L("Tablespace padrão", "Default tablespace"), false, []Field{
			fC("KIND", L("Tipo", "Kind"), "PERMANENT", "PERMANENT", "TEMPORARY"), fR("TS", "Tablespace", "USERS"),
		}, func(_ Row, x Vals) (Script, error) {
			return sqlS("ALTER DATABASE DEFAULT %sTABLESPACE %s;", iff(x["KIND"] == "TEMPORARY", "TEMPORARY ", ""), IdentIn(x["TS"]))
		}),
		act('n', L("Renomear global_name", "Rename global_name"), false, []Field{fR("NAME", L("Novo nome", "New name"), "")},
			func(_ Row, x Vals) (Script, error) {
				return sqlS("ALTER DATABASE RENAME GLOBAL_NAME TO %s;", strings.TrimSpace(x["NAME"]))
			}),
		danger(act('S', L("Parar / iniciar a instância (RMAN)", "Stop / start the instance (RMAN)"), false, []Field{
			fC("OP", L("Operação", "Operation"), "RESTART",
				"RESTART", "SHUTDOWN IMMEDIATE", "SHUTDOWN ABORT", "STARTUP", "STARTUP MOUNT", "STARTUP NOMOUNT"),
		}, func(_ Row, x Vals) (Script, error) {
			if x["OP"] == "RESTART" {
				return rmanS("SHUTDOWN IMMEDIATE;\nSTARTUP;")
			}
			return rmanS("%s;", x["OP"])
		}), ""),
	)
	db.Special = "record"

	inst := v("db.instance", L("Instância", "Instance"), `select inst_id, instance_number, instance_name, host_name, version,
	        startup_time, status, database_status, archiver, logins, shutdown_pending, instance_role, thread#
	   from gv$instance order by inst_id`,
		act('r', L("Restringir / liberar logins", "Restrict / allow logins"), false, []Field{fC("MODE", "", "ENABLE", "ENABLE", "DISABLE")},
			func(_ Row, x Vals) (Script, error) { return sqlS("ALTER SYSTEM %s RESTRICTED SESSION;", x["MODE"]) }),
		act('q', L("Quiesce / unquiesce", "Quiesce / unquiesce"), false, []Field{fC("MODE", "", "QUIESCE RESTRICTED", "QUIESCE RESTRICTED", "UNQUIESCE")},
			func(_ Row, x Vals) (Script, error) { return sqlS("ALTER SYSTEM %s;", x["MODE"]) }),
		act('s', L("Suspender / retomar I/O", "Suspend / resume I/O"), false, []Field{fC("MODE", "", "SUSPEND", "SUSPEND", "RESUME")},
			func(_ Row, x Vals) (Script, error) { return sqlS("ALTER SYSTEM %s;", x["MODE"]) }),
		act('f', L("Limpar shared pool / buffer cache", "Flush shared pool / buffer cache"), false, []Field{fC("WHAT", "", "SHARED_POOL", "SHARED_POOL", "BUFFER_CACHE")},
			func(_ Row, x Vals) (Script, error) { return sqlS("ALTER SYSTEM FLUSH %s;", x["WHAT"]) }),
	)

	svcs := v("db.services", L("Serviços", "Services"), `select s.name, s.network_name, s.creation_date, s.goal, s.clb_goal,
	        case when a.name is null then 'STOPPED' else 'RUNNING' end status
	   from dba_services s left join (select distinct name from gv$active_services) a on a.name = s.name
	  order by s.name`,
		act('n', L("Criar serviço", "Create service"), false, []Field{
			fR("NAME", L("Nome", "Name"), ""), fT("NET", L("Network name (vazio = igual)", "Network name (empty = same)"), ""),
			fB("START", L("Iniciar em seguida", "Start right away"), true),
		}, func(_ Row, x Vals) (Script, error) {
			n := strings.TrimSpace(x["NAME"])
			net := strings.TrimSpace(x["NET"])
			if net == "" {
				net = n
			}
			s := fmt.Sprintf("DBMS_SERVICE.CREATE_SERVICE(service_name => %s, network_name => %s);", Lit(n), Lit(net))
			if Yes(x["START"]) {
				s += fmt.Sprintf("\n  DBMS_SERVICE.START_SERVICE(%s);", Lit(n))
			}
			return sqlS("%s", plsql(s))
		}),
		act('s', L("Iniciar", "Start"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_SERVICE.START_SERVICE("+Lit(r["NAME"])+");"))
		}),
		act('p', L("Parar", "Stop"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_SERVICE.STOP_SERVICE("+Lit(r["NAME"])+");"))
		}),
		danger(act('x', L("Remover", "Delete"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("%s", plsql("DBMS_SERVICE.DELETE_SERVICE("+Lit(r["NAME"])+");"))
		}), "NAME"),
	)
	svcs.Help = L("Em RAC/Grid prefira srvctl: o DBMS_SERVICE não grava no OCR.", "On RAC/Grid prefer srvctl: DBMS_SERVICE does not update the OCR.")

	fnc := fT("FNC", L("FILE_NAME_CONVERT (ex.: '/pdbseed/','/nova/'; vazio = OMF)", "FILE_NAME_CONVERT (e.g. '/pdbseed/','/new/'; empty = OMF)"), "")
	fncClause := func(x Vals) string {
		if f := strings.TrimSpace(x["FNC"]); f != "" {
			return "\n  FILE_NAME_CONVERT = (" + f + ")"
		}
		return ""
	}
	pdbs := v("db.pdbs", "Pluggable databases", `select p.con_id, p.name, p.open_mode, p.restricted, p.open_time,
	        round(p.total_size/1048576) mb,
	        (select max(s.state) from dba_pdb_saved_states s where s.con_name = p.name) saved_state
	   from v$pdbs p order by p.con_id`,
		act('o', L("Abrir", "Open"), true, []Field{
			fC("MODE", L("Modo", "Mode"), "READ WRITE", "READ WRITE", "READ ONLY", "UPGRADE"),
			fB("RESTRICTED", "RESTRICTED", false), fB("ALL", L("Todas as instâncias (RAC)", "All instances (RAC)"), false),
		}, func(r Row, x Vals) (Script, error) {
			return sqlS("ALTER PLUGGABLE DATABASE %s OPEN %s%s%s;", Ident(r["NAME"]), x["MODE"],
				iff(Yes(x["RESTRICTED"]), " RESTRICTED", ""), iff(Yes(x["ALL"]), " INSTANCES = ALL", ""))
		}),
		act('c', L("Fechar", "Close"), true, []Field{fB("ALL", L("Todas as instâncias (RAC)", "All instances (RAC)"), false)},
			func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER PLUGGABLE DATABASE %s CLOSE IMMEDIATE%s;", Ident(r["NAME"]), iff(Yes(x["ALL"]), " INSTANCES = ALL", ""))
			}),
		act('s', L("Gravar / descartar estado (abre no startup)", "Save / discard state (opens on startup)"), true,
			[]Field{fC("MODE", "", "SAVE", "SAVE", "DISCARD")},
			func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER PLUGGABLE DATABASE %s %s STATE;", Ident(r["NAME"]), x["MODE"])
			}),
		act('n', L("Criar a partir do seed", "Create from seed"), false, []Field{
			fR("NAME", L("Nome da PDB", "PDB name"), ""), fR("ADMIN", L("Usuário admin", "Admin user"), "PDBADMIN"),
			fP("PWD", L("Senha do admin", "Admin password")), fnc,
			fT("MAX", L("Tamanho máximo (vazio = sem limite)", "Max size (empty = unlimited)"), ""),
			fB("OPEN", L("Abrir em seguida", "Open right away"), true),
		}, func(_ Row, x Vals) (Script, error) {
			n := IdentIn(x["NAME"])
			s := fmt.Sprintf("CREATE PLUGGABLE DATABASE %s ADMIN USER %s IDENTIFIED BY \"%s\"%s", n, IdentIn(x["ADMIN"]), x["PWD"], fncClause(x))
			if m := strings.TrimSpace(x["MAX"]); m != "" {
				sz, err := Size("MAX", m)
				if err != nil {
					return Script{}, err
				}
				s += "\n  STORAGE (MAXSIZE " + sz + ")"
			}
			s += ";"
			if Yes(x["OPEN"]) {
				s += "\nALTER PLUGGABLE DATABASE " + n + " OPEN;"
			}
			return sqlS("%s", s)
		}),
		act('k', L("Clonar", "Clone"), true, []Field{
			fR("NAME", L("Nome da nova PDB", "New PDB name"), "{NAME}_CLONE"),
			fT("LINK", L("DB link da origem (clone remoto)", "Source DB link (remote clone)"), ""), fnc,
			fB("SNAP", "SNAPSHOT COPY", false), fB("OPEN", L("Abrir em seguida", "Open right away"), true),
		}, func(r Row, x Vals) (Script, error) {
			n := IdentIn(x["NAME"])
			src := Ident(r["NAME"])
			if l := strings.TrimSpace(x["LINK"]); l != "" {
				src += "@" + l
			}
			s := "CREATE PLUGGABLE DATABASE " + n + " FROM " + src + fncClause(x) + iff(Yes(x["SNAP"]), "\n  SNAPSHOT COPY", "") + ";"
			if Yes(x["OPEN"]) {
				s += "\nALTER PLUGGABLE DATABASE " + n + " OPEN;"
			}
			return sqlS("%s", s)
		}),
		act('r', L("Refresh (PDB refreshable)", "Refresh (refreshable PDB)"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER PLUGGABLE DATABASE %s REFRESH;", Ident(r["NAME"]))
		}),
		danger(act('u', L("Unplug para XML", "Unplug to XML"), true, []Field{
			fR("XML", L("Arquivo XML", "XML file"), "/tmp/{NAME}.xml"),
		}, func(r Row, x Vals) (Script, error) {
			n := Ident(r["NAME"])
			return sqlS("ALTER PLUGGABLE DATABASE %s CLOSE IMMEDIATE;\nALTER PLUGGABLE DATABASE %s UNPLUG INTO %s;", n, n, Lit(x["XML"]))
		}), "NAME"),
		act('p', L("Plug a partir de XML", "Plug from XML"), false, []Field{
			fR("NAME", L("Nome da PDB", "PDB name"), ""), fR("XML", L("Arquivo XML", "XML file"), ""),
			fC("COPY", L("Arquivos", "Files"), "NOCOPY", "NOCOPY", "COPY", "MOVE"), fnc,
			fB("TEMP", "TEMPFILE REUSE", true), fB("OPEN", L("Abrir em seguida", "Open right away"), true),
		}, func(_ Row, x Vals) (Script, error) {
			n := IdentIn(x["NAME"])
			s := "CREATE PLUGGABLE DATABASE " + n + " USING " + Lit(x["XML"]) + " " + x["COPY"] + fncClause(x) +
				iff(Yes(x["TEMP"]), "\n  TEMPFILE REUSE", "") + ";"
			if Yes(x["OPEN"]) {
				s += "\nALTER PLUGGABLE DATABASE " + n + " OPEN;"
			}
			return sqlS("%s", s)
		}),
		act('l', L("Relocate (de outro CDB)", "Relocate (from another CDB)"), false, []Field{
			fR("NAME", L("Nome da PDB", "PDB name"), ""), fR("LINK", L("DB link para o CDB de origem", "DB link to source CDB"), ""),
		}, func(_ Row, x Vals) (Script, error) {
			n := IdentIn(x["NAME"])
			return sqlS("CREATE PLUGGABLE DATABASE %s FROM %s@%s RELOCATE;\nALTER PLUGGABLE DATABASE %s OPEN;", n, n, strings.TrimSpace(x["LINK"]), n)
		}),
		danger(act('x', L("Remover (DROP)", "Drop"), true, []Field{
			fC("FILES", L("Arquivos", "Files"), "INCLUDING DATAFILES", "INCLUDING DATAFILES", "KEEP DATAFILES"),
		}, func(r Row, x Vals) (Script, error) {
			n := Ident(r["NAME"])
			return sqlS("ALTER PLUGGABLE DATABASE %s CLOSE IMMEDIATE;\nDROP PLUGGABLE DATABASE %s %s;", n, n, x["FILES"])
		}), "NAME"),
	)
	pdbs.Needs = "cdb"
	pdbs.KeyCol = "NAME"

	return &Node{Title: L("Banco de dados", "Database"), Children: []*Node{
		{Title: L("Painel da instância", "Instance viewer"), View: &View{ID: "db.panel", Title: L("Painel da instância", "Instance viewer"), Special: "instance", Refresh: 5}},
		{Title: "Top Activity", View: &View{ID: "db.topactivity", Title: "Top Activity", Special: "topactivity", Refresh: 2,
			Help: L("Amostra o V$SESSION a cada 2 s (não usa ASH: dispensa o Diagnostics Pack).",
				"Samples V$SESSION every 2 s (no ASH: no Diagnostics Pack needed).")}},
		{Title: L("Banco", "Database"), View: db},
		{Title: L("Instância", "Instance"), View: inst},
		{Title: L("Parâmetros", "Parameters"), View: paramsView("db.params", L("Parâmetros", "Parameters"), "")},
		{Title: L("Parâmetros alterados", "Modified parameters"), View: paramsView("db.params.mod", L("Parâmetros alterados", "Modified parameters"), "where isdefault = 'FALSE'")},
		{Title: L("Serviços", "Services"), View: svcs},
		{Title: "Pluggable databases", View: pdbs},
		{Title: L("Componentes (registry)", "Components (registry)"), View: v("db.registry", L("Componentes", "Components"),
			`select comp_id, comp_name, version, status, modified, schema from dba_registry order by comp_id`)},
		{Title: L("Opções", "Options"), View: v("db.options", L("Opções", "Options"), `select parameter, value from v$option order by parameter`)},
		{Title: L("Uso de recursos (licença)", "Feature usage (licensing)"), View: v("db.features", L("Uso de recursos", "Feature usage"),
			`select name, version, detected_usages, currently_used, first_usage_date, last_usage_date, description
			   from dba_feature_usage_statistics where detected_usages > 0 order by name`)},
		{Title: L("Versão", "Version"), View: v("db.version", L("Versão", "Version"), `select banner from v$version`)},
	}}
}
