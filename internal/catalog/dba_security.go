// Aba DBA · Segurança: usuários, roles, profiles, privilégios e auditoria
// (unificada e tradicional). by • DBA BRABO
package catalog

import (
	"errors"
	"fmt"
	"strings"
)

func pwd(p string) string { return `"` + strings.ReplaceAll(p, `"`, "") + `"` }

func quotaClause(q, ts string) (string, error) {
	q = strings.ToUpper(strings.TrimSpace(q))
	if q == "" || ts == "" {
		return "", nil
	}
	if q != "UNLIMITED" {
		var err error
		if q, err = Size("QUOTA", q); err != nil {
			return "", err
		}
	}
	return " QUOTA " + q + " ON " + IdentIn(ts), nil
}

// privilégios concedidos a um usuário/role, com revoke
func rolePrivsView() *View {
	vw := v("sec.roleprivs", L("Roles concedidas", "Granted roles"), `select grantee, granted_role, admin_option, default_role, common, inherited
	   from dba_role_privs where grantee = nvl(:1, grantee) order by grantee, granted_role`,
		act('v', "Revoke", true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("REVOKE %s FROM %s;", Ident(r["GRANTED_ROLE"]), Ident(r["GRANTEE"]))
		}),
		act('d', L("Default role ON/OFF", "Default role ON/OFF"), true, []Field{fC("MODE", "", "ALL EXCEPT", "ALL EXCEPT", "ONLY THIS")},
			func(r Row, x Vals) (Script, error) {
				if x["MODE"] == "ONLY THIS" {
					return sqlS("ALTER USER %s DEFAULT ROLE %s;", Ident(r["GRANTEE"]), Ident(r["GRANTED_ROLE"]))
				}
				return sqlS("ALTER USER %s DEFAULT ROLE ALL EXCEPT %s;", Ident(r["GRANTEE"]), Ident(r["GRANTED_ROLE"]))
			}),
	)
	vw.Args = nil1
	return vw
}

func sysPrivsView() *View {
	vw := v("sec.sysprivs", L("Privilégios de sistema", "System privileges"), `select grantee, privilege, admin_option, common, inherited
	   from dba_sys_privs where grantee = nvl(:1, grantee) order by grantee, privilege`,
		act('v', "Revoke", true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("REVOKE %s FROM %s;", r["PRIVILEGE"], Ident(r["GRANTEE"]))
		}),
	)
	vw.Args = nil1
	return vw
}

func tabPrivsView() *View {
	vw := v("sec.tabprivs", L("Privilégios em objetos", "Object privileges"), `select grantee, owner, table_name object_name, type, privilege, grantable, grantor, common, inherited
	   from dba_tab_privs where grantee = nvl(:1, grantee) order by grantee, owner, table_name, privilege`,
		act('v', "Revoke", true, nil, func(r Row, _ Vals) (Script, error) {
			on := Q2(r["OWNER"], r["OBJECT_NAME"])
			if r["TYPE"] == "DIRECTORY" {
				on = "DIRECTORY " + Ident(r["OBJECT_NAME"])
			}
			return sqlS("REVOKE %s ON %s FROM %s;", r["PRIVILEGE"], on, Ident(r["GRANTEE"]))
		}),
	)
	vw.Args = nil1
	return vw
}

func grantRoleAct(k rune, col string) *Action {
	return act(k, L("Conceder role", "Grant role"), true, []Field{fR("ROLE", "Role", "CONNECT"), fB("ADMIN", "WITH ADMIN OPTION", false)},
		func(r Row, x Vals) (Script, error) {
			return sqlS("GRANT %s TO %s%s;", identList(x["ROLE"]), Ident(r[col]), iff(Yes(x["ADMIN"]), " WITH ADMIN OPTION", ""))
		})
}

func grantSysAct(k rune, col string) *Action {
	return act(k, L("Conceder privilégio de sistema", "Grant system privilege"), true, []Field{
		fR("PRIV", L("Privilégio(s) (ex.: CREATE SESSION, CREATE TABLE)", "Privilege(s) (e.g. CREATE SESSION, CREATE TABLE)"), "CREATE SESSION"),
		fB("ADMIN", "WITH ADMIN OPTION", false),
	}, func(r Row, x Vals) (Script, error) {
		return sqlS("GRANT %s TO %s%s;", strings.ToUpper(strings.TrimSpace(x["PRIV"])), Ident(r[col]), iff(Yes(x["ADMIN"]), " WITH ADMIN OPTION", ""))
	})
}

func grantObjAct(k rune, col string) *Action {
	return act(k, L("Conceder privilégio em objeto", "Grant object privilege"), true, []Field{
		fR("PRIV", L("Privilégio(s) (SELECT, INSERT, EXECUTE...)", "Privilege(s) (SELECT, INSERT, EXECUTE...)"), "SELECT"),
		fR("OBJ", L("Objeto (OWNER.NOME)", "Object (OWNER.NAME)"), ""),
		fB("GRANT", "WITH GRANT OPTION", false),
	}, func(r Row, x Vals) (Script, error) {
		parts := strings.SplitN(strings.TrimSpace(x["OBJ"]), ".", 2)
		obj := IdentIn(parts[0])
		if len(parts) == 2 {
			obj += "." + IdentIn(parts[1])
		}
		return sqlS("GRANT %s ON %s TO %s%s;", strings.ToUpper(strings.TrimSpace(x["PRIV"])), obj, Ident(r[col]), iff(Yes(x["GRANT"]), " WITH GRANT OPTION", ""))
	})
}

var profileLimits = []string{"FAILED_LOGIN_ATTEMPTS", "PASSWORD_LIFE_TIME", "PASSWORD_LOCK_TIME", "PASSWORD_GRACE_TIME",
	"PASSWORD_REUSE_MAX", "PASSWORD_REUSE_TIME", "PASSWORD_VERIFY_FUNCTION", "INACTIVE_ACCOUNT_TIME",
	"SESSIONS_PER_USER", "IDLE_TIME", "CONNECT_TIME", "CPU_PER_SESSION", "LOGICAL_READS_PER_SESSION", "PRIVATE_SGA"}

func securityNode() *Node {
	rp, sp, tp := rolePrivsView(), sysPrivsView(), tabPrivsView()
	quotas := v("sec.quotas", L("Cotas", "Quotas"), `select username, tablespace_name, round(bytes/1048576) used_mb,
	        decode(max_bytes, -1, 'UNLIMITED', round(max_bytes/1048576)) max_mb
	   from dba_ts_quotas where username = nvl(:1, username) order by 1, 2`,
		act('q', L("Alterar cota", "Change quota"), true, []Field{fR("QUOTA", L("Cota (UNLIMITED, 100M, 0)", "Quota (UNLIMITED, 100M, 0)"), "UNLIMITED")},
			func(r Row, x Vals) (Script, error) {
				q, err := quotaClause(x["QUOTA"], r["TABLESPACE_NAME"])
				if err != nil {
					return Script{}, err
				}
				return sqlS("ALTER USER %s%s;", Ident(r["USERNAME"]), q)
			}),
	)
	quotas.Args = nil1

	userActs := []*Action{
		act('n', L("Criar usuário", "Create user"), false, []Field{
			fR("USER", L("Usuário", "User"), ""), fP("PWD", L("Senha", "Password")),
			fT("DEFTS", L("Tablespace padrão", "Default tablespace"), "USERS"), fT("TMPTS", L("Temporária", "Temporary"), "TEMP"),
			fT("QUOTA", L("Cota na padrão (UNLIMITED, 100M, vazio)", "Quota on default (UNLIMITED, 100M, empty)"), "UNLIMITED"),
			fT("PROFILE", "Profile", "DEFAULT"),
			fT("ROLES", L("Roles (vírgula)", "Roles (comma)"), ""),
			fB("SESSION", L("Conceder CREATE SESSION", "Grant CREATE SESSION"), true),
			fB("EXPIRE", L("Senha expirada (troca no 1º login)", "Expired password (change at first login)"), false),
			fB("LOCK", L("Conta travada", "Account locked"), false),
		}, func(_ Row, x Vals) (Script, error) {
			u := IdentIn(x["USER"])
			s := "CREATE USER " + u + " IDENTIFIED BY " + pwd(x["PWD"])
			if d := strings.TrimSpace(x["DEFTS"]); d != "" {
				s += "\n  DEFAULT TABLESPACE " + IdentIn(d)
			}
			if t := strings.TrimSpace(x["TMPTS"]); t != "" {
				s += "\n  TEMPORARY TABLESPACE " + IdentIn(t)
			}
			q, err := quotaClause(x["QUOTA"], x["DEFTS"])
			if err != nil {
				return Script{}, err
			}
			if q != "" {
				s += "\n " + q
			}
			if p := strings.TrimSpace(x["PROFILE"]); p != "" {
				s += "\n  PROFILE " + IdentIn(p)
			}
			if Yes(x["EXPIRE"]) {
				s += "\n  PASSWORD EXPIRE"
			}
			if Yes(x["LOCK"]) {
				s += "\n  ACCOUNT LOCK"
			}
			s += ";"
			if Yes(x["SESSION"]) {
				s += "\nGRANT CREATE SESSION TO " + u + ";"
			}
			if r := strings.TrimSpace(x["ROLES"]); r != "" {
				s += "\nGRANT " + identList(r) + " TO " + u + ";"
			}
			return sqlS("%s", s)
		}),
		act('e', L("Editar", "Edit"), true, []Field{
			fT("PWD", L("Nova senha (vazio = mantém)", "New password (empty = keep)"), ""),
			fT("DEFTS", L("Tablespace padrão", "Default tablespace"), "{DEFAULT_TABLESPACE}"),
			fT("TMPTS", L("Temporária", "Temporary"), "{TEMPORARY_TABLESPACE}"), fT("PROFILE", "Profile", "{PROFILE}"),
		}, func(r Row, x Vals) (Script, error) {
			s := "ALTER USER " + Ident(r["USERNAME"])
			n := 0
			if p := x["PWD"]; p != "" {
				s += " IDENTIFIED BY " + pwd(p)
				n++
			}
			if d := strings.TrimSpace(x["DEFTS"]); d != "" && d != r["DEFAULT_TABLESPACE"] {
				s += "\n  DEFAULT TABLESPACE " + IdentIn(d)
				n++
			}
			if t := strings.TrimSpace(x["TMPTS"]); t != "" && t != r["TEMPORARY_TABLESPACE"] {
				s += "\n  TEMPORARY TABLESPACE " + IdentIn(t)
				n++
			}
			if p := strings.TrimSpace(x["PROFILE"]); p != "" && p != r["PROFILE"] {
				s += "\n  PROFILE " + IdentIn(p)
				n++
			}
			if n == 0 {
				return Script{}, errors.New(L("nada mudou", "nothing changed"))
			}
			return sqlS("%s;", s)
		}),
		act('p', L("Trocar senha", "Change password"), true, []Field{fP("PWD", L("Nova senha", "New password")), fB("EXPIRE", L("Expirar (troca no próximo login)", "Expire (change at next login)"), false)},
			func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER USER %s IDENTIFIED BY %s%s;", Ident(r["USERNAME"]), pwd(x["PWD"]), iff(Yes(x["EXPIRE"]), " PASSWORD EXPIRE", ""))
			}),
		act('l', L("Travar", "Lock"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER USER %s ACCOUNT LOCK;", Ident(r["USERNAME"]))
		}),
		act('u', L("Destravar", "Unlock"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER USER %s ACCOUNT UNLOCK;", Ident(r["USERNAME"]))
		}),
		act('x', L("Expirar senha", "Expire password"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("ALTER USER %s PASSWORD EXPIRE;", Ident(r["USERNAME"]))
		}),
		act('q', L("Cota", "Quota"), true, []Field{fR("TS", "Tablespace", "{DEFAULT_TABLESPACE}"), fR("QUOTA", L("Cota (UNLIMITED, 100M, 0)", "Quota (UNLIMITED, 100M, 0)"), "UNLIMITED")},
			func(r Row, x Vals) (Script, error) {
				q, err := quotaClause(x["QUOTA"], x["TS"])
				if err != nil {
					return Script{}, err
				}
				return sqlS("ALTER USER %s%s;", Ident(r["USERNAME"]), q)
			}),
		grantRoleAct('g', "USERNAME"), grantSysAct('s', "USERNAME"), grantObjAct('o', "USERNAME"),
		danger(act('X', L("Remover usuário", "Drop user"), true, []Field{fB("CASCADE", L("CASCADE (apaga os objetos)", "CASCADE (drops objects)"), false)},
			func(r Row, x Vals) (Script, error) {
				return sqlS("DROP USER %s%s;", Ident(r["USERNAME"]), iff(Yes(x["CASCADE"]), " CASCADE", ""))
			}), "USERNAME"),
		rep('D', "DDL", true, nil, func(r Row, _ Vals) (Script, error) {
			return reportQ(`select dbms_metadata.get_ddl('USER', %s) from dual`, Lit(r["USERNAME"]))
		}),
		drill('r', L("Roles", "Roles"), rp, one("USERNAME")),
		drill('y', L("Privilégios de sistema", "System privileges"), sp, one("USERNAME")),
		drill('b', L("Privilégios em objetos", "Object privileges"), tp, one("USERNAME")),
		drill('z', L("Cotas", "Quotas"), quotas, one("USERNAME")),
	}
	usersSQL := `select username, account_status, lock_date, expiry_date, default_tablespace, temporary_tablespace,
	        profile, created, authentication_type auth, common, oracle_maintained, last_login
	   from dba_users %s order by username`
	users := v("sec.users", L("Usuários", "Users"), fmt.Sprintf(usersSQL, ""), userActs...)
	users.KeyCol = "USERNAME"
	appUsers := v("sec.users.app", L("Usuários (sem os da Oracle)", "Users (non-Oracle)"), fmt.Sprintf(usersSQL, "where oracle_maintained = 'N'"), userActs...)
	appUsers.KeyCol = "USERNAME"

	members := v("sec.rolemembers", L("Quem tem a role", "Role grantees"), `select granted_role, grantee, admin_option, default_role
	   from dba_role_privs where granted_role = :1 order by grantee`,
		act('v', "Revoke", true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("REVOKE %s FROM %s;", Ident(r["GRANTED_ROLE"]), Ident(r["GRANTEE"]))
		}),
	)
	roles := v("sec.roles", "Roles", `select role, password_required, authentication_type auth, common, oracle_maintained from dba_roles order by role`,
		act('n', L("Criar role", "Create role"), false, []Field{fR("ROLE", "Role", ""), fT("PWD", L("Senha (vazio = NOT IDENTIFIED)", "Password (empty = NOT IDENTIFIED)"), "")},
			func(_ Row, x Vals) (Script, error) {
				s := "CREATE ROLE " + IdentIn(x["ROLE"])
				if p := x["PWD"]; p != "" {
					s += " IDENTIFIED BY " + pwd(p)
				}
				return sqlS("%s;", s)
			}),
		act('t', L("Conceder a (usuário/role)", "Grant to (user/role)"), true, []Field{fR("TO", L("Para", "To"), ""), fB("ADMIN", "WITH ADMIN OPTION", false)},
			func(r Row, x Vals) (Script, error) {
				return sqlS("GRANT %s TO %s%s;", Ident(r["ROLE"]), identList(x["TO"]), iff(Yes(x["ADMIN"]), " WITH ADMIN OPTION", ""))
			}),
		grantRoleAct('g', "ROLE"), grantSysAct('s', "ROLE"), grantObjAct('o', "ROLE"),
		danger(act('X', L("Remover role", "Drop role"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("DROP ROLE %s;", Ident(r["ROLE"]))
		}), "ROLE"),
		drill('m', L("Quem tem", "Grantees"), members, one("ROLE")),
		drill('r', L("Roles dentro", "Nested roles"), rp, one("ROLE")),
		drill('y', L("Privilégios de sistema", "System privileges"), sp, one("ROLE")),
		drill('b', L("Privilégios em objetos", "Object privileges"), tp, one("ROLE")),
	)
	roles.KeyCol = "ROLE"

	var limFields []Field
	for _, l := range profileLimits {
		limFields = append(limFields, fT(l, l, ""))
	}
	profBuild := func(head string, x Vals) (Script, error) {
		var lim []string
		for _, l := range profileLimits {
			if s := strings.TrimSpace(x[l]); s != "" {
				lim = append(lim, l+" "+s)
			}
		}
		if len(lim) == 0 {
			return Script{}, errors.New(L("preencha ao menos um limite", "fill at least one limit"))
		}
		return sqlS("%s LIMIT\n  %s;", head, strings.Join(lim, "\n  "))
	}
	profiles := v("sec.profiles", "Profiles", `select profile, resource_name, resource_type, limit, common from dba_profiles order by profile, resource_type, resource_name`,
		act('n', L("Criar profile", "Create profile"), false, append([]Field{fR("NAME", L("Nome", "Name"), "")}, limFields...),
			func(_ Row, x Vals) (Script, error) { return profBuild("CREATE PROFILE "+IdentIn(x["NAME"]), x) }),
		act('e', L("Alterar este limite", "Change this limit"), true, []Field{fR("VALUE", L("Valor (UNLIMITED, DEFAULT, número, função)", "Value (UNLIMITED, DEFAULT, number, function)"), "{LIMIT}")},
			func(r Row, x Vals) (Script, error) {
				return sqlS("ALTER PROFILE %s LIMIT %s %s;", Ident(r["PROFILE"]), r["RESOURCE_NAME"], strings.TrimSpace(x["VALUE"]))
			}),
		act('a', L("Alterar vários limites", "Change several limits"), true, limFields,
			func(r Row, x Vals) (Script, error) { return profBuild("ALTER PROFILE "+Ident(r["PROFILE"]), x) }),
		danger(act('X', L("Remover profile", "Drop profile"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("DROP PROFILE %s CASCADE;", Ident(r["PROFILE"]))
		}), "PROFILE"),
		drill('u', L("Usuários com o profile", "Users with profile"), v("sec.profusers", L("Usuários do profile", "Profile users"),
			`select username, account_status, profile from dba_users where profile = :1 order by 1`), one("PROFILE")),
	)

	polOpts := v("sec.policyopts", L("Opções da política", "Policy options"), `select policy_name, audit_option, audit_option_type, object_schema, object_name, object_type
	   from audit_unified_policies where policy_name = :1 order by audit_option_type, audit_option`)
	policies := v("sec.policies", L("Políticas de auditoria (unificada)", "Audit policies (unified)"), `select p.policy_name, p.audit_condition, p.condition_eval_opt, count(*) options,
	        case when exists (select 1 from audit_unified_enabled_policies e where e.policy_name = p.policy_name) then 'YES' else 'NO' end enabled
	   from audit_unified_policies p group by p.policy_name, p.audit_condition, p.condition_eval_opt order by 1`,
		act('n', L("Criar política", "Create policy"), false, []Field{
			fR("NAME", L("Nome", "Name"), ""),
			fT("PRIVS", L("Privilégios (ex.: CREATE ANY TABLE)", "Privileges (e.g. CREATE ANY TABLE)"), ""),
			fT("ACTIONS", L("Ações (ex.: DROP TABLE, SELECT ON HR.EMP)", "Actions (e.g. DROP TABLE, SELECT ON HR.EMP)"), ""),
			fT("ROLES", "Roles", ""), fT("WHEN", L("Condição WHEN (opcional)", "WHEN condition (optional)"), ""),
			fC("EVAL", L("Avaliar", "Evaluate"), "SESSION", "STATEMENT", "SESSION", "INSTANCE"),
		}, func(_ Row, x Vals) (Script, error) {
			s := "CREATE AUDIT POLICY " + IdentIn(x["NAME"])
			n := 0
			for _, k := range []string{"PRIVS", "ACTIONS", "ROLES"} {
				if t := strings.TrimSpace(x[k]); t != "" {
					s += "\n  " + map[string]string{"PRIVS": "PRIVILEGES", "ACTIONS": "ACTIONS", "ROLES": "ROLES"}[k] + " " + t
					n++
				}
			}
			if n == 0 {
				return Script{}, errors.New(L("informe privilégios, ações ou roles", "fill privileges, actions or roles"))
			}
			if w := strings.TrimSpace(x["WHEN"]); w != "" {
				s += "\n  WHEN " + Lit(w) + " EVALUATE PER " + x["EVAL"]
			}
			return sqlS("%s;", s)
		}),
		act('e', L("Habilitar (AUDIT POLICY)", "Enable (AUDIT POLICY)"), true, []Field{
			fC("WHO", L("Para", "For"), "ALL", "ALL", "BY", "EXCEPT"), fT("USERS", L("Usuários (BY/EXCEPT)", "Users (BY/EXCEPT)"), ""),
			fC("WHEN", L("Quando", "When"), "ALWAYS", "ALWAYS", "WHENEVER SUCCESSFUL", "WHENEVER NOT SUCCESSFUL"),
		}, func(r Row, x Vals) (Script, error) {
			s := "AUDIT POLICY " + Ident(r["POLICY_NAME"])
			if x["WHO"] != "ALL" {
				if strings.TrimSpace(x["USERS"]) == "" {
					return Script{}, errors.New(L("informe os usuários", "fill the users"))
				}
				s += " " + x["WHO"] + " " + identList(x["USERS"])
			}
			if x["WHEN"] != "ALWAYS" {
				s += " " + x["WHEN"]
			}
			return sqlS("%s;", s)
		}),
		act('d', L("Desabilitar (NOAUDIT POLICY)", "Disable (NOAUDIT POLICY)"), true, []Field{fT("USERS", L("Só para (usuários, opcional)", "Only for (users, optional)"), "")},
			func(r Row, x Vals) (Script, error) {
				s := "NOAUDIT POLICY " + Ident(r["POLICY_NAME"])
				if u := strings.TrimSpace(x["USERS"]); u != "" {
					s += " BY " + identList(u)
				}
				return sqlS("%s;", s)
			}),
		danger(act('X', L("Remover política", "Drop policy"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("DROP AUDIT POLICY %s;", Ident(r["POLICY_NAME"]))
		}), "POLICY_NAME"),
		drill('v', L("Opções", "Options"), polOpts, one("POLICY_NAME")),
	)
	policies.KeyCol = "POLICY_NAME"
	enabled := v("sec.enabledpol", L("Políticas habilitadas", "Enabled policies"), `select * from audit_unified_enabled_policies`)

	trail := v("sec.trail", L("Trilha unificada (últimos 500)", "Unified trail (last 500)"), top(500, `select event_timestamp, dbusername, action_name, object_schema, object_name,
	        return_code, unified_audit_policies, os_username, userhost, client_program_name, sql_text
	   from unified_audit_trail order by event_timestamp desc`),
		act('p', L("Expurgar a trilha", "Purge the trail"), false, []Field{fT("BEFORE", L("Antes de (dias atrás; vazio = tudo)", "Older than (days ago; empty = all)"), "90")},
			func(_ Row, x Vals) (Script, error) {
				d := strings.TrimSpace(x["BEFORE"])
				if d == "" {
					return sqlS("%s", plsql("DBMS_AUDIT_MGMT.CLEAN_AUDIT_TRAIL(audit_trail_type => DBMS_AUDIT_MGMT.AUDIT_TRAIL_UNIFIED, use_last_arch_timestamp => FALSE);"))
				}
				n, err := Num("BEFORE", d)
				if err != nil {
					return Script{}, err
				}
				return sqlS("%s", plsql(fmt.Sprintf("DBMS_AUDIT_MGMT.SET_LAST_ARCHIVE_TIMESTAMP(audit_trail_type => DBMS_AUDIT_MGMT.AUDIT_TRAIL_UNIFIED, last_archive_time => SYSTIMESTAMP - %s);\n  DBMS_AUDIT_MGMT.CLEAN_AUDIT_TRAIL(audit_trail_type => DBMS_AUDIT_MGMT.AUDIT_TRAIL_UNIFIED, use_last_arch_timestamp => TRUE);", n)))
			}),
	)

	auditOpt := func(kind string) []Field {
		return []Field{
			fR("OPT", L(kind+" (ex.: CREATE TABLE, SESSION)", kind+" (e.g. CREATE TABLE, SESSION)"), ""),
			fT("BY", L("Por usuários (opcional)", "By users (optional)"), ""),
			fC("MODE", "BY", "BY ACCESS", "BY ACCESS", "BY SESSION"),
			fC("WHEN", L("Quando", "When"), "ALWAYS", "ALWAYS", "WHENEVER SUCCESSFUL", "WHENEVER NOT SUCCESSFUL"),
		}
	}
	auditBuild := func(verb string, x Vals) (Script, error) {
		s := verb + " " + strings.ToUpper(strings.TrimSpace(x["OPT"]))
		if b := strings.TrimSpace(x["BY"]); b != "" {
			s += " BY " + identList(b)
		}
		if verb == "AUDIT" {
			s += " " + x["MODE"]
		}
		if x["WHEN"] != "ALWAYS" {
			s += " " + x["WHEN"]
		}
		return sqlS("%s;", s)
	}
	stmtAudit := v("sec.stmtaudit", L("Auditoria tradicional: comandos", "Traditional audit: statements"), `select user_name, proxy_name, audit_option, success, failure from dba_stmt_audit_opts order by audit_option, user_name`,
		act('n', "AUDIT", false, auditOpt(L("Comando", "Statement")), func(_ Row, x Vals) (Script, error) { return auditBuild("AUDIT", x) }),
		act('v', "NOAUDIT", true, nil, func(r Row, _ Vals) (Script, error) {
			s := "NOAUDIT " + r["AUDIT_OPTION"]
			if u := r["USER_NAME"]; u != "" {
				s += " BY " + Ident(u)
			}
			return sqlS("%s;", s)
		}),
	)
	privAudit := v("sec.privaudit", L("Auditoria tradicional: privilégios", "Traditional audit: privileges"), `select user_name, proxy_name, privilege, success, failure from dba_priv_audit_opts order by privilege, user_name`,
		act('n', "AUDIT", false, auditOpt(L("Privilégio", "Privilege")), func(_ Row, x Vals) (Script, error) { return auditBuild("AUDIT", x) }),
		act('v', "NOAUDIT", true, nil, func(r Row, _ Vals) (Script, error) {
			s := "NOAUDIT " + r["PRIVILEGE"]
			if u := r["USER_NAME"]; u != "" {
				s += " BY " + Ident(u)
			}
			return sqlS("%s;", s)
		}),
	)
	objAudit := v("sec.objaudit", L("Auditoria tradicional: objetos", "Traditional audit: objects"), `select owner, object_name, object_type, alt, aud, com, del, gra, ind, ins, loc, ren, sel, upd, exe, fbk, rea
	   from dba_obj_audit_opts order by owner, object_name`,
		act('n', "AUDIT", false, []Field{
			fR("OPT", L("Operações (ex.: SELECT, UPDATE)", "Operations (e.g. SELECT, UPDATE)"), "SELECT, INSERT, UPDATE, DELETE"),
			fR("OBJ", L("Objeto (OWNER.NOME)", "Object (OWNER.NAME)"), ""),
			fC("MODE", "BY", "BY ACCESS", "BY ACCESS", "BY SESSION"),
			fC("WHEN", L("Quando", "When"), "ALWAYS", "ALWAYS", "WHENEVER SUCCESSFUL", "WHENEVER NOT SUCCESSFUL"),
		}, func(_ Row, x Vals) (Script, error) {
			s := "AUDIT " + strings.ToUpper(x["OPT"]) + " ON " + strings.ToUpper(strings.TrimSpace(x["OBJ"])) + " " + x["MODE"]
			if x["WHEN"] != "ALWAYS" {
				s += " " + x["WHEN"]
			}
			return sqlS("%s;", s)
		}),
		act('v', L("NOAUDIT (tudo do objeto)", "NOAUDIT (all on object)"), true, nil, func(r Row, _ Vals) (Script, error) {
			return sqlS("NOAUDIT ALL ON %s;", Q2(r["OWNER"], r["OBJECT_NAME"]))
		}),
	)
	tradHelp := L("Auditoria tradicional (AUDIT/NOAUDIT). Do 23ai em diante ela foi descontinuada: o Oracle não aceita AUDIT novo (ORA-46401); dá para ver e tirar (NOAUDIT) o que veio de versões antigas. Para auditar, use Políticas unificadas.",
		"Traditional auditing (AUDIT/NOAUDIT). From 23ai on it is desupported: Oracle rejects new AUDIT (ORA-46401); you can still view and remove (NOAUDIT) what came from older releases. To audit, use Unified policies.")
	stmtAudit.Help, privAudit.Help, objAudit.Help = tradHelp, tradHelp, tradHelp
	auditCfg := paramsView("sec.auditcfg", L("Configuração de auditoria", "Audit settings"),
		"where name like 'audit%' or name in ('unified_audit_common_systemlog','unified_audit_systemlog','os_roles','sec_case_sensitive_logon')")

	return &Node{Title: L("Segurança", "Security"), Children: []*Node{
		{Title: L("Usuários", "Users"), View: users},
		{Title: L("Usuários (sem os da Oracle)", "Users (non-Oracle)"), View: appUsers},
		{Title: "Roles", View: roles},
		{Title: "Profiles", View: profiles},
		{Title: L("Privilégios", "Privileges"), Children: []*Node{
			{Title: L("Roles concedidas", "Granted roles"), View: rp},
			{Title: L("De sistema", "System"), View: sp},
			{Title: L("Em objetos", "Object"), View: tp},
			{Title: L("Cotas", "Quotas"), View: quotas},
		}},
		{Title: L("Auditoria", "Audit"), Children: []*Node{
			{Title: L("Configuração", "Settings"), View: auditCfg},
			{Title: L("Políticas (unificada)", "Policies (unified)"), View: policies},
			{Title: L("Políticas habilitadas", "Enabled policies"), View: enabled},
			{Title: L("Trilha unificada", "Unified trail"), View: trail},
			{Title: L("Tradicional: comandos", "Traditional: statements"), View: stmtAudit},
			{Title: L("Tradicional: privilégios", "Traditional: privileges"), View: privAudit},
			{Title: L("Tradicional: objetos", "Traditional: objects"), View: objAudit},
		}},
	}}
}
