// Aba DBA · Data Pump: export e import pelo DBMS_DATAPUMP (roda no servidor, sem
// expdp/impdp no cliente) e acompanhamento dos jobs. by • DBA BRABO
package catalog

import (
	"errors"
	"fmt"
	"strings"
)

func dpJobName(prefix, s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	return s
}

func dpCommon(x Vals) string {
	var b strings.Builder
	if n := strings.TrimSpace(x["PARALLEL"]); n != "" && n != "1" {
		fmt.Fprintf(&b, "  DBMS_DATAPUMP.SET_PARALLEL(h, %s);\n", n)
	}
	if e := strings.TrimSpace(x["EXCLUDE"]); e != "" {
		fmt.Fprintf(&b, "  DBMS_DATAPUMP.METADATA_FILTER(h, 'EXCLUDE_PATH_EXPR', %s);\n", Lit("IN ("+strList(e, true)+")"))
	}
	return b.String()
}

func dpFilters(mode, objects, owner string) (string, error) {
	switch mode {
	case "FULL":
		return "", nil
	case "SCHEMA":
		if strings.TrimSpace(objects) == "" {
			return "", errors.New(L("informe os schemas", "fill the schemas"))
		}
		return fmt.Sprintf("  DBMS_DATAPUMP.METADATA_FILTER(h, 'SCHEMA_EXPR', %s);\n", Lit("IN ("+strList(objects, true)+")")), nil
	case "TABLE":
		if strings.TrimSpace(owner) == "" || strings.TrimSpace(objects) == "" {
			return "", errors.New(L("informe o dono e as tabelas", "fill the owner and the tables"))
		}
		return fmt.Sprintf("  DBMS_DATAPUMP.METADATA_FILTER(h, 'SCHEMA_EXPR', %s);\n  DBMS_DATAPUMP.METADATA_FILTER(h, 'NAME_EXPR', %s);\n",
			Lit("IN ("+strList(owner, true)+")"), Lit("IN ("+strList(objects, true)+")")), nil
	case "TABLESPACE":
		if strings.TrimSpace(objects) == "" {
			return "", errors.New(L("informe as tablespaces", "fill the tablespaces"))
		}
		return fmt.Sprintf("  DBMS_DATAPUMP.METADATA_FILTER(h, 'TABLESPACE_EXPR', %s);\n", Lit("IN ("+strList(objects, true)+")")), nil
	}
	return "", fmt.Errorf("modo %s?", mode)
}

func dpFields(op string) []Field {
	f := []Field{
		fC("MODE", L("Modo", "Mode"), "SCHEMA", "SCHEMA", "TABLE", "FULL", "TABLESPACE"),
		fT("OBJECTS", L("Schemas / tabelas / tablespaces (vírgula)", "Schemas / tables / tablespaces (comma)"), ""),
		fT("OWNER", L("Dono das tabelas (modo TABLE)", "Table owner (TABLE mode)"), ""),
		fR("DIR", L("Diretório", "Directory"), "DATA_PUMP_DIR"),
		fR("DUMP", L("Arquivo(s) de dump", "Dump file(s)"), strings.ToLower(op)+"_%U.dmp"),
		fT("LOG", L("Log (vazio = sem log)", "Log (empty = no log)"), strings.ToLower(op)+".log"),
		fT("JOB", L("Nome do job (vazio = automático)", "Job name (empty = automatic)"), ""),
		fN("PARALLEL", L("Paralelismo", "Parallelism"), "1"),
		fT("EXCLUDE", L("Excluir (ex.: STATISTICS, GRANT)", "Exclude (e.g. STATISTICS, GRANT)"), ""),
	}
	return f
}

func datapumpNode() *Node {
	progress := v("dp.progress", L("Progresso do job", "Job progress"), `select sid, serial#, opname, target, sofar, totalwork,
	        round(sofar*100/nullif(totalwork, 0), 1) pct, elapsed_seconds, time_remaining, message
	   from v$session_longops where opname = :1 order by start_time desc`)
	attach := func(r Row, body string) (Script, error) {
		return sqlS("DECLARE\n  h NUMBER;\nBEGIN\n  h := DBMS_DATAPUMP.ATTACH(%s, %s);\n  %s\nEND;", Lit(r["JOB_NAME"]), Lit(r["OWNER_NAME"]), body)
	}
	jobs := v("dp.jobs", L("Jobs do Data Pump", "Data Pump jobs"), `select owner_name, job_name, operation, job_mode, state, degree, attached_sessions, datapump_sessions
	   from dba_datapump_jobs order by owner_name, job_name`,
		act('e', "Export", false, append(dpFields("EXP"),
			fC("CONTENT", L("Conteúdo", "Content"), "ALL", "ALL", "METADATA_ONLY", "DATA_ONLY"),
			fC("COMPRESSION", L("Compressão (ALL/DATA_ONLY pedem Advanced Compression)", "Compression (ALL/DATA_ONLY need Advanced Compression)"), "METADATA_ONLY", "NONE", "METADATA_ONLY", "ALL", "DATA_ONLY"),
			fB("CONSISTENT", L("Consistente (FLASHBACK_SCN de agora)", "Consistent (FLASHBACK_SCN now)"), true),
			fB("REUSE", L("Sobrescrever dump existente", "Overwrite existing dump"), false),
		), func(_ Row, x Vals) (Script, error) {
			flt, err := dpFilters(x["MODE"], x["OBJECTS"], x["OWNER"])
			if err != nil {
				return Script{}, err
			}
			var b strings.Builder
			b.WriteString("DECLARE\n  h NUMBER;\n  scn NUMBER;\nBEGIN\n")
			fmt.Fprintf(&b, "  h := DBMS_DATAPUMP.OPEN(operation => 'EXPORT', job_mode => %s%s);\n", Lit(x["MODE"]),
				iff(dpJobName("EXP", x["JOB"]) != "", ", job_name => "+Lit(dpJobName("EXP", x["JOB"])), ""))
			fmt.Fprintf(&b, "  DBMS_DATAPUMP.ADD_FILE(h, %s, %s%s);\n", Lit(x["DUMP"]), Lit(strings.ToUpper(x["DIR"])), iff(Yes(x["REUSE"]), ", reusefile => 1", ""))
			if l := strings.TrimSpace(x["LOG"]); l != "" {
				fmt.Fprintf(&b, "  DBMS_DATAPUMP.ADD_FILE(h, %s, %s, filetype => DBMS_DATAPUMP.KU$_FILE_TYPE_LOG_FILE%s);\n", Lit(l), Lit(strings.ToUpper(x["DIR"])), iff(Yes(x["REUSE"]), ", reusefile => 1", ""))
			}
			b.WriteString(flt)
			switch x["CONTENT"] {
			case "METADATA_ONLY":
				b.WriteString("  DBMS_DATAPUMP.DATA_FILTER(h, 'INCLUDE_ROWS', 0);\n")
			case "DATA_ONLY":
				b.WriteString("  DBMS_DATAPUMP.METADATA_FILTER(h, 'INCLUDE_METADATA', 0);\n")
			}
			if c := x["COMPRESSION"]; c != "" && c != "NONE" {
				fmt.Fprintf(&b, "  DBMS_DATAPUMP.SET_PARAMETER(h, 'COMPRESSION', %s);\n", Lit(c))
			}
			if Yes(x["CONSISTENT"]) {
				b.WriteString("  SELECT current_scn INTO scn FROM v$database;\n  DBMS_DATAPUMP.SET_PARAMETER(h, 'FLASHBACK_SCN', scn);\n")
			}
			b.WriteString(dpCommon(x))
			b.WriteString("  DBMS_DATAPUMP.START_JOB(h);\n  DBMS_DATAPUMP.DETACH(h);\nEND;")
			return sqlS("%s", b.String())
		}),
		act('i', "Import", false, append(dpFields("IMP"),
			fT("REMAP_SCHEMA", L("Remap schema (A:B, C:D)", "Remap schema (A:B, C:D)"), ""),
			fT("REMAP_TS", L("Remap tablespace (A:B)", "Remap tablespace (A:B)"), ""),
			fC("EXISTS", L("Se a tabela existir", "If the table exists"), "SKIP", "SKIP", "APPEND", "TRUNCATE", "REPLACE"),
			fB("NOARCH", L("Sem gerar redo (DISABLE_ARCHIVE_LOGGING, 12c+)", "No redo (DISABLE_ARCHIVE_LOGGING, 12c+)"), false),
			fT("SQLFILE", L("Só gerar SQLFILE (nome; não importa nada)", "Only write SQLFILE (name; imports nothing)"), ""),
		), func(_ Row, x Vals) (Script, error) {
			flt, err := dpFilters(x["MODE"], x["OBJECTS"], x["OWNER"])
			if err != nil {
				return Script{}, err
			}
			op := "IMPORT"
			if strings.TrimSpace(x["SQLFILE"]) != "" {
				op = "SQL_FILE"
			}
			dir := Lit(strings.ToUpper(x["DIR"]))
			var b strings.Builder
			b.WriteString("DECLARE\n  h NUMBER;\nBEGIN\n")
			fmt.Fprintf(&b, "  h := DBMS_DATAPUMP.OPEN(operation => %s, job_mode => %s%s);\n", Lit(op), Lit(x["MODE"]),
				iff(dpJobName("IMP", x["JOB"]) != "", ", job_name => "+Lit(dpJobName("IMP", x["JOB"])), ""))
			fmt.Fprintf(&b, "  DBMS_DATAPUMP.ADD_FILE(h, %s, %s);\n", Lit(x["DUMP"]), dir)
			if l := strings.TrimSpace(x["LOG"]); l != "" {
				fmt.Fprintf(&b, "  DBMS_DATAPUMP.ADD_FILE(h, %s, %s, filetype => DBMS_DATAPUMP.KU$_FILE_TYPE_LOG_FILE);\n", Lit(l), dir)
			}
			if op == "SQL_FILE" {
				fmt.Fprintf(&b, "  DBMS_DATAPUMP.ADD_FILE(h, %s, %s, filetype => DBMS_DATAPUMP.KU$_FILE_TYPE_SQL_FILE);\n", Lit(strings.TrimSpace(x["SQLFILE"])), dir)
			}
			b.WriteString(flt)
			for _, rm := range []struct{ field, kind string }{{"REMAP_SCHEMA", "REMAP_SCHEMA"}, {"REMAP_TS", "REMAP_TABLESPACE"}} {
				for _, pair := range strings.Split(x[rm.field], ",") {
					pair = strings.TrimSpace(pair)
					if pair == "" {
						continue
					}
					ab := strings.SplitN(pair, ":", 2)
					if len(ab) != 2 {
						return Script{}, fmt.Errorf("%s: %q (use A:B)", rm.field, pair)
					}
					fmt.Fprintf(&b, "  DBMS_DATAPUMP.METADATA_REMAP(h, '%s', %s, %s);\n", rm.kind,
						Lit(strings.ToUpper(strings.TrimSpace(ab[0]))), Lit(strings.ToUpper(strings.TrimSpace(ab[1]))))
				}
			}
			if op == "IMPORT" {
				fmt.Fprintf(&b, "  DBMS_DATAPUMP.SET_PARAMETER(h, 'TABLE_EXISTS_ACTION', %s);\n", Lit(x["EXISTS"]))
				if Yes(x["NOARCH"]) {
					b.WriteString("  DBMS_DATAPUMP.METADATA_TRANSFORM(h, 'DISABLE_ARCHIVE_LOGGING', 1);\n")
				}
			}
			b.WriteString(dpCommon(x))
			b.WriteString("  DBMS_DATAPUMP.START_JOB(h);\n  DBMS_DATAPUMP.DETACH(h);\nEND;")
			return sqlS("%s", b.String())
		}),
		act('p', L("Parar (pode retomar)", "Stop (can resume)"), true, nil, func(r Row, _ Vals) (Script, error) {
			return attach(r, "DBMS_DATAPUMP.STOP_JOB(h, immediate => 0, keep_master => 1);")
		}),
		act('r', L("Retomar", "Resume"), true, nil, func(r Row, _ Vals) (Script, error) {
			return attach(r, "DBMS_DATAPUMP.START_JOB(h, skip_current => 0);\n  DBMS_DATAPUMP.DETACH(h);")
		}),
		act('g', L("Mudar paralelismo", "Change parallelism"), true, []Field{fR("N", L("Paralelismo", "Parallelism"), "{DEGREE}")}, func(r Row, x Vals) (Script, error) {
			n, err := Num("N", x["N"])
			if err != nil {
				return Script{}, err
			}
			return attach(r, "DBMS_DATAPUMP.SET_PARALLEL(h, "+n+");\n  DBMS_DATAPUMP.DETACH(h);")
		}),
		danger(act('k', L("Matar o job", "Kill the job"), true, nil, func(r Row, _ Vals) (Script, error) {
			return attach(r, "DBMS_DATAPUMP.STOP_JOB(h, immediate => 1, keep_master => 0);")
		}), "JOB_NAME"),
		danger(act('x', L("Limpar job órfão (DROP da master table)", "Clean orphan job (drop master table)"), true, nil, func(r Row, _ Vals) (Script, error) {
			if r["STATE"] != "NOT RUNNING" {
				return Script{}, fmt.Errorf(L("só para job NOT RUNNING (está %s)", "only for NOT RUNNING jobs (state %s)"), r["STATE"])
			}
			return sqlS("DROP TABLE %s PURGE;", Q2(r["OWNER_NAME"], r["JOB_NAME"]))
		}), "JOB_NAME"),
		drill('v', L("Progresso", "Progress"), progress, one("JOB_NAME")),
	)
	jobs.Refresh = 5
	jobs.Help = L("Export/import rodam no servidor: os arquivos ficam no diretório Oracle escolhido.",
		"Export/import run on the server: files go to the chosen Oracle directory.")

	return &Node{Title: "Data Pump", Children: []*Node{
		{Title: L("Jobs, export e import", "Jobs, export and import"), View: jobs},
		{Title: L("Diretórios", "Directories"), View: v("dp.dirs", L("Diretórios", "Directories"), `select owner, directory_name, directory_path from dba_directories order by directory_name`)},
	}}
}
