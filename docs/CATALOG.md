# DBA tab: every screen and action

97 screens in the tree and 359 actions on them (plus the detail screens they open). ⚠ = destructive (type the name to confirm). 💲 = requires the Diagnostics/Tuning Pack.

Generated from the catalog by `internal/catalog/doc_test.go`. by • DBA BRABO

## Database

| Screen | Actions (key) |
|---|---|
| Instance viewer |  |
| Top Activity |  |
| Database | `A` Enable ARCHIVELOG (restarts the database, RMAN) ⚠ · `N` Disable ARCHIVELOG (restarts the database, RMAN) ⚠ · `f` Flashback Database ON/OFF · `l` Force logging ON/OFF · `g` Supplemental logging (minimal) · `d` Default tablespace · `n` Rename global_name · `S` Stop / start the instance (RMAN) ⚠ |
| Instance | `r` Restrict / allow logins · `q` Quiesce / unquiesce · `s` Suspend / resume I/O · `f` Flush shared pool / buffer cache |
| Parameters | `e` Change (ALTER SYSTEM) · `r` Reset to default · `s` Change in session · `p` Create PFILE from SPFILE · `m` Create SPFILE from memory |
| Modified parameters | `e` Change (ALTER SYSTEM) · `r` Reset to default · `s` Change in session · `p` Create PFILE from SPFILE · `m` Create SPFILE from memory |
| Services | `n` Create service · `s` Start · `p` Stop · `x` Delete ⚠ |
| Pluggable databases | `o` Open · `c` Close · `s` Save / discard state (opens on startup) · `n` Create from seed · `k` Clone · `r` Refresh (refreshable PDB) · `u` Unplug to XML ⚠ · `p` Plug from XML · `l` Relocate (from another CDB) · `x` Drop ⚠ |
| Components |  |
| Options |  |
| Feature usage |  |
| Version |  |

## Storage

| Screen | Actions (key) |
|---|---|
| Tablespaces | `n` Create · `a` Add file · `o` Online · `f` Offline ⚠ · `r` Read only / read write · `m` Rename · `d` Make database default · `u` Make active UNDO · `k` Shrink (temporary) · `g` Temporary group · `l` Logging / nologging / force logging · `x` Drop ⚠ · `D` DDL · `v` Datafiles · `e` Segments |
| Datafiles | `z` Resize · `a` Autoextend · `o` Online · `f` Offline ⚠ · `m` Move online (12c+) · `x` Drop file ⚠ |
| Temporary groups | `n` Add tablespace to group · `x` Remove from group · `d` Database default group |
| Tablespace sets |  |
| Control files | `t` Backup to trace · `b` Binary backup · `s` Record sections |
| Redo log groups | `s` Switch logfile · `c` Checkpoint · `a` Archive log current · `n` Add group · `m` Add member · `l` Clear logfile ⚠ · `x` Drop group ⚠ · `v` Members |
| Standby logs | `n` Add standby log · `x` Drop ⚠ |
| Archive logs (last 500) |  |
| Archive per hour (48 h) |  |
| Archive destinations | `s` Enable / defer · `e` Change destination |
| Undo › Undo statistics (24 h) |  |
| Undo › Undo extents | `r` Change undo_retention |
| Undo › Rollback segments | `o` Online · `f` Offline |
| Restore points | `n` Create · `x` Drop ⚠ · `F` Flashback database to this point (RMAN) ⚠ |
| Recycle bin | `r` Restore (FLASHBACK TO BEFORE DROP) · `p` Purge object ⚠ · `P` Empty the database recycle bin ⚠ · `t` Purge by tablespace ⚠ |
| Directories | `n` Create / replace · `g` Grant access · `x` Drop ⚠ · `v` Who has access |
| Largest segments | `s` Shrink (table/index) · `m` Move / rebuild |

## Security

| Screen | Actions (key) |
|---|---|
| Users | `n` Create user · `e` Edit · `p` Change password · `l` Lock · `u` Unlock · `x` Expire password · `q` Quota · `g` Grant role · `s` Grant system privilege · `o` Grant object privilege · `X` Drop user ⚠ · `D` DDL · `r` Roles · `y` System privileges · `b` Object privileges · `z` Quotas |
| Users (non-Oracle) | `n` Create user · `e` Edit · `p` Change password · `l` Lock · `u` Unlock · `x` Expire password · `q` Quota · `g` Grant role · `s` Grant system privilege · `o` Grant object privilege · `X` Drop user ⚠ · `D` DDL · `r` Roles · `y` System privileges · `b` Object privileges · `z` Quotas |
| Roles | `n` Create role · `t` Grant to (user/role) · `g` Grant role · `s` Grant system privilege · `o` Grant object privilege · `X` Drop role ⚠ · `m` Grantees · `r` Nested roles · `y` System privileges · `b` Object privileges |
| Profiles | `n` Create profile · `e` Change this limit · `a` Change several limits · `X` Drop profile ⚠ · `u` Users with profile |
| Privileges › Granted roles | `v` Revoke · `d` Default role ON/OFF |
| Privileges › System privileges | `v` Revoke |
| Privileges › Object privileges | `v` Revoke |
| Privileges › Quotas | `q` Change quota |
| Audit › Audit settings | `e` Change (ALTER SYSTEM) · `r` Reset to default · `s` Change in session · `p` Create PFILE from SPFILE · `m` Create SPFILE from memory |
| Audit › Audit policies (unified) | `n` Create policy · `e` Enable (AUDIT POLICY) · `d` Disable (NOAUDIT POLICY) · `X` Drop policy ⚠ · `v` Options |
| Audit › Enabled policies |  |
| Audit › Unified trail (last 500) | `p` Purge the trail |
| Audit › Traditional audit: statements | `n` AUDIT · `v` NOAUDIT |
| Audit › Traditional audit: privileges | `n` AUDIT · `v` NOAUDIT |
| Audit › Traditional audit: objects | `n` AUDIT · `v` NOAUDIT (all on object) |

## Backup and Recovery

| Screen | Actions (key) |
|---|---|
| RMAN configuration (non-default) | `w` SHOW ALL (everything, via RMAN) · `r` Retention policy · `o` Backup optimization · `d` Default device · `p` Parallelism and backup type · `c` Compression algorithm · `a` Control file autobackup · `f` Disk channel format · `t` Tape (SBT) channel parameters · `l` Archivelog deletion policy · `e` Backup encryption · `s` Snapshot control file · `x` Back to default (CLEAR) |
| Recovery area (FRA) | `z` FRA size · `d` FRA location · `m` Instance recovery target (MTTR) · `f` Archive format · `A` Enable ARCHIVELOG (restarts, RMAN) ⚠ · `N` Disable ARCHIVELOG (restarts, RMAN) ⚠ · `b` Flashback Database ON/OFF · `t` Flashback window (minutes) · `u` Usage by type |
| Backup jobs | `b` Whole database backup · `t` Tablespace backup · `f` Datafile backup · `a` Archivelog backup · `r` Recovery area backup to tape · `c` Custom RMAN script · `k` CROSSCHECK BACKUP / ARCHIVELOG ALL · `e` DELETE EXPIRED ⚠ · `o` DELETE OBSOLETE ⚠ · `R` REPORT OBSOLETE / NEED BACKUP · `L` LIST BACKUP SUMMARY · `g` CATALOG RECOVERY AREA · `v` Job output |
| Backup sets | `k` CROSSCHECK · `v` VALIDATE · `u` Unavailable / available · `p` Keep · `x` DELETE ⚠ · `e` Pieces |
| Image copies | `k` CROSSCHECK · `v` VALIDATE · `u` Unavailable / available · `x` DELETE ⚠ · `s` Switch datafile to this copy |
| Recovery (corrupt blocks) | `B` Recover this block · `C` Recover all corrupt blocks · `V` Validate database (fills this list) · `P` Restore preview (what would be used) · `T` Test the restore (VALIDATE, writes nothing) · `D` Restore + recover whole database ⚠ · `S` Restore + recover tablespace ⚠ · `F` Restore + recover datafile ⚠ · `A` Restore archive logs · `R` Recover a table (RECOVER TABLE, 12c+) ⚠ · `L` Flashback database ⚠ · `X` Detected failures (LIST FAILURE) |

## Data Pump

| Screen | Actions (key) |
|---|---|
| Data Pump jobs | `e` Export · `i` Import · `p` Stop (can resume) · `r` Resume · `g` Change parallelism · `k` Kill the job ⚠ · `x` Clean orphan job (drop master table) ⚠ · `v` Progress |
| Directories |  |

## Performance

| Screen | Actions (key) |
|---|---|
| AWR snapshots 💲 | `n` Take snapshot now 💲 · `r` AWR report (text) 💲 · `h` AWR report (HTML, save with Ctrl+S) 💲 · `d` Compare two periods (AWR diff) 💲 · `s` SQL report (awrsqrpt) 💲 · `a` Run ADDM on this range 💲 · `b` Create baseline from this range 💲 · `x` Drop snapshot range ⚠ 💲 |
| Baselines 💲 | `n` Create 💲 · `m` Rename 💲 · `w` System moving window (days) 💲 · `r` Baseline AWR report 💲 · `x` Drop ⚠ 💲 |
| Baseline templates 💲 | `n` Create (single) 💲 · `p` Create (repeating) 💲 · `x` Drop ⚠ 💲 |
| AWR settings 💲 | `e` Change 💲 |
| ADDM tasks 💲 | `r` Report 💲 · `n` Run ADDM (database) 💲 · `p` ADDM default parameter 💲 · `x` Delete task ⚠ 💲 · `v` Findings 💲 |
| ASH › ASH: events (30 min) 💲 | `r` ASH report 💲 |
| ASH › ASH: SQL (30 min) 💲 | `r` ASH report 💲 · `p` Plan (cursor) 💲 |
| Statspack | `n` Take snapshot · `x` Purge range ⚠ |
| SQL Monitor 💲 | `r` Report 💲 |
| SQL Tuning Advisor tasks 💲 | `r` Report 💲 · `x` Drop ⚠ 💲 |
| SQL Plan Baselines (SPM) | `e` Enable / disable · `f` Fix / unfix · `p` Baseline plan · `v` Evolve (verify new plans) · `x` Drop ⚠ |
| Statistics: stale tables | `g` Gather · `s` Gather the whole schema · `l` Lock / unlock statistics · `d` Gather dictionary and fixed stats |

## Resource Manager

| Screen | Actions (key) |
|---|---|
| Active plan |  |
| Plans | `a` Activate · `d` Deactivate (no plan) · `n` Create plan · `x` Delete plan ⚠ · `v` Directives |
| Plan directives | `n` Add directive · `e` Change directive · `x` Delete directive ⚠ |
| Consumer groups | `n` Create group · `g` Grant switch to group · `x` Delete group ⚠ |
| Mappings | `n` Map · `x` Remove mapping ⚠ |
| Mapping priorities | `e` Change priorities |
| CDB plans | `n` Create CDB plan · `a` Activate in CDB$ROOT · `x` Delete CDB plan ⚠ · `v` Directives |

## Scheduler

| Screen | Actions (key) |
|---|---|
| Jobs | `r` Run now · `e` Enable · `d` Disable · `s` Stop · `i` Change repeat interval · `n` Create job · `x` Drop ⚠ · `D` DDL · `l` Runs |
| Runs (log) | `p` Purge the log |
| Programs | `e` Enable · `d` Disable · `x` Drop ⚠ |
| Schedules | `n` Create · `x` Drop ⚠ |
| Windows | `o` Open now · `c` Close · `e` Enable · `d` Disable · `p` Change window plan |
| Chains | `e` Enable · `d` Disable · `x` Drop ⚠ |
| Credentials | `n` Create · `x` Drop ⚠ |
| DBMS_JOB | `b` Break / fix (broken) · `r` Run now |
| Automated tasks | `e` Enable · `d` Disable |

## Monitoring

| Screen | Actions (key) |
|---|---|
| Active sessions | `k` Kill session ⚠ · `d` Disconnect · `c` Cancel running SQL (18c+) · `s` SQL text · `p` Plan (cursor) · `t` Enable trace (10046 with binds and waits) · `u` Disable trace · `w` Waits · `o` Open cursors · `e` Statistics |
| User sessions | `k` Kill session ⚠ · `d` Disconnect · `c` Cancel running SQL (18c+) · `s` SQL text · `p` Plan (cursor) · `t` Enable trace (10046 with binds and waits) · `u` Disable trace · `w` Waits · `o` Open cursors · `e` Statistics |
| All sessions (with background) | `k` Kill session ⚠ · `d` Disconnect · `c` Cancel running SQL (18c+) · `s` SQL text · `p` Plan (cursor) · `t` Enable trace (10046 with binds and waits) · `u` Disable trace · `w` Waits · `o` Open cursors · `e` Statistics |
| Blocking (tree) | `k` Kill session ⚠ · `s` SQL text · `p` Plan (cursor) |
| Locked objects (DML) | `k` Kill session ⚠ · `s` SQL text |
| Top SQL | `s` SQL text · `p` Plan (cursor) · `h` Plans in history (AWR) 💲 · `t` SQL Tuning Advisor 💲 · `m` SQL Monitor report 💲 · `b` Create SQL plan baseline (SPM) · `x` Purge from shared pool |
| Long operations | `k` Kill session ⚠ |
| Wait events (since startup) |  |
| Metrics (last minute) |  |
| Tablespaces above 85% |  |
| Alert log (last 500) |  |
| Alert log: errors only |  |
| Incidents (ADR) |  |
| Invalid objects | `c` Compile · `s` Recompile schema (UTL_RECOMP) · `a` Recompile all (utlrp) · `v` Errors |
| Data Guard › Data Guard |  |
| Data Guard › Data Guard: destinations |  |
| Data Guard › Data Guard: processes |  |
