//	 ___  ____      _      ____  ____      _    ____   ___
//	/ _ \|  _ \    / \    | __ )|  _ \    / \  | __ ) / _ \
//
// | | | | |_) |  / _ \   |  _ \| |_) |  / _ \ |  _ \| | | |
// | |_| |  _ <  / ___ \  | |_) |  _ <  / ___ \| |_) | |_| |
//
//	\___/|_| \_\/_/   \_\ |____/|_| \_\/_/   \_\____/ \___/
//	                   S T U D I O
//
// ORA BRABO Studio: cliente Oracle leve no terminal (worksheet, navegador de
// objetos e aba DBA). `check` roda todas as consultas da aba DBA sem interface.
// by • DBA BRABO
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/acaciolr/ora-brabo-studio/internal/catalog"
	"github.com/acaciolr/ora-brabo-studio/internal/i18n"
	"github.com/acaciolr/ora-brabo-studio/internal/ora"
	"github.com/acaciolr/ora-brabo-studio/internal/store"
	"github.com/acaciolr/ora-brabo-studio/internal/ui"
)

var version = "dev"

func main() {
	lang := flag.String("lang", "", "pt | en")
	showVer := flag.Bool("version", false, "versão / version")
	flag.Usage = usage
	flag.Parse()
	switch strings.ToLower(*lang) {
	case "en":
		i18n.EN = true
	case "pt", "pt-br":
		i18n.EN = false
	}
	ui.Version = version
	if *showVer {
		fmt.Println("ORA BRABO Studio", version)
		return
	}
	if flag.NArg() > 0 && flag.Arg(0) == "check" {
		os.Exit(check(flag.Args()[1:]))
	}
	a := ui.New(nil)
	if err := a.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, ui.Logo+"\n\n", i18n.L(`Uso:
  ora-brabo-studio [--lang pt|en]          abre a interface
  ora-brabo-studio check <conexão>         roda todas as consultas da aba DBA (senha: ORA_BRABO_PASSWORD se não salva)
  ora-brabo-studio check -host H -port P -service S -user U [-role SYSDBA]
  ora-brabo-studio --version
`, `Usage:
  ora-brabo-studio [--lang pt|en]          opens the interface
  ora-brabo-studio check <connection>      runs every DBA tab query (password: ORA_BRABO_PASSWORD if not saved)
  ora-brabo-studio check -host H -port P -service S -user U [-role SYSDBA]
  ora-brabo-studio --version
`), "\nby • DBA BRABO\n")
}

// check conecta e roda cada consulta de tela da aba DBA (drills vão com NULL).
func check(args []string) int {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	host := fs.String("host", "", "host")
	port := fs.Int("port", 1521, "port")
	svc := fs.String("service", "", "service name")
	user := fs.String("user", "", "user")
	role := fs.String("role", "", "SYSDBA ...")
	verbose := fs.Bool("v", false, "mostra as que passaram / show passing ones")
	_ = fs.Parse(args)
	var info ora.ConnInfo
	if *host != "" {
		info = ora.ConnInfo{Name: "check", Host: *host, Port: *port, Service: *svc, User: *user, Role: *role}
	} else {
		if fs.NArg() == 0 {
			usage()
			return 2
		}
		cs, err := store.LoadConns()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		found := false
		for _, c := range cs {
			if c.Name == fs.Arg(0) {
				info, found = c, true
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, i18n.L("conexão %q não existe\n", "connection %q does not exist\n"), fs.Arg(0))
			return 2
		}
	}
	if info.Password == "" {
		info.Password = os.Getenv("ORA_BRABO_PASSWORD")
	}
	ora.DataDir = store.Dir()
	db, err := ora.Open(info)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer ora.Close(db)
	c := &ui.Conn{Info: info, DB: db}
	ok, fail := 0, 0
	seen := map[string]bool{}
	var views []*catalog.View
	catalog.Walk(catalog.DBA(), func(_ []string, n *catalog.Node) {
		add := func(v *catalog.View) {
			if v != nil && v.SQL != "" && !seen[v.ID+v.SQL] {
				seen[v.ID+v.SQL] = true
				views = append(views, v)
			}
		}
		add(n.View)
		if n.View != nil {
			for _, x := range n.View.Actions {
				add(x.Drill)
			}
		}
	})
	for _, v := range views {
		t0 := time.Now()
		r := ui.Query(c, v.SQL, nilArgs(v.SQL), 50)
		el := time.Since(t0).Round(time.Millisecond)
		if r.Err != nil {
			fail++
			fmt.Printf("ERRO  %-28s %s\n      %s\n", v.ID, v.Title, firstLine(r.Err.Error()))
			continue
		}
		ok++
		if *verbose {
			fmt.Printf("ok    %-28s %-40s %4d %s\n", v.ID, v.Title, len(r.Rows), el)
		}
	}
	fmt.Printf(i18n.L("\n%d telas: %d ok, %d com erro\n", "\n%d screens: %d ok, %d with errors\n"), ok+fail, ok, fail)
	if fail > 0 {
		return 1
	}
	return 0
}

// nilArgs devolve NULL para cada :N da consulta.
func nilArgs(q string) []any {
	n := 0
	for i := 1; i <= 9; i++ {
		if strings.Contains(q, fmt.Sprintf(":%d", i)) {
			n = i
		}
	}
	return make([]any, n)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
