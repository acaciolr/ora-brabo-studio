// Confere o catálogo inteiro: ids únicos, atalhos sem conflito, toda ação monta
// um comando com os valores padrão (ou erra de forma explicada) e nada entra em
// pânico. by • DBA BRABO
package catalog

import (
	"strings"
	"testing"
)

func sampleRow() Row {
	return Row{"NAME": "X", "VALUE": "10", "TYPE": "3", "TABLESPACE_NAME": "USERS", "FILE_NAME": "/u01/x.dbf", "FILE_ID": "4",
		"MB": "100", "CONTENTS": "PERMANENT", "OWNER": "HR", "SEGMENT_NAME": "EMP", "SEGMENT_TYPE": "TABLE", "GROUP#": "1",
		"MEMBER": "/u01/redo1.log", "DEST_ID": "2", "DESTINATION": "/arch", "USERNAME": "HR", "DEFAULT_TABLESPACE": "USERS",
		"TEMPORARY_TABLESPACE": "TEMP", "PROFILE": "DEFAULT", "GRANTEE": "HR", "GRANTED_ROLE": "CONNECT", "PRIVILEGE": "CREATE SESSION",
		"OBJECT_NAME": "EMP", "ROLE": "R1", "RESOURCE_NAME": "IDLE_TIME", "LIMIT": "30", "POLICY_NAME": "P1", "AUDIT_OPTION": "TABLE",
		"USER_NAME": "", "BS_KEY": "12", "SESSION_KEY": "3", "SNAP_ID": "100", "PREV": "99", "BASELINE_NAME": "B1", "START_SNAP_ID": "90",
		"END_SNAP_ID": "95", "TEMPLATE_NAME": "T1", "TASK_NAME": "ADDM_1", "SQL_ID": "abc123", "SQL_EXEC_ID": "16777216", "PHV": "999",
		"SQL_HANDLE": "SQL_1", "PLAN_NAME": "PL1", "TABLE_NAME": "EMP", "PLAN": "P1", "GROUP_OR_SUBPLAN": "G1", "CONSUMER_GROUP": "G1",
		"ATTRIBUTE": "ORACLE_USER", "PLUGGABLE_DATABASE": "PDB1", "JOB_NAME": "J1", "OWNER_NAME": "SYSTEM", "STATE": "NOT RUNNING",
		"DEGREE": "1", "REPEAT_INTERVAL": "FREQ=DAILY", "PROGRAM_NAME": "PR", "SCHEDULE_NAME": "S1", "WINDOW_NAME": "W1",
		"RESOURCE_PLAN": "", "CHAIN_NAME": "C1", "CREDENTIAL_NAME": "CR", "JOB": "5", "CLIENT_NAME": "auto optimizer stats collection",
		"SID": "10", "SERIAL#": "20", "INST_ID": "1", "OBJECT_TYPE": "PACKAGE BODY", "DIRECTORY_NAME": "D1", "TS_NAME": "USERS",
		"FILE#": "4", "BLOCK#": "12", "NAME_X": "", "PARTITION_NAME": ""}
}

func defaults(a *Action, r Row) Vals {
	x := Vals{}
	for _, f := range a.Fields {
		x[f.Name] = Fill(f.Default, r)
		if x[f.Name] == "" && f.Required {
			switch f.Kind {
			case Password:
				x[f.Name] = "Senha#123"
			default:
				x[f.Name] = "1"
			}
		}
	}
	return x
}

func TestCatalogo(t *testing.T) {
	root := DBA()
	ids := map[string]string{}
	nViews, nActs := 0, 0
	Walk(root, func(path []string, n *Node) {
		if n.View == nil {
			if len(n.Children) == 0 {
				t.Errorf("nó sem tela e sem filhos: %v", path)
			}
			return
		}
		vw := n.View
		nViews++
		if p, ok := ids[vw.ID]; ok && p != strings.Join(path, "/") && vw.SQL != "" {
			// a mesma tela pode aparecer em dois lugares (ex.: datafiles)
			_ = p
		}
		ids[vw.ID] = strings.Join(path, "/")
		if vw.SQL == "" && vw.Special == "" {
			t.Errorf("%s: tela sem SQL", vw.ID)
		}
		var check func(v *View, depth int)
		check = func(v *View, depth int) {
			keys := map[rune]string{}
			for _, a := range v.Actions {
				if a.Key == 0 {
					t.Errorf("%s: ação %q sem atalho", v.ID, a.Name)
				}
				if prev, ok := keys[a.Key]; ok {
					t.Errorf("%s: atalho %q repetido (%s / %s)", v.ID, a.Key, prev, a.Name)
				}
				keys[a.Key] = a.Name
			}
			for _, a := range v.Actions {
				nActs++
				if a.Drill != nil {
					if a.DrillArgs == nil {
						t.Errorf("%s: drill sem argumentos", v.ID)
					}
					if depth < 2 {
						check(a.Drill, depth+1)
					}
					continue
				}
				if a.Build == nil {
					t.Errorf("%s: ação %q sem Build", v.ID, a.Name)
					continue
				}
				r := sampleRow()
				func() {
					defer func() {
						if p := recover(); p != nil {
							t.Errorf("%s / %q: pânico %v", v.ID, a.Name, p)
						}
					}()
					s, err := a.Build(r, defaults(a, r))
					if err != nil {
						t.Logf("%s / %q: erro esperado com valores de amostra: %v", v.ID, a.Name, err)
						return
					}
					if strings.TrimSpace(s.Text) == "" {
						t.Errorf("%s / %q: comando vazio", v.ID, a.Name)
					}
					if strings.Contains(s.Text, "%!") {
						t.Errorf("%s / %q: formatação quebrada: %s", v.ID, a.Name, s.Text)
					}
					if strings.Contains(s.Text, "{") && s.Kind != RMAN {
						t.Errorf("%s / %q: sobrou {COLUNA}: %s", v.ID, a.Name, s.Text)
					}
				}()
			}
		}
		check(vw, 0)
	})
	t.Logf("%d telas, %d ações", nViews, nActs)
	if nViews < 80 {
		t.Errorf("poucas telas: %d", nViews)
	}
}

func TestIdent(t *testing.T) {
	cases := map[string]string{"EMP": "EMP", "emp": `"emp"`, "BIN$abc==$0": `"BIN$abc==$0"`, `A"B`: `"A""B"`}
	for in, want := range cases {
		if got := Ident(in); got != want {
			t.Errorf("Ident(%q) = %q, quero %q", in, got, want)
		}
	}
	if IdentIn("emp") != "EMP" || IdentIn(`"Emp"`) != `"Emp"` {
		t.Error("IdentIn")
	}
}
