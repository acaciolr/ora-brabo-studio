// Boas-vindas (primeira execução) e ajuda (F1). by • DBA BRABO
package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/acaciolr/ora-brabo-studio/internal/store"
)

// Logo é o banner do programa.
const Logo = `  ___  ____      _      ____  ____      _    ____   ___
 / _ \|  _ \    / \    | __ )|  _ \    / \  | __ ) / _ \
| | | | |_) |  / _ \   |  _ \| |_) |  / _ \ |  _ \| | | |
| |_| |  _ <  / ___ \  | |_) |  _ <  / ___ \| |_) | |_| |
 \___/|_| \_\/_/   \_\ |____/|_| \_\/_/   \_\____/ \___/
                    S T U D I O`

func helpText() string {
	return L(`[#2EBD59::b]ORA BRABO Studio[-::-]  `+Version+`
Cliente Oracle no terminal, leve e rápido: worksheet SQL, navegador de objetos e a aba DBA.

[#9fd3c7::b]Tela[-::-]
  Esquerda: [::b]Conexões[::-] (com o navegador de objetos) e [::b]DBA[::-]. F2 alterna; clique também funciona.
  Direita: abas (worksheets, telas DBA, objetos). Ctrl+PgUp/PgDn ou Alt+←/→ trocam; Alt+1..9 vão direto.
  F3 passa o foco entre árvore, editor e resultado. Ctrl+W fecha a aba. Ctrl+Q sai.

[#9fd3c7::b]Conexões[::-]
  Enter conecta e abre a árvore.  e editar  c copiar  x remover  d desconectar  w nova worksheet
  "+ Nova conexão": básica (host/porta/serviço ou SID), TNS (alias do tnsnames.ora ou descritor),
  papel SYSDBA/SYSOPER/SYSBACKUP/SYSDG/SYSKM, wallet, SSL e o comando do RMAN.
  Senhas salvas ficam cifradas (AES-GCM) em `+store.Dir()+`.
  Importa o connections.json do SQL Developer (as senhas não vêm).

[#9fd3c7::b]Navegador de objetos[::-]
  Enter numa pasta lista; Enter no objeto abre o visualizador (Tab troca as abas).
  Letras fazem as ações do tipo (veja a linha de baixo do visualizador). D mostra o DDL. r recarrega a pasta.
  Aba Dados: Ctrl+F filtro WHERE, Ctrl+E edita a célula, Ctrl+N nova linha, Ctrl+D apaga (sempre com prévia do DML).

[#9fd3c7::b]Worksheet[::-]
  Sessão própria e [::b]sem autocommit[::-] (F11 commit, F12 rollback; Ctrl+T liga o autocommit).
  F9 (ou Ctrl+E) executa o comando do cursor ou a seleção; F5 roda o script todo como no SQL*Plus.
  F10 plano de execução; F6 autotrace (estatísticas + plano real). Esc cancela o que estiver rodando.
  Ctrl+Space completa (tabelas, views, colunas por tabela/alias). Ctrl+R histórico. Ctrl+O/Ctrl+S abrir/salvar.
  Variáveis :nome são pedidas antes de executar. DBMS_OUTPUT aparece sozinho.
  Comandos do SQL*Plus: DESC, SET SERVEROUTPUT, SET AUTOCOMMIT, SHOW USER/ERRORS/PARAMETER/SGA/CON_NAME/PDBS, PROMPT, EXEC.
  Resultado: Enter/v valor da célula, R registro, y copia célula, Y copia tudo, o ordena, / filtra, x exporta (CSV/INSERT/JSON).
  t alterna entre a grade e a saída em texto.

[#9fd3c7::b]Aba DBA[::-]
  Banco, armazenamento, segurança, backup/recovery (RMAN), Data Pump, desempenho (AWR/ASH/ADDM, Statspack,
  SQL Tuning, SPM, estatísticas), Resource Manager, Scheduler e monitoramento (sessões, locks, Top SQL, alert log...).
  Cada tela: F5 atualiza, F4 atualização automática, Enter mostra o registro, ? ajuda da tela, letras = ações.
  Toda ação mostra o comando antes (dá para editar). As destrutivas pedem para digitar o nome do alvo.
  RMAN roda pelo "Comando RMAN" da conexão (ssh, docker exec ou local), recebendo o script pela entrada.
  [orange]AWR, ASH, ADDM, SQL Monitor e SQL Tuning exigem licença dos packs (Diagnostics/Tuning).[-]
  O programa avisa antes; Statspack é a alternativa sem licença.

[#9fd3c7::b]Idioma[::-]
  Português por padrão. Inglês: ORA_BRABO_LANG=en (ou --lang en). Terminal sem o ícone ⛁: ORA_BRABO_ASCII=1.

[gray]by • DBA BRABO[-]`,
		`[#2EBD59::b]ORA BRABO Studio[-::-]  `+Version+`
Light and fast Oracle client for the terminal: SQL worksheet, object browser and the DBA tab.

[#9fd3c7::b]Screen[-::-]
  Left: [::b]Connections[::-] (with the object browser) and [::b]DBA[::-]. F2 switches; mouse clicks work too.
  Right: tabs (worksheets, DBA screens, objects). Ctrl+PgUp/PgDn or Alt+←/→ switch; Alt+1..9 jump.
  F3 moves focus between tree, editor and result. Ctrl+W closes the tab. Ctrl+Q quits.

[#9fd3c7::b]Connections[::-]
  Enter connects and opens the tree.  e edit  c copy  x delete  d disconnect  w new worksheet
  "+ New connection": basic (host/port/service or SID), TNS (tnsnames.ora alias or descriptor),
  role SYSDBA/SYSOPER/SYSBACKUP/SYSDG/SYSKM, wallet, SSL and the RMAN command.
  Saved passwords are encrypted (AES-GCM) in `+store.Dir()+`.
  Imports SQL Developer's connections.json (passwords are not imported).

[#9fd3c7::b]Object browser[::-]
  Enter on a folder lists it; Enter on an object opens the viewer (Tab switches tabs).
  Letters run the type's actions (see the viewer's bottom line). D shows the DDL. r reloads the folder.
  Data tab: Ctrl+F WHERE filter, Ctrl+E edit cell, Ctrl+N new row, Ctrl+D delete (always previews the DML).

[#9fd3c7::b]Worksheet[::-]
  Own session, [::b]no autocommit[::-] (F11 commit, F12 rollback; Ctrl+T turns autocommit on).
  F9 (or Ctrl+E) runs the statement at the cursor or the selection; F5 runs the whole script like SQL*Plus.
  F10 execution plan; F6 autotrace (statistics + actual plan). Esc cancels what is running.
  Ctrl+Space completes (tables, views, columns by table/alias). Ctrl+R history. Ctrl+O/Ctrl+S open/save.
  :name bind variables are asked before running. DBMS_OUTPUT shows up by itself.
  SQL*Plus commands: DESC, SET SERVEROUTPUT, SET AUTOCOMMIT, SHOW USER/ERRORS/PARAMETER/SGA/CON_NAME/PDBS, PROMPT, EXEC.
  Result: Enter/v cell value, R record, y copy cell, Y copy all, o sort, / filter, x export (CSV/INSERT/JSON).
  t toggles between the grid and the text output.

[#9fd3c7::b]DBA tab[::-]
  Database, storage, security, backup/recovery (RMAN), Data Pump, performance (AWR/ASH/ADDM, Statspack,
  SQL Tuning, SPM, statistics), Resource Manager, Scheduler and monitoring (sessions, locks, Top SQL, alert log...).
  Every screen: F5 refresh, F4 auto refresh, Enter shows the record, ? screen help, letters = actions.
  Every action shows the command first (you can edit it). Destructive ones ask you to type the target name.
  RMAN runs through the connection's "RMAN command" (ssh, docker exec or local), receiving the script on stdin.
  [orange]AWR, ASH, ADDM, SQL Monitor and SQL Tuning require the Diagnostics/Tuning Pack licenses.[-]
  The program warns first; Statspack is the license-free alternative.

[#9fd3c7::b]Language[::-]
  Portuguese by default. English: ORA_BRABO_LANG=en (or --lang en). Terminal without the ⛁ glyph: ORA_BRABO_ASCII=1.

[gray]by • DBA BRABO[-]`)
}

// NewHelp abre a ajuda (ou volta para ela).
func NewHelp(a *App) Tab {
	t := NewTextTab(a, nil, L("Ajuda", "Help"), "")
	t.SetDynamicColors(true).SetWrap(true)
	t.SetText(helpText())
	t.text = helpText()
	return t
}

// NewWelcome é a tela da primeira execução.
func NewWelcome(a *App) Tab {
	t := NewTextTab(a, nil, L("Bem-vindo", "Welcome"), "")
	t.SetDynamicColors(true).SetWrap(true)
	txt := fmt.Sprintf("[#2EBD59]%s[-]\n\n", tview.Escape(Logo)) + L(
		`  Bem-vindo ao [::b]ORA BRABO Studio[::-].

  1. Na esquerda, Enter em [#2EBD59]+ Nova conexão[-] (ou importe as do SQL Developer).
  2. Enter na conexão: abre o navegador de objetos e a worksheet (Ctrl+N).
  3. F2 mostra a aba [::b]DBA[::-] com as telas de administração.

  F1 abre a ajuda completa a qualquer momento.

  [gray]Enter cria a primeira conexão agora.  by • DBA BRABO[-]`,
		`  Welcome to [::b]ORA BRABO Studio[::-].

  1. On the left, Enter on [#2EBD59]+ New connection[-] (or import SQL Developer's).
  2. Enter on the connection: opens the object browser and a worksheet (Ctrl+N).
  3. F2 shows the [::b]DBA[::-] tab with the administration screens.

  F1 opens the full help at any time.

  [gray]Enter creates the first connection now.  by • DBA BRABO[-]`)
	t.SetText(txt)
	t.text = txt
	t.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyEnter {
			a.connForm(nil)
			return nil
		}
		return ev
	})
	return t
}
