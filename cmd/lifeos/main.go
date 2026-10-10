// Command lifeos is the single LifeOS binary: REST API, admin pages, MCP
// server, CLI and jobs. Subcommands are thin adapters over internal/service.
package main

import (
	"errors"
	"fmt"
	"os"
)

var errNotImplemented = errors.New("not implemented")

type command struct {
	name    string
	usage   string
	summary string
	run     func(args []string) error
}

var commands = []command{
	{"serve", "serve", "run the REST API, admin pages and MCP server", serve},
	{"migrate", "migrate", "apply goose migrations", migrate},
	{"job", "job <name>", "run one scheduled job", job},
	{"import-csv", "import-csv <importer> <file>", "import a CSV export", importCSV},
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	for _, c := range commands {
		if c.name != os.Args[1] {
			continue
		}
		if err := c.run(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "lifeos %s: %v\n", c.name, err)
			os.Exit(1)
		}
		return
	}
	fmt.Fprintf(os.Stderr, "lifeos: unknown command %q\n\n", os.Args[1])
	usage()
	os.Exit(2)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: lifeos <command> [args]")
	fmt.Fprintln(os.Stderr)
	for _, c := range commands {
		fmt.Fprintf(os.Stderr, "  %-30s %s\n", c.usage, c.summary)
	}
}

func serve(args []string) error     { return errNotImplemented }
func job(args []string) error       { return errNotImplemented }
func importCSV(args []string) error { return errNotImplemented }
