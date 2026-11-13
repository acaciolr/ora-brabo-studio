// Painel da instância (resumo que se atualiza) e Top Activity (amostras do
// V$SESSION com gráfico por classe de espera, sem ASH). by • DBA BRABO
package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/acaciolr/ora-brabo-studio/internal/ora"
)

// ticker roda f a cada d enquanto a aba estiver aberta e visível.
type ticker struct {
	stop chan struct{}
	once sync.Once
}

func newTicker(a *App, tab Tab, d time.Duration, f func()) *ticker {
	t := &ticker{stop: make(chan struct{})}
	go func() {
		tk := time.NewTicker(d)
		defer tk.Stop()
		for {
			select {
			case <-t.stop:
				return
			case <-tk.C:
				a.tv.QueueUpdateDraw(func() {
					if a.tabs.Current() == tab && a.modals == 0 {
						f()
					}
				})
			}
		}
	}()
	return t
}

func (t *ticker) Close() { t.once.Do(func() { close(t.stop) }) }

func bar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	n := int(pct/100*float64(width) + 0.5)
	col := "green"
	switch {
	case pct >= 90:
		col = "red"
	case pct >= 75:
		col = "yellow"
	}
	return fmt.Sprintf("[%s]%s[gray]%s[-]", col, strings.Repeat("█", n), strings.Repeat("░", width-n))
}

func num(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}

func cellv(r ora.Result, row, col int) string {
	if row < len(r.Rows) && col < len(r.Rows[row]) {
		return r.Rows[row][col].V
	}
	return ""
}

// ---- painel da instância ----

// InstancePanel é o resumo da instância.
type InstancePanel struct {
	*tview.TextView
	baseTab
	tk   *ticker
	busy bool
}

// NewInstancePanel cria o painel.
func NewInstancePanel(a *App, c *Conn) *InstancePanel {
	p := &InstancePanel{TextView: tview.NewTextView().SetDynamicColors(true).SetScrollable(true)}
	p.baseTab = baseTab{a: a, c: c, title: L("Painel da instância", "Instance viewer"), main: p.TextView}
	p.SetText(L(" carregando…", " loading…"))
	p.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if ev.Key() == tcell.KeyF5 {
			p.load()
			return nil
		}
		return ev
	})
	p.load()
	p.tk = newTicker(a, p, 5*time.Second, p.load)
	return p
}

func (p *InstancePanel) Close() { p.tk.Close() }
func (p *InstancePanel) Hint() string {
	return L("[yellow]F5[-] atualiza (automático a cada 5 s)  [yellow]↑↓[-] rolar", "[yellow]F5[-] refresh (auto every 5 s)  [yellow]↑↓[-] scroll")
}

const panelInst = `select i.instance_name, i.host_name, i.version_full, i.status, to_char(i.startup_time, 'yyyy-mm-dd hh24:mi') startup,
       round(sysdate - i.startup_time, 1) days, d.name, d.log_mode, d.open_mode, d.database_role, d.flashback_on, d.force_logging, d.cdb
  from v$instance i, v$database d`

const panelLoad = `select
  (select count(*) from v$session where type = 'USER') sessions,
  (select count(*) from v$session where type = 'USER' and status = 'ACTIVE' and wait_class <> 'Idle') active,
  (select count(*) from v$session where blocking_session is not null) blocked,
  (select value from v$parameter where name = 'processes') processes,
  (select count(*) from v$process) procs,
  (select round(sum(value)/1048576) from v$sga) sga_mb,
  (select round(value/1048576) from v$pgastat where name = 'total PGA allocated') pga_mb,
  (select round(value/1048576) from v$pgastat where name = 'aggregate PGA target parameter') pga_target_mb,
  (select round(value, 1) from v$sysmetric where metric_name = 'Host CPU Utilization (%)' and group_id = 2) host_cpu,
  (select round(value, 2) from v$sysmetric where metric_name = 'Average Active Sessions' and group_id = 2) aas,
  (select round(value) from v$sysmetric where metric_name = 'User Transaction Per Sec' and group_id = 2) tps,
  (select round(value) from v$sysmetric where metric_name = 'Executions Per Sec' and group_id = 2) execs,
  (select round(value, 1) from v$sysmetric where metric_name = 'Buffer Cache Hit Ratio' and group_id = 2) bchr,
  (select round(value/1048576, 1) from v$sysmetric where metric_name = 'Redo Generated Per Sec' and group_id = 2) redo_mb
 from dual`

// loadSQL: dentro de um PDB a V$SYSMETRIC vem vazia; as métricas do
// container ficam na V$CON_SYSMETRIC (12.2+, só o grupo de 60 s, sem CPU do host).
func loadSQL(c *Conn) string {
	if !c.CDB || c.ConName == "" || c.ConName == "CDB$ROOT" {
		return panelLoad
	}
	return strings.ReplaceAll(strings.ReplaceAll(panelLoad, " and group_id = 2", ""), "v$sysmetric", "v$con_sysmetric")
}

const panelTS = `select * from (select tablespace_name, round(used_percent, 1) pct, round(tablespace_size * (select value from v$parameter where name = 'db_block_size') / 1073741824, 1) gb
  from dba_tablespace_usage_metrics order by used_percent desc) where rownum <= 8`

const panelWaits = `select * from (select event, wait_class, round(time_waited_micro/1e6) secs, total_waits from v$system_event
  where wait_class <> 'Idle' order by time_waited_micro desc) where rownum <= 6`

const panelFRA = `select round(space_limit/1073741824, 1), round(space_used/1073741824, 1), round(space_reclaimable/1073741824, 1)
  from v$recovery_file_dest where space_limit > 0`

const panelAlert = `select * from (select to_char(originating_timestamp, 'mm-dd hh24:mi') t, substr(message_text, 1, 110) msg
  from v$diag_alert_ext where message_text like '%ORA-%' and originating_timestamp > sysdate - 1 order by originating_timestamp desc) where rownum <= 5`

func (p *InstancePanel) load() {
	if p.busy {
		return
	}
	p.busy = true
	c := p.c
	p.a.Go(func() func() {
		inst := Query(c, panelInst, nil, 1)
		ld := Query(c, loadSQL(c), nil, 1)
		ts := Query(c, panelTS, nil, 10)
		ws := Query(c, panelWaits, nil, 10)
		fra := Query(c, panelFRA, nil, 1)
		al := Query(c, panelAlert, nil, 5)
		var b strings.Builder
		h := func(s string) { fmt.Fprintf(&b, "\n [#9fd3c7::b]%s[-::-]\n", s) }
		if inst.Err != nil {
			b.WriteString(" [red]" + tview.Escape(inst.Err.Error()) + "[-]\n " + L("O painel precisa de acesso às V$ (SELECT_CATALOG_ROLE ou DBA).", "The panel needs access to V$ views (SELECT_CATALOG_ROLE or DBA)."))
		} else {
			g := func(i int) string { return tview.Escape(cellv(inst, 0, i)) }
			fmt.Fprintf(&b, " %s [::b]%s[::-]  %s  %s\n", Icon(), g(6), g(0)+"@"+g(1), g(2))
			fmt.Fprintf(&b, "   %s %s · %s · %s · %s · flashback %s · force logging %s · CDB %s\n", L("status", "status"), g(3), g(8), g(9), g(7), g(10), g(11), g(12))
			fmt.Fprintf(&b, "   %s %s (%s %s)\n", L("no ar desde", "up since"), g(4), g(5), L("dias", "days"))
		}
		if ld.Err == nil {
			g := func(i int) string { return cellv(ld, 0, i) }
			h(L("Carga", "Load"))
			cpu := num(g(8))
			fmt.Fprintf(&b, "   CPU host   %s %5.1f%%\n", bar(cpu, 30), cpu)
			pr, prmax := num(g(4)), num(g(3))
			if prmax > 0 {
				fmt.Fprintf(&b, "   %s %s %4.0f/%-4.0f\n", L("Processos ", "Processes "), bar(pr/prmax*100, 30), pr, prmax)
			}
			fmt.Fprintf(&b, "   %s %s · %s %s · %s [red]%s[-] · AAS %s · TPS %s · %s/s %s · %s %s%% · redo %s MB/s\n",
				L("sessões", "sessions"), g(0), L("ativas", "active"), g(1), L("bloqueadas", "blocked"), g(2), g(9), g(10), L("execuções", "executions"), g(11), "buffer hit", g(12), g(13))
			h(L("Memória", "Memory"))
			pga, pgat := num(g(6)), num(g(7))
			fmt.Fprintf(&b, "   SGA %s MB · PGA %s MB", g(5), g(6))
			if pgat > 0 {
				fmt.Fprintf(&b, " %s %s / %.0f MB", bar(pga/pgat*100, 20), L("do alvo", "of target"), pgat)
			}
			b.WriteString("\n")
		}
		if ts.Err == nil && len(ts.Rows) > 0 {
			h(L("Tablespaces (mais cheias)", "Tablespaces (fullest)"))
			for i := range ts.Rows {
				pct := num(cellv(ts, i, 1))
				fmt.Fprintf(&b, "   %-22s %s %5.1f%%  %s GB\n", tview.Escape(cellv(ts, i, 0)), bar(pct, 30), pct, cellv(ts, i, 2))
			}
		}
		if fra.Err == nil && len(fra.Rows) > 0 {
			lim, used := num(cellv(fra, 0, 0)), num(cellv(fra, 0, 1))
			h("Fast Recovery Area")
			if lim > 0 {
				fmt.Fprintf(&b, "   %s %5.1f%%  %s / %s GB (%s %s GB)\n", bar(used/lim*100, 30), used/lim*100, cellv(fra, 0, 1), cellv(fra, 0, 0), L("recuperável", "reclaimable"), cellv(fra, 0, 2))
			}
		}
		if ws.Err == nil && len(ws.Rows) > 0 {
			h(L("Maiores esperas desde o startup", "Top waits since startup"))
			for i := range ws.Rows {
				fmt.Fprintf(&b, "   %-40s %-14s %10s s  %s\n", tview.Escape(trunc(cellv(ws, i, 0), 40)), tview.Escape(cellv(ws, i, 1)), cellv(ws, i, 2), cellv(ws, i, 3))
			}
		}
		if al.Err == nil {
			h(L("Erros ORA- no alert log (24 h)", "ORA- errors in alert log (24 h)"))
			if len(al.Rows) == 0 {
				b.WriteString("   [green]" + L("nenhum", "none") + "[-]\n")
			}
			for i := range al.Rows {
				fmt.Fprintf(&b, "   [gray]%s[-] [red]%s[-]\n", cellv(al, i, 0), tview.Escape(cellv(al, i, 1)))
			}
		}
		fmt.Fprintf(&b, "\n [gray]%s %s[-]\n", L("atualizado", "updated"), time.Now().Format("15:04:05"))
		txt := b.String()
		return func() {
			p.busy = false
			row, col := p.GetScrollOffset()
			p.SetText(txt)
			p.ScrollTo(row, col)
		}
	})
}

func trunc(s string, n int) string {
	if len([]rune(s)) > n {
		return string([]rune(s)[:n-1]) + "…"
	}
	return s
}

// ---- Top Activity ----

var waitColors = map[string]string{
	"CPU": "#2EBD59", "User I/O": "#3b82f6", "System I/O": "#06b6d4", "Concurrency": "#a855f7", "Application": "#ef4444",
	"Commit": "#f97316", "Configuration": "#eab308", "Network": "#94a3b8", "Cluster": "#f5f5f4", "Administrative": "#78716c",
	"Scheduler": "#84cc16", "Queueing": "#d946ef", "Other": "#f472b6",
}

var waitOrder = []string{"CPU", "User I/O", "System I/O", "Concurrency", "Application", "Commit", "Configuration", "Network", "Cluster", "Administrative", "Scheduler", "Queueing", "Other"}

type sample struct {
	at      time.Time
	byClass map[string]int
	sqls    map[string]int // sql_id → sessões
	sess    map[string]int // sid,serial|user|program → amostras
}

// TopActivity amostra as sessões ativas.
type TopActivity struct {
	*tview.Flex
	baseTab
	chart   *tview.TextView
	grid    *Grid
	samples []sample
	tk      *ticker
	busy    bool
	cpus    float64
	bySQL   bool
}

// NewTopActivity cria a tela.
func NewTopActivity(a *App, c *Conn) *TopActivity {
	t := &TopActivity{Flex: tview.NewFlex().SetDirection(tview.FlexRow), chart: tview.NewTextView().SetDynamicColors(true), grid: NewGrid(a), bySQL: true}
	t.AddItem(t.chart, 16, 0, false).AddItem(t.grid, 0, 1, true)
	t.baseTab = baseTab{a: a, c: c, title: "Top Activity", main: t.grid}
	t.grid.Keys = func(ev *tcell.EventKey) *tcell.EventKey {
		switch {
		case ev.Key() == tcell.KeyF5:
			t.sample()
			return nil
		case ev.Key() == tcell.KeyRune && ev.Rune() == 's':
			t.bySQL = !t.bySQL
			t.draw()
			return nil
		case ev.Key() == tcell.KeyRune && ev.Rune() == 'p' && t.bySQL:
			if i := t.grid.Selected(); i >= 0 {
				id := t.grid.RowMap(i)["SQL_ID"]
				if id != "" {
					a.openText(c, "Plan "+id, `select plan_table_output from table(dbms_xplan.display_cursor(:1, null, 'TYPICAL'))`, []any{id})
				}
			}
			return nil
		case ev.Key() == tcell.KeyRune && ev.Rune() == 't' && t.bySQL:
			if i := t.grid.Selected(); i >= 0 {
				id := t.grid.RowMap(i)["SQL_ID"]
				if id != "" {
					a.openText(c, "SQL "+id, `select sql_fulltext from v$sqlarea where sql_id = :1`, []any{id})
				}
			}
			return nil
		}
		return ev
	}
	t.grid.OnEnter = func(i int, m map[string]string) { a.TextBox(L("Linha", "Row"), t.grid.recordText(i)) }
	go func() {
		r := Query(c, `select value from v$parameter where name = 'cpu_count'`, nil, 1)
		t.cpus = num(cellv(r, 0, 0))
	}()
	t.chart.SetText(L(" amostrando o V$SESSION a cada 2 s…", " sampling V$SESSION every 2 s…"))
	t.sample()
	t.tk = newTicker(a, t, 2*time.Second, t.sample)
	return t
}

func (t *TopActivity) Close() { t.tk.Close() }
func (t *TopActivity) Hint() string {
	return L("[yellow]s[-] SQL / sessões  [yellow]p[-] plano  [yellow]t[-] texto do SQL  [yellow]Enter[-] detalhe  (amostra a cada 2 s; últimos 5 min)",
		"[yellow]s[-] SQL / sessions  [yellow]p[-] plan  [yellow]t[-] SQL text  [yellow]Enter[-] detail  (sample every 2 s; last 5 min)")
}

const taSQL = `select decode(state, 'WAITING', wait_class, 'CPU') cls, nvl(sql_id, '-') sql_id,
       sid || ',' || serial# || '|' || nvl(username, '(' || type || ')') || '|' || nvl(program, '') || '|' || nvl(event, 'ON CPU') s
  from v$session
 where status = 'ACTIVE' and (wait_class <> 'Idle' or state <> 'WAITING') and sid <> sys_context('userenv', 'sid')`

func (t *TopActivity) sample() {
	if t.busy {
		return
	}
	t.busy = true
	t.a.Go(func() func() {
		r := Query(t.c, taSQL, nil, 10000)
		return func() {
			t.busy = false
			if r.Err != nil {
				t.chart.SetText(" [red]" + tview.Escape(r.Err.Error()) + "[-]")
				return
			}
			s := sample{at: time.Now(), byClass: map[string]int{}, sqls: map[string]int{}, sess: map[string]int{}}
			for _, row := range r.Rows {
				cls := row[0].V
				if _, ok := waitColors[cls]; !ok {
					cls = "Other"
				}
				s.byClass[cls]++
				if row[1].V != "-" {
					s.sqls[row[1].V+"|"+cls]++
				}
				s.sess[row[2].V+"|"+cls]++
			}
			t.samples = append(t.samples, s)
			if len(t.samples) > 150 {
				t.samples = t.samples[len(t.samples)-150:]
			}
			t.draw()
		}
	})
}

func (t *TopActivity) draw() {
	_, _, w, _ := t.chart.GetInnerRect()
	width := w - 8
	if width < 20 {
		width = 100
	}
	sm := t.samples
	if len(sm) > width {
		sm = sm[len(sm)-width:]
	}
	maxv := t.cpus
	for _, s := range sm {
		n := 0
		for _, v := range s.byClass {
			n += v
		}
		if float64(n) > maxv {
			maxv = float64(n)
		}
	}
	if maxv < 1 {
		maxv = 1
	}
	const rows = 12
	var b strings.Builder
	for lvl := rows; lvl >= 1; lvl-- {
		thr := maxv * float64(lvl) / rows
		label := "      "
		if lvl == rows || lvl == rows/2 {
			label = fmt.Sprintf("%5.1f ", thr)
		}
		if t.cpus > 0 && t.cpus <= thr && t.cpus > maxv*float64(lvl-1)/rows {
			label = "[red]CPU─[-]  "
		}
		b.WriteString(label + "│")
		for _, s := range sm {
			acc := 0.0
			cell := " "
			for _, cls := range waitOrder {
				acc += float64(s.byClass[cls])
				if acc >= thr-maxv/rows/2 && s.byClass[cls] > 0 {
					cell = "[" + waitColors[cls] + "]█[-]"
					break
				}
			}
			b.WriteString(cell)
		}
		b.WriteString("\n")
	}
	b.WriteString("      └" + strings.Repeat("─", len(sm)) + "\n       ")
	tot := map[string]int{}
	all := 0
	for _, s := range t.samples {
		for k, v := range s.byClass {
			tot[k] += v
			all += v
		}
	}
	for _, cls := range waitOrder {
		if tot[cls] > 0 {
			fmt.Fprintf(&b, "[%s]■[-] %s %.0f%%  ", waitColors[cls], cls, float64(tot[cls])*100/float64(all))
		}
	}
	if len(t.samples) > 0 {
		last := 0
		for _, v := range t.samples[len(t.samples)-1].byClass {
			last += v
		}
		fmt.Fprintf(&b, "\n       [gray]%s %d · CPUs %.0f · %d %s[-]", L("ativas agora", "active now"), last, t.cpus, len(t.samples), L("amostras", "samples"))
	}
	t.chart.SetText(b.String())
	t.drawTop()
}

func (t *TopActivity) drawTop() {
	agg := map[string]map[string]int{}
	n := len(t.samples)
	for _, s := range t.samples {
		src := s.sess
		if t.bySQL {
			src = s.sqls
		}
		for k, v := range src {
			i := strings.LastIndex(k, "|")
			key, cls := k[:i], k[i+1:]
			if agg[key] == nil {
				agg[key] = map[string]int{}
			}
			agg[key][cls] += v
		}
	}
	type kv struct {
		k   string
		tot int
		cls map[string]int
	}
	var list []kv
	for k, m := range agg {
		tot := 0
		for _, v := range m {
			tot += v
		}
		list = append(list, kv{k, tot, m})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].tot > list[j].tot })
	var r ora.Result
	if t.bySQL {
		r.Cols = []string{"SQL_ID", "AAS", "%", L("ATIVIDADE", "ACTIVITY"), L("CLASSE_PRINCIPAL", "TOP_CLASS")}
	} else {
		r.Cols = []string{"SID,SERIAL#", L("USUARIO", "USER"), "PROGRAM", L("EVENTO", "EVENT"), "AAS", "%", L("CLASSE_PRINCIPAL", "TOP_CLASS")}
	}
	r.Types = make([]string, len(r.Cols))
	all := 0
	for _, x := range list {
		all += x.tot
	}
	for i, x := range list {
		if i >= 50 {
			break
		}
		top, tv := "", 0
		for c, v := range x.cls {
			if v > tv {
				top, tv = c, v
			}
		}
		aas := fmt.Sprintf("%.2f", float64(x.tot)/float64(max(n, 1)))
		pct := fmt.Sprintf("%.1f", float64(x.tot)*100/float64(max(all, 1)))
		if t.bySQL {
			r.Rows = append(r.Rows, []ora.Cell{{V: x.k}, {V: aas}, {V: pct}, {V: strings.Repeat("■", int(float64(x.tot)*30/float64(max(all, 1)))+1)}, {V: top}})
		} else {
			p := strings.SplitN(x.k, "|", 4)
			for len(p) < 4 {
				p = append(p, "")
			}
			r.Rows = append(r.Rows, []ora.Cell{{V: p[0]}, {V: p[1]}, {V: p[2]}, {V: p[3]}, {V: aas}, {V: pct}, {V: top}})
		}
	}
	t.grid.SetResult(r)
}
