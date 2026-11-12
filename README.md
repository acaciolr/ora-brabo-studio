<div align="center">

```
  ___  ____      _      ____  ____      _    ____   ___
 / _ \|  _ \    / \    | __ )|  _ \    / \  | __ ) / _ \
| | | | |_) |  / _ \   |  _ \| |_) |  / _ \ |  _ \| | | |
| |_| |  _ <  / ___ \  | |_) |  _ <  / ___ \| |_) | |_| |
 \___/|_| \_\/_/   \_\ |____/|_| \_\/_/   \_\____/ \___/
                    S T U D I O
```

**Cliente Oracle leve e rápido no terminal: worksheet SQL, navegador de objetos e aba DBA**

*Light and fast Oracle client for the terminal: SQL worksheet, object browser and DBA tab*

[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?style=flat-square&logo=go)](https://go.dev)
[![Oracle](https://img.shields.io/badge/Oracle-11g%20→%2026ai-F80000?style=flat-square&logo=oracle)](https://www.oracle.com/database/)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)
[![DBA BRABO](https://img.shields.io/badge/by-DBA%20BRABO-orange?style=flat-square)](https://github.com/acaciolr)

[Português](#português) · [English](#english)

</div>

---

## Português

### O que é

O **ORA BRABO Studio** é um cliente Oracle completo que roda no terminal, inclusive por SSH. É um binário único em Go, sem Java e sem Instant Client: a conexão usa o driver [go-ora](https://github.com/sijms/go-ora), escrito em Go puro. Abre em menos de um segundo.

O ícone verde de banco ⛁ fica no canto superior esquerdo. À esquerda ficam **Conexões** (com o navegador de objetos) e **DBA**; à direita, as abas.

### Recursos

| Área | O que tem |
|---|---|
| **Conexões** | Básica (host/porta/serviço ou SID), TNS (alias do `tnsnames.ora` ou descritor), papéis SYSDBA/SYSOPER/SYSBACKUP/SYSDG/SYSKM, wallet, SSL/TCPS, cor por conexão. A senha é guardada opcionalmente, cifrada com AES-GCM. Importa o `connections.json` do SQL Developer. |
| **Worksheet** | Sessão própria por aba, **sem autocommit** (F11 commit, F12 rollback, aviso de pendência ao fechar). F9 executa o comando do cursor e F5 roda o script como no SQL*Plus (`;`, `/`, blocos PL/SQL). F10 mostra o plano e F6 o autotrace (estatísticas + plano real). Também tem: binds `:var`, DBMS_OUTPUT, erros de compilação com linha, DESC/SET/SHOW/PROMPT/EXEC, completar (Ctrl+Space), histórico (Ctrl+R), abrir/salvar e cancelar (Esc). |
| **Resultado** | Grade com valor da célula, registro, cópia, ordenação, filtro e exportação em CSV / INSERT / JSON. |
| **Objetos** | Tabelas, views, índices, packages, procedures, funções, gatilhos, tipos, sequências, sinônimos, MVs, DB links, jobs, filas e lixeira, para o próprio usuário e para os outros. O visualizador tem abas por tipo: colunas, dados, constraints, índices, estatísticas, partições, segmentos, grants, dependências, código, erros e DDL. Cada tipo tem suas ações (compilar, rebuild, shrink, move, truncate, drop...). Na aba Dados é possível editar por ROWID. |
| **Aba DBA** | 97 telas e 389 ações em 9 grupos. **Banco**: painel da instância, Top Activity, parâmetros, serviços, PDBs. **Armazenamento**: tablespaces, datafiles, redo, archive, undo, restore points, lixeira. **Segurança**: usuários, roles, profiles, privilégios, auditoria unificada e tradicional. **Backup/Recovery**: RMAN (configuração, jobs, backup sets, cópias, restore/recover, recover table, flashback). **Data Pump**: export/import pelo DBMS_DATAPUMP. **Desempenho**: AWR, ASH, ADDM, Statspack, SQL Monitor, SQL Tuning, SPM, estatísticas. **Resource Manager**, **Scheduler** e **Monitoramento**: sessões, bloqueios, Top SQL, long ops, esperas, métricas, alert log, incidentes, inválidos, Data Guard. |

**Segurança das ações:** antes de executar, toda ação mostra o comando gerado, e dá para editá-lo. As ações destrutivas pedem que você digite o nome do alvo. O RMAN roda pelo *Comando RMAN* da conexão, que recebe o script pela entrada (`ssh oracle@db '. ~/.bash_profile; rman target /'`, `docker exec -i meu-oracle rman target /` ou local).

**Licenças:** AWR, ASH, ADDM, SQL Monitor e SQL Tuning exigem o Diagnostics Pack e o Tuning Pack. O programa avisa antes de usá-los e mostra o `control_management_pack_access`. Sem os packs, use o Statspack.

### Instalar

```bash
go install github.com/acaciolr/ora-brabo-studio/cmd/ora-brabo-studio@latest
# ou
git clone https://github.com/acaciolr/ora-brabo-studio && cd ora-brabo-studio
CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o ora-brabo-studio ./cmd/ora-brabo-studio
```

### Usar

```bash
ora-brabo-studio                 # interface
ora-brabo-studio --lang en       # em inglês (ou ORA_BRABO_LANG=en)
ora-brabo-studio check minha-conexao -v   # roda todas as consultas da aba DBA, sem interface
```

F1 abre a ajuda completa. Os atalhos principais são:
- F2: alterna entre Conexões e DBA.
- F3: muda o foco.
- Ctrl+N: nova worksheet.
- Ctrl+W: fecha a aba.
- Ctrl+Q: sai.

Configuração: `~/.config/ora-brabo-studio` (ou `ORA_BRABO_STUDIO_HOME`). Em terminal sem o glifo ⛁, use `ORA_BRABO_ASCII=1`.

---

## English

### What it is

**ORA BRABO Studio** is a full Oracle client that runs in the terminal, including over SSH. It is a single Go binary with no Java and no Instant Client: it connects through the pure-Go [go-ora](https://github.com/sijms/go-ora) driver. It starts in under a second.

The green database icon ⛁ sits in the top-left corner. On the left are **Connections** (with the object browser) and **DBA**; on the right, the tabs.

### Features

| Area | What you get |
|---|---|
| **Connections** | Basic (host/port/service or SID), TNS (`tnsnames.ora` alias or descriptor), SYSDBA/SYSOPER/SYSBACKUP/SYSDG/SYSKM roles, wallet, SSL/TCPS, per-connection color. Passwords can optionally be saved, encrypted with AES-GCM. Imports SQL Developer's `connections.json`. |
| **Worksheet** | One session per tab, **no autocommit** (F11 commit, F12 rollback, warning about pending changes on close). F9 runs the statement at the cursor and F5 runs the script SQL*Plus style (`;`, `/`, PL/SQL blocks). F10 shows the plan and F6 the autotrace (statistics + actual plan). Also: `:var` binds, DBMS_OUTPUT, compile errors with line numbers, DESC/SET/SHOW/PROMPT/EXEC, completion (Ctrl+Space), history (Ctrl+R), open/save and cancel (Esc). |
| **Result** | Grid with cell value, record view, copy, sort, filter and export to CSV / INSERT / JSON. |
| **Objects** | Tables, views, indexes, packages, procedures, functions, triggers, types, sequences, synonyms, MVs, DB links, jobs, queues and recycle bin, for your own user and for other users. The viewer has tabs per type: columns, data, constraints, indexes, stats, partitions, segments, grants, dependencies, code, errors and DDL. Each type has its own actions (compile, rebuild, shrink, move, truncate, drop...). The Data tab allows editing by ROWID. |
| **DBA tab** | 97 screens and 389 actions in 9 groups. **Database**: instance viewer, Top Activity, parameters, services, PDBs. **Storage**: tablespaces, datafiles, redo, archive, undo, restore points, recycle bin. **Security**: users, roles, profiles, privileges, unified and traditional audit. **Backup/Recovery**: RMAN (configuration, jobs, backup sets, copies, restore/recover, recover table, flashback). **Data Pump**: export/import through DBMS_DATAPUMP. **Performance**: AWR, ASH, ADDM, Statspack, SQL Monitor, SQL Tuning, SPM, statistics. **Resource Manager**, **Scheduler** and **Monitoring**: sessions, blocking, Top SQL, long ops, waits, metrics, alert log, incidents, invalid objects, Data Guard. |

**Safe actions:** before running, every action shows the generated command, and you can edit it. Destructive actions ask you to type the target name. RMAN runs through the connection's *RMAN command*, which receives the script on stdin (`ssh oracle@db '. ~/.bash_profile; rman target /'`, `docker exec -i my-oracle rman target /` or local).

**Licensing:** AWR, ASH, ADDM, SQL Monitor and SQL Tuning require the Diagnostics Pack and the Tuning Pack. The program warns before using them and shows `control_management_pack_access`. Without the packs, use Statspack.

### Install

```bash
go install github.com/acaciolr/ora-brabo-studio/cmd/ora-brabo-studio@latest
```

### Use

```bash
ora-brabo-studio                 # UI (Portuguese by default)
ora-brabo-studio --lang en       # English (or ORA_BRABO_LANG=en)
ora-brabo-studio check my-connection -v   # runs every DBA tab query, headless
```

F1 opens the full help. The main shortcuts are:
- F2: switches between Connections and DBA.
- F3: moves focus.
- Ctrl+N: new worksheet.
- Ctrl+W: closes the tab.
- Ctrl+Q: quits.

---

<div align="center">

Oracle e SQL Developer são marcas da Oracle Corporation. Este projeto é independente e não tem vínculo com a Oracle.
*Oracle and SQL Developer are trademarks of Oracle Corporation. This project is independent and not affiliated with Oracle.*

**by • DBA BRABO**

</div>
