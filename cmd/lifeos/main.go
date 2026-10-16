// Command lifeos is the single LifeOS binary: REST API, admin pages, MCP
// server, CLI and jobs. Subcommands are thin adapters over internal/service.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	lifeosmcp "github.com/Gautam-J/lifeos-core/internal/mcp"
	"github.com/Gautam-J/lifeos-core/internal/version"
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
	{"instructions", "instructions", "print the MCP server instructions, to paste into a client", printInstructions},
	{"version", "version", "print the release of this binary", printVersion},
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

func printVersion(args []string) error {
	writeVersion(os.Stdout)
	return nil
}

func writeVersion(w io.Writer) {
	fmt.Fprintln(w, "lifeos", version.Version)
}

func printInstructions(args []string) error {
	writeInstructions(os.Stdout)
	return nil
}

func writeInstructions(w io.Writer) {
	fmt.Fprintln(w, lifeosmcp.Instructions())
}

func job(args []string) error       { return errNotImplemented }
func importCSV(args []string) error { return errNotImplemented }
