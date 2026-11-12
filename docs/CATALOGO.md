# Aba DBA: todas as telas e ações

97 telas na árvore e 359 ações nelas (fora as telas de detalhe abertas por elas). ⚠ = destrutiva (confirma digitando o nome). 💲 = exige Diagnostics/Tuning Pack.

Gerado do catálogo por `internal/catalog/doc_test.go`. by • DBA BRABO

## Banco de dados

| Tela | Ações (tecla) |
|---|---|
| Painel da instância |  |
| Top Activity |  |
| Banco | `A` Ligar ARCHIVELOG (reinicia o banco, RMAN) ⚠ · `N` Desligar ARCHIVELOG (reinicia o banco, RMAN) ⚠ · `f` Flashback Database ON/OFF · `l` Force logging ON/OFF · `g` Supplemental logging (mínimo) · `d` Tablespace padrão · `n` Renomear global_name · `S` Parar / iniciar a instância (RMAN) ⚠ |
| Instância | `r` Restringir / liberar logins · `q` Quiesce / unquiesce · `s` Suspender / retomar I/O · `f` Limpar shared pool / buffer cache |
| Parâmetros | `e` Alterar (ALTER SYSTEM) · `r` Voltar ao padrão (RESET) · `s` Alterar só na sessão · `p` Criar PFILE a partir do SPFILE · `m` Criar SPFILE a partir da memória |
| Parâmetros alterados | `e` Alterar (ALTER SYSTEM) · `r` Voltar ao padrão (RESET) · `s` Alterar só na sessão · `p` Criar PFILE a partir do SPFILE · `m` Criar SPFILE a partir da memória |
| Serviços | `n` Criar serviço · `s` Iniciar · `p` Parar · `x` Remover ⚠ |
| Pluggable databases | `o` Abrir · `c` Fechar · `s` Gravar / descartar estado (abre no startup) · `n` Criar a partir do seed · `k` Clonar · `r` Refresh (PDB refreshable) · `u` Unplug para XML ⚠ · `p` Plug a partir de XML · `l` Relocate (de outro CDB) · `x` Remover (DROP) ⚠ |
| Componentes |  |
| Opções |  |
| Uso de recursos |  |
| Versão |  |

## Armazenamento

| Tela | Ações (tecla) |
|---|---|
| Tablespaces | `n` Criar · `a` Adicionar arquivo · `o` Online · `f` Offline ⚠ · `r` Somente leitura / leitura e escrita · `m` Renomear · `d` Tornar padrão do banco · `u` Tornar UNDO ativo · `k` Shrink (temporária) · `g` Grupo de temporárias · `l` Logging / nologging / force logging · `x` Remover (DROP) ⚠ · `D` DDL · `v` Datafiles · `e` Segmentos |
| Datafiles | `z` Redimensionar · `a` Autoextend · `o` Online · `f` Offline ⚠ · `m` Mover online (12c+) · `x` Remover arquivo ⚠ |
| Grupos de temporárias | `n` Pôr tablespace num grupo · `x` Tirar do grupo · `d` Grupo padrão do banco |
| Tablespace sets |  |
| Control files | `t` Backup para trace · `b` Backup binário · `s` Seções (registros) |
| Grupos de redo | `s` Switch logfile · `c` Checkpoint · `a` Archive log current · `n` Adicionar grupo · `m` Adicionar membro · `l` Limpar (CLEAR LOGFILE) ⚠ · `x` Remover grupo ⚠ · `v` Membros |
| Standby logs | `n` Adicionar standby log · `x` Remover ⚠ |
| Archive logs (últimos 500) |  |
| Archive por hora (48 h) |  |
| Destinos de archive | `s` Habilitar / adiar (DEFER) · `e` Alterar destino |
| Undo › Estatísticas de undo (24 h) |  |
| Undo › Extents de undo | `r` Alterar undo_retention |
| Undo › Rollback segments | `o` Online · `f` Offline |
| Restore points | `n` Criar · `x` Remover ⚠ · `F` Flashback do banco para este ponto (RMAN) ⚠ |
| Lixeira | `r` Restaurar (FLASHBACK TO BEFORE DROP) · `p` Expurgar objeto ⚠ · `P` Esvaziar a lixeira do banco (DBA_RECYCLEBIN) ⚠ · `t` Esvaziar por tablespace ⚠ |
| Diretórios | `n` Criar / alterar · `g` Conceder acesso · `x` Remover ⚠ · `v` Quem tem acesso |
| Maiores segmentos | `s` Shrink (tabela/índice) · `m` Mover / rebuild |

## Segurança

| Tela | Ações (tecla) |
|---|---|
| Usuários | `n` Criar usuário · `e` Editar · `p` Trocar senha · `l` Travar · `u` Destravar · `x` Expirar senha · `q` Cota · `g` Conceder role · `s` Conceder privilégio de sistema · `o` Conceder privilégio em objeto · `X` Remover usuário ⚠ · `D` DDL · `r` Roles · `y` Privilégios de sistema · `b` Privilégios em objetos · `z` Cotas |
| Usuários (sem os da Oracle) | `n` Criar usuário · `e` Editar · `p` Trocar senha · `l` Travar · `u` Destravar · `x` Expirar senha · `q` Cota · `g` Conceder role · `s` Conceder privilégio de sistema · `o` Conceder privilégio em objeto · `X` Remover usuário ⚠ · `D` DDL · `r` Roles · `y` Privilégios de sistema · `b` Privilégios em objetos · `z` Cotas |
| Roles | `n` Criar role · `t` Conceder a (usuário/role) · `g` Conceder role · `s` Conceder privilégio de sistema · `o` Conceder privilégio em objeto · `X` Remover role ⚠ · `m` Quem tem · `r` Roles dentro · `y` Privilégios de sistema · `b` Privilégios em objetos |
| Profiles | `n` Criar profile · `e` Alterar este limite · `a` Alterar vários limites · `X` Remover profile ⚠ · `u` Usuários com o profile |
| Privilégios › Roles concedidas | `v` Revoke · `d` Default role ON/OFF |
| Privilégios › Privilégios de sistema | `v` Revoke |
| Privilégios › Privilégios em objetos | `v` Revoke |
| Privilégios › Cotas | `q` Alterar cota |
| Auditoria › Configuração de auditoria | `e` Alterar (ALTER SYSTEM) · `r` Voltar ao padrão (RESET) · `s` Alterar só na sessão · `p` Criar PFILE a partir do SPFILE · `m` Criar SPFILE a partir da memória |
| Auditoria › Políticas de auditoria (unificada) | `n` Criar política · `e` Habilitar (AUDIT POLICY) · `d` Desabilitar (NOAUDIT POLICY) · `X` Remover política ⚠ · `v` Opções |
| Auditoria › Políticas habilitadas |  |
| Auditoria › Trilha unificada (últimos 500) | `p` Expurgar a trilha |
| Auditoria › Auditoria tradicional: comandos | `n` AUDIT · `v` NOAUDIT |
| Auditoria › Auditoria tradicional: privilégios | `n` AUDIT · `v` NOAUDIT |
| Auditoria › Auditoria tradicional: objetos | `n` AUDIT · `v` NOAUDIT (tudo do objeto) |

## Backup e Recovery

| Tela | Ações (tecla) |
|---|---|
| Configuração RMAN (não padrão) | `w` SHOW ALL (tudo, via RMAN) · `r` Retenção · `o` Otimização de backup · `d` Dispositivo padrão · `p` Paralelismo e tipo de backup · `c` Algoritmo de compressão · `a` Autobackup do control file · `f` Formato do canal de disco · `t` Parâmetros do canal de fita (SBT) · `l` Política de remoção de archive · `e` Criptografia do backup · `s` Snapshot do control file · `x` Voltar ao padrão (CLEAR) |
| Área de recuperação (FRA) | `z` Tamanho da FRA · `d` Local da FRA · `m` Meta de recuperação de instância (MTTR) · `f` Formato do archive · `A` Ligar ARCHIVELOG (reinicia, RMAN) ⚠ · `N` Desligar ARCHIVELOG (reinicia, RMAN) ⚠ · `b` Flashback Database ON/OFF · `t` Janela do flashback (minutos) · `u` Uso por tipo |
| Jobs de backup | `b` Backup do banco inteiro · `t` Backup de tablespaces · `f` Backup de datafiles · `a` Backup de archive logs · `r` Backup da área de recuperação para fita · `c` Script RMAN livre · `k` CROSSCHECK BACKUP / ARCHIVELOG ALL · `e` DELETE EXPIRED ⚠ · `o` DELETE OBSOLETE ⚠ · `R` REPORT OBSOLETE / NEED BACKUP · `L` LIST BACKUP SUMMARY · `g` CATALOG RECOVERY AREA · `v` Saída do job |
| Backup sets | `k` CROSSCHECK · `v` VALIDATE · `u` Indisponível / disponível · `p` Guardar (KEEP) · `x` DELETE ⚠ · `e` Peças |
| Cópias (image copies) | `k` CROSSCHECK · `v` VALIDATE · `u` Indisponível / disponível · `x` DELETE ⚠ · `s` Trocar o datafile por esta cópia (SWITCH) |
| Recuperação (blocos corrompidos) | `B` Recuperar este bloco · `C` Recuperar todos os corrompidos · `V` Validar o banco (preenche esta lista) · `P` Prévia do restore (o que seria usado) · `T` Testar o restore (VALIDATE, não grava) · `D` Restore + recover do banco inteiro ⚠ · `S` Restore + recover de tablespace ⚠ · `F` Restore + recover de datafile ⚠ · `A` Restaurar archive logs · `R` Recuperar uma tabela (RECOVER TABLE, 12c+) ⚠ · `L` Flashback do banco ⚠ · `X` Falhas detectadas (LIST FAILURE) |

## Data Pump

| Tela | Ações (tecla) |
|---|---|
| Jobs do Data Pump | `e` Export · `i` Import · `p` Parar (pode retomar) · `r` Retomar · `g` Mudar paralelismo · `k` Matar o job ⚠ · `x` Limpar job órfão (DROP da master table) ⚠ · `v` Progresso |
| Diretórios |  |

## Performance

| Tela | Ações (tecla) |
|---|---|
| Snapshots AWR 💲 | `n` Tirar snapshot agora 💲 · `r` Relatório AWR (texto) 💲 · `h` Relatório AWR (HTML, salve com Ctrl+S) 💲 · `d` Comparar dois períodos (AWR diff) 💲 · `s` Relatório de um SQL (awrsqrpt) 💲 · `a` Rodar ADDM neste intervalo 💲 · `b` Criar baseline deste intervalo 💲 · `x` Apagar intervalo de snapshots ⚠ 💲 |
| Baselines 💲 | `n` Criar 💲 · `m` Renomear 💲 · `w` Janela móvel do sistema (dias) 💲 · `r` Relatório AWR da baseline 💲 · `x` Remover ⚠ 💲 |
| Templates de baseline 💲 | `n` Criar (única vez) 💲 · `p` Criar (repetitivo) 💲 · `x` Remover ⚠ 💲 |
| Configuração do AWR 💲 | `e` Alterar 💲 |
| Tarefas do ADDM 💲 | `r` Relatório 💲 · `n` Rodar ADDM (banco) 💲 · `p` Parâmetro padrão do ADDM 💲 · `x` Remover tarefa ⚠ 💲 · `v` Achados 💲 |
| ASH › ASH: eventos (30 min) 💲 | `r` Relatório ASH 💲 |
| ASH › ASH: SQL (30 min) 💲 | `r` Relatório ASH 💲 · `p` Plano (cursor) 💲 |
| Statspack | `n` Tirar snapshot · `x` Apagar intervalo ⚠ |
| SQL Monitor 💲 | `r` Relatório 💲 |
| Tarefas do SQL Tuning Advisor 💲 | `r` Relatório 💲 · `x` Remover ⚠ 💲 |
| SQL Plan Baselines (SPM) | `e` Habilitar / desabilitar · `f` Fixar / soltar · `p` Plano da baseline · `v` Evoluir (verificar planos novos) · `x` Remover ⚠ |
| Estatísticas: tabelas desatualizadas | `g` Coletar · `s` Coletar o schema inteiro · `l` Travar / destravar estatísticas · `d` Coletar estatísticas do dicionário e fixas |

## Resource Manager

| Tela | Ações (tecla) |
|---|---|
| Plano em uso |  |
| Planos | `a` Ativar · `d` Desativar (nenhum plano) · `n` Criar plano · `x` Remover plano ⚠ · `v` Diretivas |
| Diretivas do plano | `n` Adicionar diretiva · `e` Alterar diretiva · `x` Remover diretiva ⚠ |
| Grupos de consumidores | `n` Criar grupo · `g` Permitir troca para o grupo · `x` Remover grupo ⚠ |
| Mapeamentos | `n` Mapear · `x` Remover mapeamento ⚠ |
| Prioridades dos mapeamentos | `e` Alterar prioridades |
| Planos do CDB | `n` Criar plano CDB · `a` Ativar no CDB$ROOT · `x` Remover plano CDB ⚠ · `v` Diretivas |

## Scheduler

| Tela | Ações (tecla) |
|---|---|
| Jobs | `r` Executar agora · `e` Habilitar · `d` Desabilitar · `s` Parar · `i` Alterar a repetição · `n` Criar job · `x` Remover ⚠ · `D` DDL · `l` Execuções |
| Execuções (log) | `p` Limpar o log |
| Programs | `e` Habilitar · `d` Desabilitar · `x` Remover ⚠ |
| Schedules | `n` Criar · `x` Remover ⚠ |
| Janelas | `o` Abrir agora · `c` Fechar · `e` Habilitar · `d` Desabilitar · `p` Trocar o plano da janela |
| Chains | `e` Habilitar · `d` Desabilitar · `x` Remover ⚠ |
| Credenciais | `n` Criar · `x` Remover ⚠ |
| DBMS_JOB | `b` Quebrar / consertar (broken) · `r` Executar agora |
| Tarefas automáticas | `e` Habilitar · `d` Desabilitar |

## Monitoramento

| Tela | Ações (tecla) |
|---|---|
| Sessões ativas | `k` Matar sessão (KILL) ⚠ · `d` Desconectar (DISCONNECT) · `c` Cancelar o SQL em execução (18c+) · `s` Texto do SQL · `p` Plano (cursor) · `t` Ligar trace (10046 com binds e esperas) · `u` Desligar trace · `w` Esperas · `o` Cursores abertos · `e` Estatísticas |
| Sessões de usuário | `k` Matar sessão (KILL) ⚠ · `d` Desconectar (DISCONNECT) · `c` Cancelar o SQL em execução (18c+) · `s` Texto do SQL · `p` Plano (cursor) · `t` Ligar trace (10046 com binds e esperas) · `u` Desligar trace · `w` Esperas · `o` Cursores abertos · `e` Estatísticas |
| Todas as sessões (com background) | `k` Matar sessão (KILL) ⚠ · `d` Desconectar (DISCONNECT) · `c` Cancelar o SQL em execução (18c+) · `s` Texto do SQL · `p` Plano (cursor) · `t` Ligar trace (10046 com binds e esperas) · `u` Desligar trace · `w` Esperas · `o` Cursores abertos · `e` Estatísticas |
| Bloqueios (árvore) | `k` Matar sessão (KILL) ⚠ · `s` Texto do SQL · `p` Plano (cursor) |
| Objetos travados (DML) | `k` Matar sessão (KILL) ⚠ · `s` Texto do SQL |
| Top SQL | `s` Texto do SQL · `p` Plano (cursor) · `h` Planos no histórico (AWR) 💲 · `t` SQL Tuning Advisor 💲 · `m` Relatório do SQL Monitor 💲 · `b` Criar SQL plan baseline (SPM) · `x` Tirar do shared pool (PURGE) |
| Operações longas | `k` Matar sessão (KILL) ⚠ |
| Eventos de espera (desde o startup) |  |
| Métricas (último minuto) |  |
| Tablespaces acima de 85% |  |
| Alert log (últimas 500) |  |
| Alert log: só erros |  |
| Incidentes (ADR) |  |
| Objetos inválidos | `c` Compilar · `s` Recompilar o schema (UTL_RECOMP) · `a` Recompilar tudo (utlrp) · `v` Erros |
| Data Guard › Data Guard |  |
| Data Guard › Data Guard: destinos |  |
| Data Guard › Data Guard: processos |  |
