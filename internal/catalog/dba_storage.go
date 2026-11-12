// Aba DBA · Armazenamento: tablespaces, datafiles, temp groups, tablespace sets,
// control files, redo, archive logs, undo, restore points, lixeira, diretórios
// e maiores segmentos. by • DBA BRABO
package catalog

import (
	"fmt"
	"strings"
)

func autoextClause(x Vals) (string, error) {
	if !Yes(x["AUTO"]) {
		return " AUTOEXTEND OFF", nil
	}
	s := " AUTOEXTEND ON"
	if n := strings.TrimSpace(x["NEXT"]); n != "" {
		sz, err := Size("NEXT", n)
		if err != nil {
			return "", err
		}
		s += " NEXT " + sz
	}
	if m := strings.ToUpper(strings.TrimSpace(x["MAX"])); m != "" {
		if m != "UNLIMITED" {
			var err error
			if m, err = Size("MAXSIZE", m); err != nil {
				return "", err
			}
		}
		s += " MAXSIZE " + m
	}
	return s, nil
}

func fileFields(defSize string) []Field {
	return []Field{
		fT("FILE", L("Arquivo (vazio = OMF / +DATA)", "File (empty = OMF / +DATA)"), ""),
		fR("SIZE", L("Tamanho", "Size"), defSize),
		fB("AUTO", "AUTOEXTEND", true),
		fT("NEXT", "NEXT", "100M"),
		fT("MAX", "MAXSIZE", "UNLIMITED"),
	}
}

func fileSpec(x Vals) (string, error) {
	sz, err := Size(L("Tamanho", "Size"), x["SIZE"])
	if err != nil {
		return "", err
	}
	ae, err := autoextClause(x)
	if err != nil {
		return "", err
	}
	f := strings.TrimSpace(x["FILE"])
	if f != "" {
		return Lit(f) + " SIZE " + sz + ae, nil
	}
	return "SIZE " + sz + ae, nil
}

func fileKind(r Row) string {
	if r["CONTENTS"] == "TEMPORARY" || r["KIND"] == "TEMP" {
		return "TEMPFILE"
	}
	return "DATAFILE"
}

func datafilesView() *View {
	vw := v("st.datafiles", "Datafiles", `select * from (
	  select f.file_id, f.tablespace_name, f.file_name, round(f.bytes/1048576) mb, f.autoextensible auto,
	         round(f.maxbytes/1048576) max_mb, round(f.increment_by*t.block_size/1048576) next_mb,
	         f.status, f.online_status, 'DATA' kind
	    from dba_data_files f join dba_tablespaces t on t.tablespace_name = f.tablespace_name
	  union all
	  select f.file_id, f.tablespace_name, f.file_name, round(f.bytes/1048576), f.autoextensible,
	         round(f.maxbytes/1048576), round(f.increment_by*t.block_size/1048576), f.status, null, 'TEMP'
	    from dba_temp_files f join dba_tablespaces t on t.tablespace_name = f.tablespace_name)
	 where tablespace_name = nvl(:1, tablespace_name)
	 order by tablespace_name, kind, file_id`,
		act('z', L("Redimensionar", "Resize"), true, []Field{fR("SIZE", L("Novo tamanho", "New size"), "{MB}M")},
			func(r Row, x Vals) (Script, error) {
				sz, err := Size("SIZE", x["SIZE"])
				if err != nil {
					return Script{}, err
				}
				return sqlS("ALTER DATABASE %s %s RESIZE %s;", fileKind(r), Lit(r["FILE_NAME"]), sz)
			}),
		act('a', "Autoextend", true, []Field{fB("AUTO", "AUTOEXTEND", true), fT("NEXT", "NEXT", "100M"), fT("MAX", "MAXSIZE", "UNLIMITED")},
			func(r Row, x Vals) (Script, error) {
				ae, err := autoextClause(x)
				if err != nil {
					return Script{}, err
				}
				return sqlS("ALTER DATABASE %s %s%s;", fileKind(r), Lit(r["FILE_NAME"]), ae)
			}),
		act('o', L("Online", "Online"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER DATABASE %s %s ONLINE;", fileKind(r), Lit(r["FILE_NAME"]))
		}),
		danger(act('f', L("Offline", "Offline"), true, []Field{fB("DROP", L("OFFLINE DROP (banco em NOARCHIVELOG)", "OFFLINE DROP (NOARCHIVELOG database)"), false)},
			func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER DATABASE %s %s OFFLINE%s;", fileKind(r), Lit(r["FILE_NAME"]), iff(Yes(x["DROP"]), " DROP", ""))
			}), "FILE_ID"),
		act('m', L("Mover online (12c+)", "Move online (12c+)"), true, []Field{
			fR("TO", L("Novo arquivo (ou +DG)", "New file (or +DG)"), "{FILE_NAME}"), fB("KEEP", L("Manter o original", "Keep original"), false), fB("REUSE", "REUSE", false),
		}, func(r Row, x Vals) (Script, error) {
			return sqlS("ALTER DATABASE MOVE DATAFILE %s TO %s%s%s;", Lit(r["FILE_NAME"]), Lit(x["TO"]), iff(Yes(x["REUSE"]), " REUSE", ""), iff(Yes(x["KEEP"]), " KEEP", ""))
		}),
		danger(act('x', L("Remover arquivo", "Drop file"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER TABLESPACE %s DROP %s %s;", Ident(r["TABLESPACE_NAME"]), fileKind(r), Lit(r["FILE_NAME"]))
		}), "FILE_ID"),
	)
	vw.Args = nil1
	vw.KeyCol = "FILE_NAME"
	return vw
}

func segmentsView() *View {
	vw := v("st.segments", L("Maiores segmentos", "Largest segments"), top(300, `select owner, segment_name, partition_name, segment_type,
	        tablespace_name, round(bytes/1048576) mb, extents
	   from dba_segments where tablespace_name = nvl(:1, tablespace_name) order by bytes desc`),
		act('s', L("Shrink (tabela/índice)", "Shrink (table/index)"), true, []Field{fB("CASCADE", "CASCADE", true), fB("COMPACT", "COMPACT", false)},
			func(r Row, x Vals) (Script, error) {
				obj := Q2(r["OWNER"], r["SEGMENT_NAME"])
				switch {
				case strings.HasPrefix(r["SEGMENT_TYPE"], "TABLE"):
					return sqlS("ALTER TABLE %s ENABLE ROW MOVEMENT;\nALTER TABLE %s SHRINK SPACE%s%s;", obj, obj,
						iff(Yes(x["COMPACT"]), " COMPACT", ""), iff(Yes(x["CASCADE"]), " CASCADE", ""))
				case strings.HasPrefix(r["SEGMENT_TYPE"], "INDEX"):
					return sqlS("ALTER INDEX %s SHRINK SPACE%s;", obj, iff(Yes(x["COMPACT"]), " COMPACT", ""))
				}
				return Script{}, fmt.Errorf(L("shrink só para tabela ou índice (%s)", "shrink only for table or index (%s)"), r["SEGMENT_TYPE"])
			}),
		act('m', L("Mover / rebuild", "Move / rebuild"), true, []Field{fT("TS", L("Tablespace destino (vazio = mesma)", "Target tablespace (empty = same)"), ""), fB("ONLINE", "ONLINE", true)},
			func(r Row, x Vals) (Script, error) {
				obj := Q2(r["OWNER"], r["SEGMENT_NAME"])
				ts := ""
				if t := strings.TrimSpace(x["TS"]); t != "" {
					ts = " TABLESPACE " + IdentIn(t)
				}
				on := iff(Yes(x["ONLINE"]), " ONLINE", "")
				part := ""
				if p := r["PARTITION_NAME"]; p != "" {
					part = " PARTITION " + Ident(p)
				}
				switch {
				case strings.HasPrefix(r["SEGMENT_TYPE"], "TABLE"):
					return sqlS("ALTER TABLE %s MOVE%s%s%s;", obj, part, ts, on)
				case strings.HasPrefix(r["SEGMENT_TYPE"], "INDEX"):
					return sqlS("ALTER INDEX %s REBUILD%s%s%s;", obj, part, ts, on)
				}
				return Script{}, fmt.Errorf("move: %s", r["SEGMENT_TYPE"])
			}),
	)
	vw.Args = nil1
	return vw
}

func storageNode() *Node {
	dfs := datafilesView()
	segs := segmentsView()
	ts := v("st.tablespaces", "Tablespaces", `select t.tablespace_name, t.contents, t.status, t.bigfile, t.extent_management,
	        t.segment_space_management ssm, t.encrypted,
	        round(nvl((select sum(bytes) from dba_data_files f where f.tablespace_name = t.tablespace_name),
	                  (select sum(bytes) from dba_temp_files f where f.tablespace_name = t.tablespace_name))/1048576) alloc_mb,
	        round(m.used_space*t.block_size/1048576) used_mb,
	        round(m.tablespace_size*t.block_size/1048576) max_mb, round(m.used_percent, 1) pct_used
	   from dba_tablespaces t left join dba_tablespace_usage_metrics m on m.tablespace_name = t.tablespace_name
	  order by t.tablespace_name`,
		act('n', L("Criar", "Create"), false, append([]Field{
			fR("NAME", L("Nome", "Name"), ""),
			fC("TYPE", L("Tipo", "Type"), "PERMANENT", "PERMANENT", "TEMPORARY", "UNDO"),
			fB("BIGFILE", "BIGFILE", false),
		}, append(fileFields("100M"), fB("ENCRYPT", L("Cifrar (TDE)", "Encrypt (TDE)"), false))...),
			func(_ Row, x Vals) (Script, error) {
				spec, err := fileSpec(x)
				if err != nil {
					return Script{}, err
				}
				kind, file := "", "DATAFILE"
				switch x["TYPE"] {
				case "TEMPORARY":
					kind, file = "TEMPORARY ", "TEMPFILE"
				case "UNDO":
					kind = "UNDO "
				}
				// 23ai em diante o padrão virou BIGFILE: diz sempre qual é
				s := fmt.Sprintf("CREATE %s%sTABLESPACE %s %s %s", iff(Yes(x["BIGFILE"]), "BIGFILE ", "SMALLFILE "), kind, IdentIn(x["NAME"]), file, spec)
				if Yes(x["ENCRYPT"]) && x["TYPE"] == "PERMANENT" {
					s += " ENCRYPTION ENCRYPT"
				}
				return sqlS("%s;", s)
			}),
		act('a', L("Adicionar arquivo", "Add file"), true, fileFields("100M"), func(r Row, x Vals) (Script, error) {
			spec, err := fileSpec(x)
			if err != nil {
				return Script{}, err
			}
			return sqlS("ALTER TABLESPACE %s ADD %s %s;", Ident(r["TABLESPACE_NAME"]), fileKind(r), spec)
		}),
		act('o', "Online", true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER TABLESPACE %s ONLINE;", Ident(r["TABLESPACE_NAME"]))
		}),
		danger(act('f', "Offline", true, []Field{fC("MODE", "", "NORMAL", "NORMAL", "TEMPORARY", "IMMEDIATE")},
			func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER TABLESPACE %s OFFLINE %s;", Ident(r["TABLESPACE_NAME"]), x["MODE"])
			}), "TABLESPACE_NAME"),
		act('r', L("Somente leitura / leitura e escrita", "Read only / read write"), true, []Field{fC("MODE", "", "READ ONLY", "READ ONLY", "READ WRITE")},
			func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER TABLESPACE %s %s;", Ident(r["TABLESPACE_NAME"]), x["MODE"])
			}),
		act('m', L("Renomear", "Rename"), true, []Field{fR("NEW", L("Novo nome", "New name"), "")},
			func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER TABLESPACE %s RENAME TO %s;", Ident(r["TABLESPACE_NAME"]), IdentIn(x["NEW"]))
			}),
		act('d', L("Tornar padrão do banco", "Make database default"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER DATABASE DEFAULT %sTABLESPACE %s;", iff(r["CONTENTS"] == "TEMPORARY", "TEMPORARY ", ""), Ident(r["TABLESPACE_NAME"]))
		}),
		act('u', L("Tornar UNDO ativo", "Make active UNDO"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER SYSTEM SET undo_tablespace = %s SCOPE = BOTH;", Lit(r["TABLESPACE_NAME"]))
		}),
		act('k', L("Shrink (temporária)", "Shrink (temporary)"), true, []Field{fT("KEEP", L("Manter (ex.: 100M; vazio = mínimo)", "Keep (e.g. 100M; empty = minimum)"), "")},
			func(r Row, x Vals) (Script, error) {
				k := ""
				if s := strings.TrimSpace(x["KEEP"]); s != "" {
					sz, err := Size("KEEP", s)
					if err != nil {
						return Script{}, err
					}
					k = " KEEP " + sz
				}
				return sqlS("ALTER TABLESPACE %s SHRINK SPACE%s;", Ident(r["TABLESPACE_NAME"]), k)
			}),
		act('g', L("Grupo de temporárias", "Temporary group"), true, []Field{fT("GROUP", L("Grupo (vazio = tirar do grupo)", "Group (empty = remove from group)"), "")},
			func(r Row, x Vals) (Script, error) {
				g := "''"
				if s := strings.TrimSpace(x["GROUP"]); s != "" {
					g = IdentIn(s)
				}
				return sqlS("ALTER TABLESPACE %s TABLESPACE GROUP %s;", Ident(r["TABLESPACE_NAME"]), g)
			}),
		act('l', "Logging / nologging / force logging", true, []Field{fC("MODE", "", "LOGGING", "LOGGING", "NOLOGGING", "FORCE LOGGING", "NO FORCE LOGGING")},
			func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER TABLESPACE %s %s;", Ident(r["TABLESPACE_NAME"]), x["MODE"])
			}),
		danger(act('x', L("Remover (DROP)", "Drop"), true, []Field{
			fB("CONTENTS", "INCLUDING CONTENTS AND DATAFILES", true), fB("CASCADE", "CASCADE CONSTRAINTS", false),
		}, func(r Row, x Vals) (Script, error) {
			return sqlS("DROP TABLESPACE %s%s%s;", Ident(r["TABLESPACE_NAME"]),
				iff(Yes(x["CONTENTS"]), " INCLUDING CONTENTS AND DATAFILES", ""), iff(Yes(x["CASCADE"]), " CASCADE CONSTRAINTS", ""))
		}), "TABLESPACE_NAME"),
		rep('D', "DDL", true, nil, func(r Row, _ Vals) (Script, error) {
			return reportQ("select dbms_metadata.get_ddl('TABLESPACE', %s) from dual", Lit(r["TABLESPACE_NAME"]))
		}),
		drill('v', "Datafiles", dfs, one("TABLESPACE_NAME")),
		drill('e', L("Segmentos", "Segments"), segs, one("TABLESPACE_NAME")),
	)
	ts.KeyCol = "TABLESPACE_NAME"

	tgroups := v("st.tempgroups", L("Grupos de temporárias", "Temporary groups"), `select group_name, tablespace_name from dba_tablespace_groups order by 1, 2`,
		act('n', L("Pôr tablespace num grupo", "Add tablespace to group"), false, []Field{fR("TS", L("Tablespace temporária", "Temporary tablespace"), ""), fR("GROUP", L("Grupo", "Group"), "")},
			func(_ Row, x Vals) (Script, error) {
				return sqlS("ALTER TABLESPACE %s TABLESPACE GROUP %s;", IdentIn(x["TS"]), IdentIn(x["GROUP"]))
			}),
		act('x', L("Tirar do grupo", "Remove from group"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER TABLESPACE %s TABLESPACE GROUP '';", Ident(r["TABLESPACE_NAME"]))
		}),
		act('d', L("Grupo padrão do banco", "Database default group"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER DATABASE DEFAULT TEMPORARY TABLESPACE %s;", Ident(r["GROUP_NAME"]))
		}),
	)

	ctl := v("st.controlfiles", "Control files", `select name, status, is_recovery_dest_file fra, block_size, file_size_blks blocks from v$controlfile`,
		act('t', L("Backup para trace", "Backup to trace"), false, []Field{fT("FILE", L("Arquivo (vazio = trace da sessão)", "File (empty = session trace)"), "/tmp/control_obs.sql"), fB("RESETLOGS", "RESETLOGS", false)},
			func(_ Row, x Vals) (Script, error) {
				s := "ALTER DATABASE BACKUP CONTROLFILE TO TRACE"
				if f := strings.TrimSpace(x["FILE"]); f != "" {
					s += " AS " + Lit(f) + " REUSE"
				}
				if Yes(x["RESETLOGS"]) {
					s += " RESETLOGS"
				}
				return sqlS("%s;", s)
			}),
		act('b', L("Backup binário", "Binary backup"), false, []Field{fR("FILE", L("Arquivo", "File"), "/tmp/control_obs.bkp")},
			func(_ Row, x Vals) (Script, error) {
				return sqlS("ALTER DATABASE BACKUP CONTROLFILE TO %s REUSE;", Lit(x["FILE"]))
			}),
		drill('s', L("Seções (registros)", "Record sections"), v("st.ctlsections", L("Seções do control file", "Control file sections"),
			`select type, record_size, records_total, records_used, first_index, last_index from v$controlfile_record_section where nvl(:1, 'x') is not null order by type`), one("NAME")),
	)

	logMembers := v("st.logmembers", L("Membros do redo", "Redo members"), `select group#, member, type, status, is_recovery_dest_file fra
	   from v$logfile where group# = nvl(:1, group#) order by group#, member`,
		danger(act('x', L("Remover membro", "Drop member"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER DATABASE DROP LOGFILE MEMBER %s;", Lit(r["MEMBER"]))
		}), "GROUP#"),
	)
	logMembers.Args = nil1
	sizeF := fR("SIZE", L("Tamanho", "Size"), "200M")
	redo := v("st.redo", L("Grupos de redo", "Redo log groups"), `select l.group#, l.thread#, l.sequence#, round(l.bytes/1048576) mb, l.members,
	        l.archived, l.status, l.first_time,
	        (select listagg(f.member, ', ') within group (order by f.member) from v$logfile f where f.group# = l.group#) files
	   from v$log l order by l.thread#, l.group#`,
		act('s', "Switch logfile", false, nil, func(Row, Vals) (Script, error) { return sqlS("ALTER SYSTEM SWITCH LOGFILE;") }),
		act('c', "Checkpoint", false, []Field{fB("GLOBAL", "GLOBAL (RAC)", false)}, func(_ Row, x Vals) (Script, error) {
			return sqlS("ALTER SYSTEM CHECKPOINT%s;", iff(Yes(x["GLOBAL"]), " GLOBAL", ""))
		}),
		act('a', "Archive log current", false, nil, func(Row, Vals) (Script, error) { return sqlS("ALTER SYSTEM ARCHIVE LOG CURRENT;") }),
		act('n', L("Adicionar grupo", "Add group"), false, []Field{
			fT("THREAD", L("Thread (RAC)", "Thread (RAC)"), ""), fT("GROUP", L("Número do grupo (vazio = próximo)", "Group number (empty = next)"), ""),
			fT("FILES", L("Arquivos ('a','b'; vazio = OMF)", "Files ('a','b'; empty = OMF)"), ""), sizeF,
		}, func(_ Row, x Vals) (Script, error) {
			sz, err := Size("SIZE", x["SIZE"])
			if err != nil {
				return Script{}, err
			}
			s := "ALTER DATABASE ADD LOGFILE"
			if t := strings.TrimSpace(x["THREAD"]); t != "" {
				if t, err = Num("THREAD", t); err != nil {
					return Script{}, err
				}
				s += " THREAD " + t
			}
			if g := strings.TrimSpace(x["GROUP"]); g != "" {
				if g, err = Num("GROUP", g); err != nil {
					return Script{}, err
				}
				s += " GROUP " + g
			}
			if f := fileList(x["FILES"]); f != "" {
				s += " (" + f + ")"
			}
			return sqlS("%s SIZE %s;", s, sz)
		}),
		act('m', L("Adicionar membro", "Add member"), true, []Field{fR("FILE", L("Arquivo (ou +DG)", "File (or +DG)"), "")},
			func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER DATABASE ADD LOGFILE MEMBER %s TO GROUP %s;", Lit(x["FILE"]), r["GROUP#"])
			}),
		danger(act('l', L("Limpar (CLEAR LOGFILE)", "Clear logfile"), true, []Field{fB("UNARCH", "UNARCHIVED", false)},
			func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER DATABASE CLEAR %sLOGFILE GROUP %s;", iff(Yes(x["UNARCH"]), "UNARCHIVED ", ""), r["GROUP#"])
			}), "GROUP#"),
		danger(act('x', L("Remover grupo", "Drop group"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER DATABASE DROP LOGFILE GROUP %s;", r["GROUP#"])
		}), "GROUP#"),
		drill('v', L("Membros", "Members"), logMembers, one("GROUP#")),
	)
	stby := v("st.standbylogs", L("Standby logs", "Standby logs"), `select group#, thread#, sequence#, round(bytes/1048576) mb, used, archived, status from v$standby_log order by thread#, group#`,
		act('n', L("Adicionar standby log", "Add standby log"), false, []Field{fT("THREAD", "Thread", "1"), fT("FILES", L("Arquivos (vazio = OMF)", "Files (empty = OMF)"), ""), sizeF},
			func(_ Row, x Vals) (Script, error) {
				sz, err := Size("SIZE", x["SIZE"])
				if err != nil {
					return Script{}, err
				}
				s := "ALTER DATABASE ADD STANDBY LOGFILE"
				if t := strings.TrimSpace(x["THREAD"]); t != "" {
					if t, err = Num("THREAD", t); err != nil {
						return Script{}, err
					}
					s += " THREAD " + t
				}
				if f := fileList(x["FILES"]); f != "" {
					s += " (" + f + ")"
				}
				return sqlS("%s SIZE %s;", s, sz)
			}),
		danger(act('x', L("Remover", "Drop"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER DATABASE DROP STANDBY LOGFILE GROUP %s;", r["GROUP#"])
		}), "GROUP#"),
	)

	arch := v("st.archived", L("Archive logs (últimos 500)", "Archive logs (last 500)"), top(500, `select recid, thread#, sequence#, name, first_time, next_time,
	        round(blocks*block_size/1048576) mb, archived, applied, deleted, status, dest_id, first_change#
	   from v$archived_log order by recid desc`))
	archHour := v("st.archhour", L("Archive por hora (48 h)", "Archive per hour (48 h)"), `select trunc(first_time, 'HH24') hora, thread#, count(*) qtd,
	        round(sum(blocks*block_size)/1048576) mb
	   from v$archived_log where first_time > sysdate - 2 and dest_id = (select min(dest_id) from v$archived_log)
	  group by trunc(first_time, 'HH24'), thread# order by 1 desc, 2`)
	dests := v("st.archdest", L("Destinos de archive", "Archive destinations"), `select dest_id, dest_name, status, target, destination, binding, error
	   from v$archive_dest where status <> 'INACTIVE' or destination is not null order by dest_id`,
		act('s', L("Habilitar / adiar (DEFER)", "Enable / defer"), true, []Field{fC("STATE", "", "ENABLE", "ENABLE", "DEFER")},
			func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER SYSTEM SET log_archive_dest_state_%s = %s SCOPE = BOTH;", r["DEST_ID"], x["STATE"])
			}),
		act('e', L("Alterar destino", "Change destination"), true, []Field{fR("VALUE", L("Valor (ex.: LOCATION=USE_DB_RECOVERY_FILE_DEST)", "Value (e.g. LOCATION=USE_DB_RECOVERY_FILE_DEST)"), "LOCATION={DESTINATION}")},
			func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER SYSTEM SET log_archive_dest_%s = %s SCOPE = BOTH;", r["DEST_ID"], Lit(x["VALUE"]))
			}),
	)

	undo := v("st.undostat", L("Estatísticas de undo (24 h)", "Undo statistics (24 h)"), top(144, `select begin_time, end_time, undoblks, txncount, maxquerylen,
	        maxconcurrency, ssolderrcnt ora1555, nospaceerrcnt nospace, tuned_undoretention tuned_ret
	   from v$undostat order by begin_time desc`))
	undoExt := v("st.undoext", L("Extents de undo", "Undo extents"), `select tablespace_name, status, count(*) extents, round(sum(bytes)/1048576) mb
	   from dba_undo_extents group by tablespace_name, status order by 1, 2`,
		act('r', L("Alterar undo_retention", "Change undo_retention"), false, []Field{fR("SEC", L("Segundos", "Seconds"), "900")},
			func(_ Row, x Vals) (Script, error) {
				s, err := Num("SEC", x["SEC"])
				if err != nil {
					return Script{}, err
				}
				return sqlS("ALTER SYSTEM SET undo_retention = %s SCOPE = BOTH;", s)
			}),
	)
	rbs := v("st.rbs", "Rollback segments", `select segment_name, owner, tablespace_name, status, segment_id, file_id from dba_rollback_segs order by segment_id`,
		act('o', "Online", true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER ROLLBACK SEGMENT %s ONLINE;", Ident(r["SEGMENT_NAME"]))
		}),
		act('f', "Offline", true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER ROLLBACK SEGMENT %s OFFLINE;", Ident(r["SEGMENT_NAME"]))
		}),
	)
	rbs.Help = L("Online/offline só valem com undo_management = MANUAL.", "Online/offline only apply with undo_management = MANUAL.")

	rp := v("st.restorepoints", "Restore points", `select name, scn, time, guarantee_flashback_database guarantee,
	        round(storage_size/1048576) mb, database_incarnation# incarnation, preserved
	   from v$restore_point order by scn`,
		act('n', L("Criar", "Create"), false, []Field{fR("NAME", L("Nome", "Name"), "ANTES_DA_MUDANCA"), fB("G", "GUARANTEE FLASHBACK DATABASE", false)},
			func(_ Row, x Vals) (Script, error) {
				return sqlS("CREATE RESTORE POINT %s%s;", IdentIn(x["NAME"]), iff(Yes(x["G"]), " GUARANTEE FLASHBACK DATABASE", ""))
			}),
		danger(act('x', L("Remover", "Drop"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("DROP RESTORE POINT %s;", Ident(r["NAME"]))
		}), "NAME"),
		danger(act('F', L("Flashback do banco para este ponto (RMAN)", "Flashback database to this point (RMAN)"), true, nil,
			func(r Row, _ Vals) (Script, error) {
				return rmanS("SHUTDOWN IMMEDIATE;\nSTARTUP MOUNT;\nFLASHBACK DATABASE TO RESTORE POINT %s;\nALTER DATABASE OPEN RESETLOGS;", Ident(r["NAME"]))
			}), "NAME"),
	)

	bin := v("st.recyclebin", L("Lixeira", "Recycle bin"), `select owner, object_name, original_name, type, ts_name, droptime, can_undrop, can_purge, space
	   from dba_recyclebin order by droptime desc`,
		act('r', L("Restaurar (FLASHBACK TO BEFORE DROP)", "Restore (FLASHBACK TO BEFORE DROP)"), true, []Field{fT("NEW", L("Renomear para (opcional)", "Rename to (optional)"), "")},
			func(r Row, x Vals) (Script, error) {
				if r["TYPE"] != "TABLE" {
					return Script{}, fmt.Errorf(L("só tabelas voltam da lixeira (%s volta junto com a tabela)", "only tables can be restored (%s comes back with its table)"), r["TYPE"])
				}
				s := "FLASHBACK TABLE " + Q2(r["OWNER"], r["OBJECT_NAME"]) + " TO BEFORE DROP"
				if n := strings.TrimSpace(x["NEW"]); n != "" {
					s += " RENAME TO " + IdentIn(n)
				}
				return sqlS("%s;", s)
			}),
		danger(act('p', L("Expurgar objeto", "Purge object"), true, nil, func(r Row, _ Vals) (Script, error) {
			kind := "TABLE"
			if strings.HasPrefix(r["TYPE"], "INDEX") {
				kind = "INDEX"
			}
			return sqlS("PURGE %s %s;", kind, Q2(r["OWNER"], r["OBJECT_NAME"]))
		}), "ORIGINAL_NAME"),
		danger(act('P', L("Esvaziar a lixeira do banco (DBA_RECYCLEBIN)", "Empty the database recycle bin"), false, nil,
			func(Row, Vals) (Script, error) { return sqlS("PURGE DBA_RECYCLEBIN;") }), ""),
		danger(act('t', L("Esvaziar por tablespace", "Purge by tablespace"), true, []Field{fT("USER", L("Só do usuário (opcional)", "Only for user (optional)"), "")},
			func(r Row, x Vals) (Script, error) {
				s := "PURGE TABLESPACE " + Ident(r["TS_NAME"])
				if u := strings.TrimSpace(x["USER"]); u != "" {
					s += " USER " + IdentIn(u)
				}
				return sqlS("%s;", s)
			}), "TS_NAME"),
	)

	dirs := v("st.directories", L("Diretórios", "Directories"), `select owner, directory_name, directory_path from dba_directories order by directory_name`,
		act('n', L("Criar / alterar", "Create / replace"), false, []Field{fR("NAME", L("Nome", "Name"), ""), fR("PATH", L("Caminho no servidor", "Server path"), "")},
			func(_ Row, x Vals) (Script, error) {
				return sqlS("CREATE OR REPLACE DIRECTORY %s AS %s;", IdentIn(x["NAME"]), Lit(x["PATH"]))
			}),
		act('g', L("Conceder acesso", "Grant access"), true, []Field{fR("TO", L("Para (usuário/role)", "To (user/role)"), ""), fC("PRIV", L("Privilégio", "Privilege"), "READ, WRITE", "READ, WRITE", "READ", "WRITE", "EXECUTE")},
			func(r Row, x Vals) (Script, error) {
				return sqlS("GRANT %s ON DIRECTORY %s TO %s;", x["PRIV"], Ident(r["DIRECTORY_NAME"]), IdentIn(x["TO"]))
			}),
		danger(act('x', L("Remover", "Drop"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("DROP DIRECTORY %s;", Ident(r["DIRECTORY_NAME"]))
		}), "DIRECTORY_NAME"),
		drill('v', L("Quem tem acesso", "Who has access"), v("st.dirprivs", L("Acesso ao diretório", "Directory access"),
			`select grantee, privilege, grantable from dba_tab_privs where type = 'DIRECTORY' and table_name = :1 order by 1, 2`), one("DIRECTORY_NAME")),
	)

	tsSets := v("st.tssets", "Tablespace sets", `select * from dba_tablespace_sets`)
	tsSets.Help = L("Só existe em bancos com sharding.", "Only exists on sharded databases.")
	tsSets.Requires = "DBA_TABLESPACE_SETS"
	tsSets.Missing = tsSets.Help

	return &Node{Title: L("Armazenamento", "Storage"), Children: []*Node{
		{Title: "Tablespaces", View: ts},
		{Title: "Datafiles", View: dfs},
		{Title: L("Grupos de temporárias", "Temporary groups"), View: tgroups},
		{Title: "Tablespace sets", View: tsSets},
		{Title: "Control files", View: ctl},
		{Title: L("Grupos de redo", "Redo log groups"), View: redo},
		{Title: L("Standby logs", "Standby logs"), View: stby},
		{Title: "Archive logs", View: arch},
		{Title: L("Archive por hora", "Archive per hour"), View: archHour},
		{Title: L("Destinos de archive", "Archive destinations"), View: dests},
		{Title: "Undo", Children: []*Node{
			{Title: L("Estatísticas", "Statistics"), View: undo},
			{Title: "Extents", View: undoExt},
			{Title: "Rollback segments", View: rbs},
		}},
		{Title: "Restore points", View: rp},
		{Title: L("Lixeira", "Recycle bin"), View: bin},
		{Title: L("Diretórios", "Directories"), View: dirs},
		{Title: L("Maiores segmentos", "Largest segments"), View: segs},
	}}
}

// fileList aceita a lista com ou sem aspas: a, 'b', +DATA viram 'a', 'b', '+DATA'.
func fileList(s string) string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if len(f) > 1 && strings.HasPrefix(f, "'") && strings.HasSuffix(f, "'") {
			f = strings.ReplaceAll(f[1:len(f)-1], "''", "'")
		}
		out = append(out, Lit(f))
	}
	return strings.Join(out, ", ")
}
