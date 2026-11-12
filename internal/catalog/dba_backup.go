// Aba DBA · Backup e Recovery (RMAN): configuração, área de recuperação, jobs,
// backup sets, cópias, backup, manutenção, restore/recover e flashback.
// Os scripts RMAN vão para o comando RMAN da conexão (ssh/docker exec/local).
// by • DBA BRABO
package catalog

import (
	"errors"
	"fmt"
	"strings"
)

func untilClause(x Vals) (string, error) {
	val := strings.TrimSpace(x["VALUE"])
	switch x["UNTIL"] {
	case "", L("(nenhum)", "(none)"):
		return "", nil
	case "TIME":
		if val == "" {
			return "", errors.New(L("informe a data (AAAA-MM-DD HH24:MI:SS)", "fill the date (YYYY-MM-DD HH24:MI:SS)"))
		}
		return fmt.Sprintf(`SET UNTIL TIME "TO_DATE('%s','YYYY-MM-DD HH24:MI:SS')";`, strings.ReplaceAll(val, "'", "")), nil
	case "SCN":
		n, err := Num("SCN", val)
		if err != nil {
			return "", err
		}
		return "SET UNTIL SCN " + n + ";", nil
	case "SEQUENCE":
		parts := strings.Fields(val)
		if len(parts) == 0 {
			return "", errors.New(L("informe a sequência (e a thread: 120 1)", "fill the sequence (and thread: 120 1)"))
		}
		n, err := Num("SEQUENCE", parts[0])
		if err != nil {
			return "", err
		}
		th := "1"
		if len(parts) > 1 {
			if th, err = Num("THREAD", parts[1]); err != nil {
				return "", err
			}
		}
		return "SET UNTIL SEQUENCE " + n + " THREAD " + th + ";", nil
	case "RESTORE POINT":
		if val == "" {
			return "", errors.New(L("informe o restore point", "fill the restore point"))
		}
		return "SET UNTIL RESTORE POINT " + IdentIn(val) + ";", nil
	}
	return "", fmt.Errorf("UNTIL %s?", x["UNTIL"])
}

func untilFields() []Field {
	return []Field{
		fC("UNTIL", L("Até (recuperação incompleta)", "Until (incomplete recovery)"), L("(nenhum)", "(none)"),
			L("(nenhum)", "(none)"), "TIME", "SCN", "SEQUENCE", "RESTORE POINT"),
		fT("VALUE", L("Valor do 'até'", "'Until' value"), ""),
	}
}

func backupOpts(x Vals) string {
	var b strings.Builder
	if Yes(x["COMPRESSED"]) {
		b.WriteString(" AS COMPRESSED BACKUPSET")
	}
	switch x["LEVEL"] {
	case "0":
		b.WriteString(" INCREMENTAL LEVEL 0")
	case "1":
		b.WriteString(" INCREMENTAL LEVEL 1")
	case "1 CUMULATIVE":
		b.WriteString(" INCREMENTAL LEVEL 1 CUMULATIVE")
	}
	if d := x["DEVICE"]; d != "" {
		b.WriteString(" DEVICE TYPE " + d)
	}
	if s := strings.TrimSpace(x["SECTION"]); s != "" {
		b.WriteString(" SECTION SIZE " + strings.ToUpper(s))
	}
	if t := strings.TrimSpace(x["TAG"]); t != "" {
		b.WriteString(" TAG " + Lit(strings.ToUpper(t)))
	}
	if f := strings.TrimSpace(x["FORMAT"]); f != "" {
		b.WriteString(" FORMAT " + Lit(f))
	}
	return b.String()
}

func backupFields(withLevel bool) []Field {
	f := []Field{}
	if withLevel {
		f = append(f, fC("LEVEL", L("Tipo", "Type"), "FULL", "FULL", "0", "1", "1 CUMULATIVE"))
	}
	return append(f,
		fC("DEVICE", L("Destino", "Device"), "DISK", "DISK", "SBT"),
		fB("COMPRESSED", L("Comprimido", "Compressed"), true),
		fT("TAG", "TAG", ""),
		fT("FORMAT", L("FORMAT (vazio = padrão/FRA)", "FORMAT (empty = default/FRA)"), ""),
		fT("SECTION", L("SECTION SIZE (multisection, ex.: 8G)", "SECTION SIZE (multisection, e.g. 8G)"), ""),
	)
}

func backupNode() *Node {
	cfg := v("bk.config", L("Configuração RMAN (não padrão)", "RMAN configuration (non-default)"), `select conf#, name, value from v$rman_configuration order by conf#`,
		rep('w', L("SHOW ALL (tudo, via RMAN)", "SHOW ALL (everything, via RMAN)"), false, nil, func(Row, Vals) (Script, error) {
			return Script{Kind: RMAN, Text: "SHOW ALL;"}, nil
		}),
		act('r', L("Retenção", "Retention policy"), false, []Field{
			fC("TYPE", L("Tipo", "Type"), "RECOVERY WINDOW", "RECOVERY WINDOW", "REDUNDANCY", "NONE"), fT("N", L("Dias / cópias", "Days / copies"), "7"),
		}, func(_ Row, x Vals) (Script, error) {
			switch x["TYPE"] {
			case "NONE":
				return rmanS("CONFIGURE RETENTION POLICY TO NONE;")
			case "REDUNDANCY":
				n, err := Num("N", x["N"])
				if err != nil {
					return Script{}, err
				}
				return rmanS("CONFIGURE RETENTION POLICY TO REDUNDANCY %s;", n)
			}
			n, err := Num("N", x["N"])
			if err != nil {
				return Script{}, err
			}
			return rmanS("CONFIGURE RETENTION POLICY TO RECOVERY WINDOW OF %s DAYS;", n)
		}),
		act('o', L("Otimização de backup", "Backup optimization"), false, []Field{fC("MODE", "", "ON", "ON", "OFF")},
			func(_ Row, x Vals) (Script, error) { return rmanS("CONFIGURE BACKUP OPTIMIZATION %s;", x["MODE"]) }),
		act('d', L("Dispositivo padrão", "Default device"), false, []Field{fC("DEV", "", "DISK", "DISK", "SBT")},
			func(_ Row, x Vals) (Script, error) { return rmanS("CONFIGURE DEFAULT DEVICE TYPE TO %s;", x["DEV"]) }),
		act('p', L("Paralelismo e tipo de backup", "Parallelism and backup type"), false, []Field{
			fC("DEV", L("Dispositivo", "Device"), "DISK", "DISK", "SBT"), fR("N", L("Paralelismo", "Parallelism"), "2"),
			fC("TYPE", L("Tipo", "Type"), "COMPRESSED BACKUPSET", "BACKUPSET", "COMPRESSED BACKUPSET", "COPY"),
		}, func(_ Row, x Vals) (Script, error) {
			n, err := Num("N", x["N"])
			if err != nil {
				return Script{}, err
			}
			return rmanS("CONFIGURE DEVICE TYPE %s PARALLELISM %s BACKUP TYPE TO %s;", x["DEV"], n, x["TYPE"])
		}),
		act('c', L("Algoritmo de compressão", "Compression algorithm"), false, []Field{
			fC("ALG", L("Algoritmo (LOW/MEDIUM/HIGH pedem Advanced Compression)", "Algorithm (LOW/MEDIUM/HIGH need Advanced Compression)"), "BASIC", "BASIC", "LOW", "MEDIUM", "HIGH"),
		}, func(_ Row, x Vals) (Script, error) { return rmanS("CONFIGURE COMPRESSION ALGORITHM '%s';", x["ALG"]) }),
		act('a', L("Autobackup do control file", "Control file autobackup"), false, []Field{
			fC("MODE", "", "ON", "ON", "OFF"), fT("FORMAT", L("FORMAT (vazio = padrão; ex.: /bkp/%F)", "FORMAT (empty = default; e.g. /bkp/%F)"), ""),
		}, func(_ Row, x Vals) (Script, error) {
			s := "CONFIGURE CONTROLFILE AUTOBACKUP " + x["MODE"] + ";"
			if f := strings.TrimSpace(x["FORMAT"]); f != "" {
				s += "\nCONFIGURE CONTROLFILE AUTOBACKUP FORMAT FOR DEVICE TYPE DISK TO " + Lit(f) + ";"
			}
			return rmanS("%s", s)
		}),
		act('f', L("Formato do canal de disco", "Disk channel format"), false, []Field{fR("FORMAT", "FORMAT", "/u01/backup/%d_%T_%U")},
			func(_ Row, x Vals) (Script, error) {
				return rmanS("CONFIGURE CHANNEL DEVICE TYPE DISK FORMAT %s;", Lit(x["FORMAT"]))
			}),
		act('t', L("Parâmetros do canal de fita (SBT)", "Tape (SBT) channel parameters"), false, []Field{fR("PARMS", "PARMS", "SBT_LIBRARY=/opt/lib/libobk.so, ENV=(OB_MEDIA_FAMILY=rman)")},
			func(_ Row, x Vals) (Script, error) {
				return rmanS("CONFIGURE CHANNEL DEVICE TYPE SBT PARMS %s;", Lit(x["PARMS"]))
			}),
		act('l', L("Política de remoção de archive", "Archivelog deletion policy"), false, []Field{
			fC("POL", L("Política", "Policy"), "NONE", "NONE", "APPLIED ON ALL STANDBY", "SHIPPED TO ALL STANDBY", "BACKED UP 1 TIMES TO DISK", "BACKED UP 2 TIMES TO DISK", "BACKED UP 1 TIMES TO SBT"),
		}, func(_ Row, x Vals) (Script, error) {
			return rmanS("CONFIGURE ARCHIVELOG DELETION POLICY TO %s;", x["POL"])
		}),
		act('e', L("Criptografia do backup", "Backup encryption"), false, []Field{
			fC("MODE", "", "ON", "ON", "OFF"), fC("ALG", L("Algoritmo", "Algorithm"), "AES256", "AES128", "AES192", "AES256"),
		}, func(_ Row, x Vals) (Script, error) {
			return rmanS("CONFIGURE ENCRYPTION FOR DATABASE %s;\nCONFIGURE ENCRYPTION ALGORITHM '%s';", x["MODE"], x["ALG"])
		}),
		act('s', L("Snapshot do control file", "Snapshot control file"), false, []Field{fR("FILE", L("Arquivo", "File"), "/u01/app/oracle/snapcf.f")},
			func(_ Row, x Vals) (Script, error) {
				return rmanS("CONFIGURE SNAPSHOT CONTROLFILE NAME TO %s;", Lit(x["FILE"]))
			}),
		act('x', L("Voltar ao padrão (CLEAR)", "Back to default (CLEAR)"), true, nil, func(r Row, _ Vals) (Script, error) {
			return rmanS("CONFIGURE %s CLEAR;", r["NAME"])
		}),
	)
	cfg.Needs = "rman"

	usage := v("bk.fra.usage", L("Uso da área de recuperação", "Recovery area usage"), `select file_type, percent_space_used pct_used, percent_space_reclaimable pct_reclaim, number_of_files files
	   from v$recovery_area_usage where nvl(:1, 'x') is not null order by percent_space_used desc`)
	fra := v("bk.fra", L("Área de recuperação (FRA)", "Recovery area (FRA)"), `select name, round(space_limit/1048576) limit_mb, round(space_used/1048576) used_mb,
	        round(space_reclaimable/1048576) reclaim_mb, number_of_files files
	   from v$recovery_file_dest`,
		act('z', L("Tamanho da FRA", "FRA size"), false, []Field{fR("SIZE", L("Tamanho", "Size"), "20G")},
			func(_ Row, x Vals) (Script, error) {
				sz, err := Size("SIZE", x["SIZE"])
				if err != nil {
					return Script{}, err
				}
				return sqlS("ALTER SYSTEM SET db_recovery_file_dest_size = %s SCOPE = BOTH;", sz)
			}),
		act('d', L("Local da FRA", "FRA location"), false, []Field{fR("DEST", L("Caminho ou +DG", "Path or +DG"), "+FRA")},
			func(_ Row, x Vals) (Script, error) {
				return sqlS("ALTER SYSTEM SET db_recovery_file_dest = %s SCOPE = BOTH;", Lit(x["DEST"]))
			}),
		act('m', L("Meta de recuperação de instância (MTTR)", "Instance recovery target (MTTR)"), false, []Field{fR("SEC", L("Segundos", "Seconds"), "300")},
			func(_ Row, x Vals) (Script, error) {
				n, err := Num("SEC", x["SEC"])
				if err != nil {
					return Script{}, err
				}
				return sqlS("ALTER SYSTEM SET fast_start_mttr_target = %s SCOPE = BOTH;", n)
			}),
		act('f', L("Formato do archive", "Archive format"), false, []Field{fR("FMT", "log_archive_format", "%t_%s_%r.arc")},
			func(_ Row, x Vals) (Script, error) {
				return sqlS("ALTER SYSTEM SET log_archive_format = %s SCOPE = SPFILE;", Lit(x["FMT"]))
			}),
		danger(act('A', L("Ligar ARCHIVELOG (reinicia, RMAN)", "Enable ARCHIVELOG (restarts, RMAN)"), false, nil,
			func(Row, Vals) (Script, error) { return archivelogScript(true) }), ""),
		danger(act('N', L("Desligar ARCHIVELOG (reinicia, RMAN)", "Disable ARCHIVELOG (restarts, RMAN)"), false, nil,
			func(Row, Vals) (Script, error) { return archivelogScript(false) }), ""),
		act('b', "Flashback Database ON/OFF", false, []Field{fC("MODE", "", "ON", "ON", "OFF")},
			func(_ Row, x Vals) (Script, error) { return sqlS("ALTER DATABASE FLASHBACK %s;", x["MODE"]) }),
		act('t', L("Janela do flashback (minutos)", "Flashback window (minutes)"), false, []Field{fR("MIN", L("Minutos", "Minutes"), "1440")},
			func(_ Row, x Vals) (Script, error) {
				n, err := Num("MIN", x["MIN"])
				if err != nil {
					return Script{}, err
				}
				return sqlS("ALTER SYSTEM SET db_flashback_retention_target = %s SCOPE = BOTH;", n)
			}),
		drill('u', L("Uso por tipo", "Usage by type"), usage, one("NAME")),
	)

	output := v("bk.output", L("Saída do RMAN", "RMAN output"), `select output from v$rman_output where session_key = :1 order by recid`)
	output.Special = "text"
	// ações de backup e manutenção (ficam na tela de jobs)
	jobs := v("bk.jobs", L("Jobs de backup", "Backup jobs"), top(300, `select session_key, input_type, status, start_time, end_time,
	        round(elapsed_seconds) secs, input_bytes_display input, output_bytes_display output,
	        output_device_type device, round(compression_ratio, 2) ratio
	   from v$rman_backup_job_details order by start_time desc`),
		act('b', L("Backup do banco inteiro", "Whole database backup"), false, append(backupFields(true),
			fC("ARCH", L("Archive logs", "Archive logs"), "PLUS ARCHIVELOG", "PLUS ARCHIVELOG", L("(sem)", "(none)")),
			fB("DELETE", L("Apagar archives copiados (DELETE INPUT)", "Delete backed-up archives (DELETE INPUT)"), false),
			fB("CTL", L("Control file e spfile junto", "Control file and spfile too"), true),
		), func(_ Row, x Vals) (Script, error) {
			s := "BACKUP" + backupOpts(x) + " DATABASE"
			if x["ARCH"] == "PLUS ARCHIVELOG" {
				s += " PLUS ARCHIVELOG" + iff(Yes(x["DELETE"]), " DELETE INPUT", "")
			}
			s += ";"
			if Yes(x["CTL"]) {
				s += "\nBACKUP CURRENT CONTROLFILE;\nBACKUP SPFILE;"
			}
			return rmanS("%s", s)
		}),
		act('t', L("Backup de tablespaces", "Tablespace backup"), false, append([]Field{fR("TS", L("Tablespaces (vírgula)", "Tablespaces (comma)"), "USERS")}, backupFields(true)...),
			func(_ Row, x Vals) (Script, error) {
				return rmanS("BACKUP%s TABLESPACE %s;", backupOpts(x), identList(x["TS"]))
			}),
		act('f', L("Backup de datafiles", "Datafile backup"), false, append([]Field{fR("FILES", L("Números dos datafiles (vírgula)", "Datafile numbers (comma)"), "1")}, backupFields(true)...),
			func(_ Row, x Vals) (Script, error) {
				var ns []string
				for _, p := range strings.Split(x["FILES"], ",") {
					n, err := Num("FILE", p)
					if err != nil {
						return Script{}, err
					}
					ns = append(ns, n)
				}
				return rmanS("BACKUP%s DATAFILE %s;", backupOpts(x), strings.Join(ns, ", "))
			}),
		act('a', L("Backup de archive logs", "Archivelog backup"), false, append([]Field{
			fC("WHICH", L("Quais", "Which"), "ALL", "ALL", "NOT BACKED UP 1 TIMES", "FROM TIME 'SYSDATE-1'"),
			fB("DELETE", "DELETE INPUT", false),
		}, backupFields(false)...), func(_ Row, x Vals) (Script, error) {
			w := "ALL"
			if x["WHICH"] == "NOT BACKED UP 1 TIMES" {
				w = "ALL NOT BACKED UP 1 TIMES"
			} else if strings.HasPrefix(x["WHICH"], "FROM") {
				w = x["WHICH"]
			}
			return rmanS("BACKUP%s ARCHIVELOG %s%s;", backupOpts(x), w, iff(Yes(x["DELETE"]), " DELETE INPUT", ""))
		}),
		act('r', L("Backup da área de recuperação para fita", "Recovery area backup to tape"), false, nil, func(Row, Vals) (Script, error) {
			return rmanS("BACKUP DEVICE TYPE SBT RECOVERY AREA;")
		}),
		act('c', L("Script RMAN livre", "Custom RMAN script"), false, []Field{fM("SCRIPT", "Script", "RUN {\n  BACKUP DATABASE;\n}")},
			func(_ Row, x Vals) (Script, error) { return rmanS("%s", x["SCRIPT"]) }),
		act('k', "CROSSCHECK BACKUP / ARCHIVELOG ALL", false, nil, func(Row, Vals) (Script, error) {
			return rmanS("CROSSCHECK BACKUP;\nCROSSCHECK COPY;\nCROSSCHECK ARCHIVELOG ALL;")
		}),
		danger(act('e', "DELETE EXPIRED", false, nil, func(Row, Vals) (Script, error) {
			return rmanS("DELETE NOPROMPT EXPIRED BACKUP;\nDELETE NOPROMPT EXPIRED COPY;\nDELETE NOPROMPT EXPIRED ARCHIVELOG ALL;")
		}), ""),
		danger(act('o', "DELETE OBSOLETE", false, nil, func(Row, Vals) (Script, error) {
			return rmanS("DELETE NOPROMPT OBSOLETE;")
		}), ""),
		rep('R', "REPORT OBSOLETE / NEED BACKUP", false, nil, func(Row, Vals) (Script, error) {
			return Script{Kind: RMAN, Text: "REPORT OBSOLETE;\nREPORT NEED BACKUP;\nREPORT UNRECOVERABLE;"}, nil
		}),
		rep('L', "LIST BACKUP SUMMARY", false, nil, func(Row, Vals) (Script, error) {
			return Script{Kind: RMAN, Text: "LIST BACKUP SUMMARY;"}, nil
		}),
		act('g', "CATALOG RECOVERY AREA", false, nil, func(Row, Vals) (Script, error) {
			return rmanS("CATALOG RECOVERY AREA NOPROMPT;")
		}),
		drill('v', L("Saída do job", "Job output"), output, one("SESSION_KEY")),
	)
	jobs.Needs = "rman"

	pieces := v("bk.pieces", L("Peças do backup set", "Backup pieces"), `select recid, piece#, copy#, handle, device_type, status, round(bytes/1048576) mb, completion_time, tag
	   from v$backup_piece where (set_stamp, set_count) in (select set_stamp, set_count from v$backup_set where recid = :1)
	  order by piece#, copy#`)
	bsKey := func(r Row) string { return r["BS_KEY"] }
	sets := v("bk.sets", "Backup sets", top(500, `select s.recid bs_key, s.backup_type, s.incremental_level lvl, s.pieces, s.start_time, s.completion_time,
	        round(s.elapsed_seconds) secs, s.controlfile_included ctl, s.keep, s.keep_until,
	        (select max(p.device_type) from v$backup_piece p where p.set_stamp = s.set_stamp and p.set_count = s.set_count) device,
	        (select round(sum(p.bytes)/1048576) from v$backup_piece p where p.set_stamp = s.set_stamp and p.set_count = s.set_count) mb,
	        (select max(p.status) from v$backup_piece p where p.set_stamp = s.set_stamp and p.set_count = s.set_count) status,
	        (select max(p.tag) from v$backup_piece p where p.set_stamp = s.set_stamp and p.set_count = s.set_count) tag
	   from v$backup_set s order by s.completion_time desc`),
		act('k', "CROSSCHECK", true, nil, func(r Row, _ Vals) (Script, error) { return rmanS("CROSSCHECK BACKUPSET %s;", bsKey(r)) }),
		act('v', "VALIDATE", true, nil, func(r Row, _ Vals) (Script, error) { return rmanS("VALIDATE BACKUPSET %s;", bsKey(r)) }),
		act('u', L("Indisponível / disponível", "Unavailable / available"), true, []Field{fC("ST", "", "UNAVAILABLE", "UNAVAILABLE", "AVAILABLE")},
			func(r Row, x Vals) (Script, error) { return rmanS("CHANGE BACKUPSET %s %s;", bsKey(r), x["ST"]) }),
		act('p', L("Guardar (KEEP)", "Keep"), true, []Field{fC("MODE", "", "UNTIL", "UNTIL", "NOKEEP"), fT("DAYS", L("Dias", "Days"), "365")},
			func(r Row, x Vals) (Script, error) {
				if x["MODE"] == "NOKEEP" {
					return rmanS("CHANGE BACKUPSET %s NOKEEP;", bsKey(r))
				}
				n, err := Num("DAYS", x["DAYS"])
				if err != nil {
					return Script{}, err
				}
				return rmanS("CHANGE BACKUPSET %s KEEP UNTIL TIME 'SYSDATE+%s';", bsKey(r), n)
			}),
		danger(act('x', "DELETE", true, nil, func(r Row, _ Vals) (Script, error) {
			return rmanS("DELETE NOPROMPT BACKUPSET %s;", bsKey(r))
		}), "BS_KEY"),
		drill('e', L("Peças", "Pieces"), pieces, one("BS_KEY")),
	)
	sets.Needs = "rman"

	copies := v("bk.copies", L("Cópias (image copies)", "Image copies"), `select recid, file#, name, tag, completion_time, round(blocks*block_size/1048576) mb,
	        status, incremental_level lvl, keep
	   from v$datafile_copy where status <> 'D' order by completion_time desc`,
		act('k', "CROSSCHECK", true, nil, func(r Row, _ Vals) (Script, error) { return rmanS("CROSSCHECK DATAFILECOPY %s;", Lit(r["NAME"])) }),
		act('v', "VALIDATE", true, nil, func(r Row, _ Vals) (Script, error) { return rmanS("VALIDATE DATAFILECOPY %s;", Lit(r["NAME"])) }),
		act('u', L("Indisponível / disponível", "Unavailable / available"), true, []Field{fC("ST", "", "UNAVAILABLE", "UNAVAILABLE", "AVAILABLE")},
			func(r Row, x Vals) (Script, error) {
				return rmanS("CHANGE DATAFILECOPY %s %s;", Lit(r["NAME"]), x["ST"])
			}),
		danger(act('x', "DELETE", true, nil, func(r Row, _ Vals) (Script, error) {
			return rmanS("DELETE NOPROMPT DATAFILECOPY %s;", Lit(r["NAME"]))
		}), "FILE#"),
		act('s', L("Trocar o datafile por esta cópia (SWITCH)", "Switch datafile to this copy"), true, nil, func(r Row, _ Vals) (Script, error) {
			return rmanS("SQL 'ALTER DATABASE DATAFILE %s OFFLINE';\nSWITCH DATAFILE %s TO COPY;\nRECOVER DATAFILE %s;\nSQL 'ALTER DATABASE DATAFILE %s ONLINE';", r["FILE#"], r["FILE#"], r["FILE#"], r["FILE#"])
		}),
	)
	copies.Needs = "rman"

	recovery := v("bk.recovery", L("Recuperação (blocos corrompidos)", "Recovery (corrupt blocks)"), `select file#, block#, blocks, corruption_change#, corruption_type from v$database_block_corruption order by file#, block#`,
		act('B', L("Recuperar este bloco", "Recover this block"), true, nil, func(r Row, _ Vals) (Script, error) {
			return rmanS("RECOVER DATAFILE %s BLOCK %s;", r["FILE#"], r["BLOCK#"])
		}),
		act('C', L("Recuperar todos os corrompidos", "Recover all corrupt blocks"), false, nil, func(Row, Vals) (Script, error) {
			return rmanS("RECOVER CORRUPTION LIST;")
		}),
		act('V', L("Validar o banco (preenche esta lista)", "Validate database (fills this list)"), false, []Field{fB("LOGICAL", "CHECK LOGICAL", true)},
			func(_ Row, x Vals) (Script, error) {
				return rmanS("VALIDATE%s DATABASE;", iff(Yes(x["LOGICAL"]), " CHECK LOGICAL", ""))
			}),
		rep('P', L("Prévia do restore (o que seria usado)", "Restore preview (what would be used)"), false, nil, func(Row, Vals) (Script, error) {
			return Script{Kind: RMAN, Text: "RESTORE DATABASE PREVIEW SUMMARY;"}, nil
		}),
		act('T', L("Testar o restore (VALIDATE, não grava)", "Test the restore (VALIDATE, writes nothing)"), false, nil, func(Row, Vals) (Script, error) {
			return rmanS("RESTORE DATABASE VALIDATE;\nRESTORE ARCHIVELOG ALL VALIDATE;")
		}),
		danger(act('D', L("Restore + recover do banco inteiro", "Restore + recover whole database"), false, untilFields(),
			func(_ Row, x Vals) (Script, error) {
				u, err := untilClause(x)
				if err != nil {
					return Script{}, err
				}
				s := "SHUTDOWN IMMEDIATE;\nSTARTUP MOUNT;\nRUN {\n"
				if u != "" {
					s += "  " + u + "\n"
				}
				s += "  RESTORE DATABASE;\n  RECOVER DATABASE;\n}\n"
				s += "ALTER DATABASE OPEN" + iff(u != "", " RESETLOGS", "") + ";"
				return rmanS("%s", s)
			}), ""),
		danger(act('S', L("Restore + recover de tablespace", "Restore + recover tablespace"), false, []Field{fR("TS", "Tablespace", "USERS")},
			func(_ Row, x Vals) (Script, error) {
				t := IdentIn(x["TS"])
				return rmanS("SQL 'ALTER TABLESPACE %s OFFLINE IMMEDIATE';\nRESTORE TABLESPACE %s;\nRECOVER TABLESPACE %s;\nSQL 'ALTER TABLESPACE %s ONLINE';", t, t, t, t)
			}), ""),
		danger(act('F', L("Restore + recover de datafile", "Restore + recover datafile"), false, []Field{fR("FILE", L("Número do datafile", "Datafile number"), "")},
			func(_ Row, x Vals) (Script, error) {
				n, err := Num("FILE", x["FILE"])
				if err != nil {
					return Script{}, err
				}
				return rmanS("SQL 'ALTER DATABASE DATAFILE %s OFFLINE';\nRESTORE DATAFILE %s;\nRECOVER DATAFILE %s;\nSQL 'ALTER DATABASE DATAFILE %s ONLINE';", n, n, n, n)
			}), ""),
		act('A', L("Restaurar archive logs", "Restore archive logs"), false, []Field{
			fR("FROM", L("Da sequência", "From sequence"), ""), fR("TO", L("Até a sequência", "Until sequence"), ""), fT("THREAD", "Thread", "1"),
			fT("DEST", L("Para (vazio = destino padrão)", "To (empty = default destination)"), ""),
		}, func(_ Row, x Vals) (Script, error) {
			a, err := Num("FROM", x["FROM"])
			if err != nil {
				return Script{}, err
			}
			b, err := Num("TO", x["TO"])
			if err != nil {
				return Script{}, err
			}
			th, err := Num("THREAD", x["THREAD"])
			if err != nil {
				return Script{}, err
			}
			s := "RUN {\n"
			if d := strings.TrimSpace(x["DEST"]); d != "" {
				s += "  SET ARCHIVELOG DESTINATION TO " + Lit(d) + ";\n"
			}
			s += fmt.Sprintf("  RESTORE ARCHIVELOG FROM SEQUENCE %s UNTIL SEQUENCE %s THREAD %s;\n}", a, b, th)
			return rmanS("%s", s)
		}),
		danger(act('R', L("Recuperar uma tabela (RECOVER TABLE, 12c+)", "Recover a table (RECOVER TABLE, 12c+)"), false, append([]Field{
			fR("TABLE", L("Tabela (OWNER.TABELA)", "Table (OWNER.TABLE)"), ""),
			fR("AUX", L("Destino auxiliar (disco livre)", "Auxiliary destination (free disk)"), "/u01/aux"),
			fT("REMAP", L("Renomear para (opcional)", "Remap to (optional)"), ""),
		}, untilFields()...), func(_ Row, x Vals) (Script, error) {
			u, err := untilClause(x)
			if err != nil {
				return Script{}, err
			}
			if u == "" {
				return Script{}, errors.New(L("RECOVER TABLE precisa de um 'até'", "RECOVER TABLE needs an 'until'"))
			}
			until := strings.TrimSuffix(strings.TrimPrefix(u, "SET "), ";")
			s := "RECOVER TABLE " + strings.ToUpper(strings.TrimSpace(x["TABLE"])) + "\n  " + until + "\n  AUXILIARY DESTINATION " + Lit(x["AUX"])
			if r := strings.TrimSpace(x["REMAP"]); r != "" {
				s += "\n  REMAP TABLE " + strings.ToUpper(strings.TrimSpace(x["TABLE"])) + ":" + strings.ToUpper(r)
			}
			return rmanS("%s;", s)
		}), ""),
		danger(act('L', L("Flashback do banco", "Flashback database"), false, []Field{
			fC("TO", L("Para", "To"), "RESTORE POINT", "RESTORE POINT", "SCN", "TIME"), fR("VALUE", L("Valor (nome, SCN ou AAAA-MM-DD HH24:MI:SS)", "Value (name, SCN or YYYY-MM-DD HH24:MI:SS)"), ""),
		}, func(_ Row, x Vals) (Script, error) {
			val := strings.TrimSpace(x["VALUE"])
			var to string
			switch x["TO"] {
			case "SCN":
				n, err := Num("SCN", val)
				if err != nil {
					return Script{}, err
				}
				to = "SCN " + n
			case "TIME":
				to = fmt.Sprintf(`TIME "TO_DATE('%s','YYYY-MM-DD HH24:MI:SS')"`, strings.ReplaceAll(val, "'", ""))
			default:
				to = "RESTORE POINT " + IdentIn(val)
			}
			return rmanS("SHUTDOWN IMMEDIATE;\nSTARTUP MOUNT;\nFLASHBACK DATABASE TO %s;\nALTER DATABASE OPEN RESETLOGS;", to)
		}), ""),
		rep('X', L("Falhas detectadas (LIST FAILURE)", "Detected failures (LIST FAILURE)"), false, nil, func(Row, Vals) (Script, error) {
			return Script{Kind: RMAN, Text: "LIST FAILURE;"}, nil
		}),
	)
	recovery.Needs = "rman"
	recovery.Help = L("Ações de restore param o banco: confira o comando antes de executar.",
		"Restore actions shut the database down: check the command before running.")

	return &Node{Title: L("Backup e Recovery", "Backup and Recovery"), Children: []*Node{
		{Title: L("Configuração RMAN", "RMAN settings"), View: cfg},
		{Title: L("Área de recuperação", "Recovery area"), View: fra},
		{Title: L("Backup e manutenção (jobs)", "Backup and maintenance (jobs)"), View: jobs},
		{Title: "Backup sets", View: sets},
		{Title: L("Cópias", "Image copies"), View: copies},
		{Title: L("Restore e recover", "Restore and recover"), View: recovery},
	}}
}
