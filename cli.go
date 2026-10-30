package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"

	"bibliocell/internal/library"
	"bibliocell/internal/settings"
)

const cliUsage = `BiblioCell command line — operates the real SQLite library directly.

Usage:
  BiblioCell [-data-dir DIR] <command> [args]

Commands:
  status                              database path, schema version, counts
  papers list [-q TEXT] [-limit N]    search papers (JSON)
  papers get ID                       one paper by ID
  papers add [-f FILE]                add one paper from JSON (stdin by default)
  papers import [-f FILE] [-dry-run]  atomically import a JSON array of papers
  papers update ID [-f FILE]          apply a JSON patch (stdin by default)
  datasets list                       datasets with repositories and papers
  sources list                        connector configurations and last runs
  sources set ID [-enabled=BOOL] [-query TEXT] [-max N] [-provider geo|sra|ena]
  sources sync ID                     fetch one source now
  settings get                        current settings.json
  settings set [-db PATH] [-font-size N] [-mcp-token TOKEN]
  mcp                                 MCP endpoint and bearer token
  sql STATEMENT                       run raw SQL (SELECT prints rows, others print affected)

Data directory: -data-dir, else $BIBLIOCELL_DATA_DIR, else the app's user data directory.
Output is JSON. Settings changes take effect on the next app start.
`

var cliCommands = map[string]bool{
	"status": true, "papers": true, "datasets": true, "sources": true,
	"settings": true, "mcp": true, "sql": true, "help": true,
}

// isCLICommand tells a command line invocation from a desktop launch, whose
// arguments (deep links, files) must reach the window instead.
func isCLICommand(arg string) bool {
	switch arg {
	case "-h", "--help", "-data-dir", "--data-dir":
		return true
	}
	return cliCommands[arg] || strings.HasPrefix(arg, "-data-dir=") || strings.HasPrefix(arg, "--data-dir=")
}

// cli holds what every subcommand needs.
type cli struct {
	dataDir  string
	stdin    io.Reader
	stdout   io.Writer
	stderr   io.Writer
	settings *settings.Store
	store    *library.Store
}

// runCLI executes one command and returns the process exit code.
func runCLI(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	c := &cli{stdin: stdin, stdout: stdout, stderr: stderr}
	flags := flag.NewFlagSet("BiblioCell", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&c.dataDir, "data-dir", os.Getenv("BIBLIOCELL_DATA_DIR"), "application data directory")
	flags.Usage = func() { fmt.Fprint(stderr, cliUsage) }
	if err := flags.Parse(args); err != nil {
		return 2
	}
	rest := flags.Args()
	if len(rest) == 0 || rest[0] == "help" {
		fmt.Fprint(stdout, cliUsage)
		return 0
	}
	if c.dataDir == "" {
		dir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			fmt.Fprintf(stderr, "locate data directory: %v\n", err)
			return 1
		}
		c.dataDir = dir
	}
	// Settings require absolute database paths, so the directory must be too.
	if dir, err := filepath.Abs(c.dataDir); err == nil {
		c.dataDir = dir
	}
	if err := c.run(rest[0], rest[1:]); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func (c *cli) run(command string, args []string) error {
	if !cliCommands[command] {
		return fmt.Errorf("unknown command %q; run with -h for usage", command)
	}
	var err error
	if c.settings, err = openSettings(c.dataDir); err != nil {
		fmt.Fprintf(c.stderr, "using default settings: %v\n", err)
	}
	if command != "settings" {
		if c.store, err = library.Open(c.settings.Get().DatabasePath); err != nil {
			return err
		}
		defer c.store.Close()
	}
	ctx := context.Background()
	switch command {
	case "status":
		return c.print(c.store.Status())
	case "papers":
		return c.papers(ctx, args)
	case "datasets":
		return c.print(c.store.ListDatasets(ctx))
	case "sources":
		return c.sources(ctx, args)
	case "settings":
		return c.configure(args)
	case "mcp":
		return c.print(map[string]string{
			"endpoint": "http://" + library.MCPAddress + "/mcp", "token": c.settings.Get().MCPToken,
		}, nil)
	case "sql":
		if len(args) != 1 {
			return errors.New("usage: sql STATEMENT")
		}
		rows, affected, err := c.store.SQL(ctx, args[0])
		if err != nil {
			return err
		}
		if rows == nil {
			return c.print(map[string]int64{"affected": affected}, nil)
		}
		return c.print(rows, nil)
	}
	return nil
}

func (c *cli) papers(ctx context.Context, args []string) error {
	sub, rest := split(args)
	switch sub {
	case "list":
		flags := flag.NewFlagSet("papers list", flag.ContinueOnError)
		query := flags.String("q", "", "text matched against title, DOI, authors, species, organs, tags")
		limit := flags.Int("limit", 0, "maximum papers to print, 0 for all")
		if err := flags.Parse(rest); err != nil {
			return err
		}
		papers, err := c.store.ListPapers(ctx)
		if err != nil {
			return err
		}
		return c.print(library.FilterPapers(papers, *query, *limit), nil)
	case "get":
		if len(rest) != 1 {
			return errors.New("usage: papers get ID")
		}
		return c.print(c.store.GetPaper(ctx, rest[0]))
	case "add":
		var paper library.Paper
		if err := c.readJSON(rest, &paper); err != nil {
			return err
		}
		if paper.Source == "" {
			paper.Source = "CLI"
		}
		return c.print(c.store.AddPaper(ctx, paper))
	case "import":
		var papers []library.Paper
		flags := flag.NewFlagSet("papers import", flag.ContinueOnError)
		dryRun := flags.Bool("dry-run", false, "validate the batch and roll back without writing")
		file := flags.String("f", "", "JSON file holding an array of papers; stdin when omitted")
		if err := flags.Parse(rest); err != nil {
			return err
		}
		if err := c.decode(*file, &papers); err != nil {
			return err
		}
		for i := range papers {
			if papers[i].Source == "" {
				papers[i].Source = "CLI"
			}
		}
		return c.print(c.store.ImportPapers(ctx, papers, *dryRun))
	case "update":
		if len(rest) == 0 {
			return errors.New("usage: papers update ID [-f FILE]")
		}
		var patch library.PaperPatch
		if err := c.readJSON(rest[1:], &patch); err != nil {
			return err
		}
		return c.print(c.store.UpdatePaper(ctx, rest[0], patch))
	}
	return errors.New("usage: papers list|get|add|import|update")
}

func (c *cli) sources(ctx context.Context, args []string) error {
	sub, rest := split(args)
	switch sub {
	case "list":
		return c.print(c.store.ListSources(ctx))
	case "sync":
		if len(rest) != 1 {
			return errors.New("usage: sources sync ID")
		}
		return c.print(c.store.SyncSource(ctx, rest[0]))
	case "set":
		if len(rest) == 0 {
			return errors.New("usage: sources set ID [flags]")
		}
		current, err := c.store.SourceStatus(ctx, rest[0])
		if err != nil {
			return err
		}
		config := library.SourceConfig{
			ID: current.ID, Enabled: current.Enabled, Query: current.Query,
			MaxResults: current.MaxResults, Provider: current.Provider,
		}
		flags := flag.NewFlagSet("sources set", flag.ContinueOnError)
		flags.BoolVar(&config.Enabled, "enabled", config.Enabled, "run this source when syncing")
		flags.StringVar(&config.Query, "query", config.Query, "search expression")
		flags.IntVar(&config.MaxResults, "max", config.MaxResults, "results per sync, 1-100")
		flags.StringVar(&config.Provider, "provider", config.Provider, "geo, sra, or ena for sequence-repositories")
		if err := flags.Parse(rest[1:]); err != nil {
			return err
		}
		return c.print(c.store.SaveSourceConfig(ctx, config))
	}
	return errors.New("usage: sources list|set|sync")
}

func (c *cli) configure(args []string) error {
	sub, rest := split(args)
	switch sub {
	case "get":
		return c.print(c.settings.Get(), nil)
	case "set":
		config := c.settings.Get()
		flags := flag.NewFlagSet("settings set", flag.ContinueOnError)
		flags.StringVar(&config.DatabasePath, "db", config.DatabasePath, "absolute SQLite path, used on next start")
		flags.IntVar(&config.FontSize, "font-size", config.FontSize, "interface font size, 12-20")
		flags.StringVar(&config.MCPToken, "mcp-token", config.MCPToken, "MCP bearer token, used on next start")
		if err := flags.Parse(rest); err != nil {
			return err
		}
		if err := c.settings.Save(config); err != nil {
			return err
		}
		return c.print(c.settings.Get(), nil)
	}
	return errors.New("usage: settings get|set")
}

// readJSON decodes the -f FILE flag or stdin into target.
func (c *cli) readJSON(args []string, target any) error {
	flags := flag.NewFlagSet("json", flag.ContinueOnError)
	file := flags.String("f", "", "JSON file; stdin when omitted")
	if err := flags.Parse(args); err != nil {
		return err
	}
	return c.decode(*file, target)
}

func (c *cli) decode(file string, target any) error {
	reader := c.stdin
	if file != "" {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		reader = f
	}
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	return nil
}

// print writes value as indented JSON unless err is set.
func (c *cli) print(value any, err error) error {
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func split(args []string) (string, []string) {
	if len(args) == 0 {
		return "", nil
	}
	return args[0], args[1:]
}
