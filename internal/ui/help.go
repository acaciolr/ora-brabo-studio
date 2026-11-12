// Logo e ajuda (F1). by • DBA BRABO
package ui

import (
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
  Em cima: [::b]barra de menus[::-] (Arquivo, Editar, Exibir, Navegar, Executar, Ferramentas, Janela, Ajuda) e a
  barra de ferramentas (✚ ▶ ▷ ■ ⇲ ✓ ✗ ⛁). F10 abre os menus; ←/→ trocam de menu; clique também funciona.
  Esquerda: painel [::b]Conexões[::-] (com o navegador de objetos) e, embaixo, o painel [::b]DBA[::-].
  F2 alterna entre os painéis; Ctrl+←/→ e Ctrl+↑/↓ (no painel) mudam o tamanho; Exibir esconde/mostra o painel.
  Direita: abas (worksheets, telas DBA, objetos). Ctrl+PgUp/PgDn ou Alt+←/→ trocam; Alt+1..9 vão direto.
  F3 passa o foco entre painéis, editor e resultado. Ctrl+W fecha a aba. Ctrl+Q sai.
  [::b]Clique direito[::-] (ou Shift+F10 / Ctrl+Space no painel) abre o menu de contexto do que estiver selecionado.
  Clique seleciona; [::b]duplo clique[::-] abre. Ctrl+C copia. Embaixo, a barra de status (clique = mensagens).

[#9fd3c7::b]Conexões[::-]
  Na entrada abre o diálogo [::b]Conectar[::-]: conexões salvas à esquerda, propriedades à direita
  (Testar, Salvar, Conectar). F9 reabre o diálogo a qualquer momento.
  No painel: Enter conecta e abre a árvore.  e editar  c copiar  x remover  d desconectar  w nova worksheet
  Tipos: Básico (host/porta/serviço ou SID), TNS (alias, com sugestões, ou descritor), Cloud Wallet (.zip do
  Autonomous: lista os serviços), LDAP (servidor + contexto) e URL personalizada (jdbc:oracle:thin:@...).
  Autenticação: usuário e senha, sistema operacional (SO) ou Kerberos (ticket do kinit).
  Papel SYSDBA/SYSOPER/SYSBACKUP/SYSDG/SYSKM/SYSRAC/SYSASM. Ctrl+PgUp/PgDn troca as abas do diálogo:
  Avançado (proxy, wallet e senha do wallet, SSL/TCPS, certificado, pasta do tnsnames, RMAN, cor) e
  SSH (túnel por um bastion: chave, agente ou senha; a chave do host é conferida).
  Senhas salvas ficam cifradas (AES-GCM) em `+store.Dir()+`.
  Importa o connections.json do SQL Developer com o tipo de cada conexão (as senhas não vêm).

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
  Resultado em abas: Resultado, Saída do script, Plano, Autotrace, DBMS Output (• = saída nova).
  Ctrl+↑/↓ muda a altura do editor. Barra da worksheet: ▶ ▷ ⇲ autotrace ✓ ✗ ■ limpar histórico autocommit.
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
  Top: [::b]menu bar[::-] (File, Edit, View, Navigate, Run, Tools, Window, Help) and the toolbar
  (✚ ▶ ▷ ■ ⇲ ✓ ✗ ⛁). F10 opens the menus; ←/→ switch menus; mouse clicks work too.
  Left: the [::b]Connections[::-] pane (with the object browser) and, below it, the [::b]DBA[::-] pane.
  F2 switches panes; Ctrl+←/→ and Ctrl+↑/↓ (in a pane) resize; View hides/shows the pane.
  Right: tabs (worksheets, DBA screens, objects). Ctrl+PgUp/PgDn or Alt+←/→ switch; Alt+1..9 jump.
  F3 moves focus between panes, editor and result. Ctrl+W closes the tab. Ctrl+Q quits.
  [::b]Right click[::-] (or Shift+F10 / Ctrl+Space in a pane) opens the context menu of the selection.
  Click selects; [::b]double click[::-] opens. Ctrl+C copies. At the bottom, the status bar (click = messages).

[#9fd3c7::b]Connections[::-]
  At startup the [::b]Connect[::-] dialog opens: saved connections on the left, properties on the right
  (Test, Save, Connect). F9 reopens the dialog at any time.
  In the pane: Enter connects and opens the tree.  e edit  c copy  x delete  d disconnect  w new worksheet
  Types: Basic (host/port/service or SID), TNS (alias, with suggestions, or descriptor), Cloud Wallet
  (Autonomous .zip: lists its services), LDAP (server + context) and Custom URL (jdbc:oracle:thin:@...).
  Authentication: user and password, operating system (OS) or Kerberos (kinit ticket).
  Role SYSDBA/SYSOPER/SYSBACKUP/SYSDG/SYSKM/SYSRAC/SYSASM. Ctrl+PgUp/PgDn switches the dialog tabs:
  Advanced (proxy, wallet and wallet password, SSL/TCPS, certificate, tnsnames folder, RMAN, color) and
  SSH (tunnel through a bastion: key, agent or password; the host key is checked).
  Saved passwords are encrypted (AES-GCM) in `+store.Dir()+`.
  Imports SQL Developer's connections.json with each connection's type (passwords are not imported).

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
  Results in tabs: Result, Script output, Plan, Autotrace, DBMS Output (• = new output).
  Ctrl+↑/↓ changes the editor height. Worksheet bar: ▶ ▷ ⇲ autotrace ✓ ✗ ■ clear history autocommit.
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
