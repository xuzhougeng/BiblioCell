package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/egoist/mygo"

	"bibliocell/internal/library"
	"bibliocell/internal/settings"
)

const cliUsage = `BiblioCell command line — operates the real SQLite library directly.

Usage:
  BiblioCell [-data-dir DIR] <command> [args]

Commands:
  status                              database path, schema version, counts
  papers list [-q TEXT] [-screening S] [-source S] [-batch ID] [-needs-review] [-limit N] [-offset N]
                                      search papers (JSON)
  papers get ID                       one paper by ID
  papers add [-f FILE]                add one paper from JSON (stdin by default)
  papers import [-f FILE] [-batch ID] [-dry-run]
                                      atomically import a JSON array, skipping known papers
  papers update ID [-f FILE]          apply a JSON patch (stdin by default)
  papers attach ID [-type fulltext|supplement] FILE...
                                      copy files into the storage folder
  papers files ID                     files attached to a paper
  papers undo BATCH                   remove an import batch, keeping what a person worked on
  datasets list [-q TEXT] [-status S] [-limit N] [-offset N]
                                      datasets with repositories and papers
  datasets get ID                     one dataset by ID
  datasets add [-f FILE]              add a dataset from JSON, or add links to a known one
  datasets update ID [-f FILE]        apply a JSON patch
  datasets link ID [-f FILE]          link a paper from JSON, with evidence
  evidence list [-paper ID] [-dataset ID]
                                      recorded claims and their verdicts
  evidence add [-f FILE]              record a JSON array of claims
  sources list                        connector configurations and last runs
  sources set ID [-enabled=BOOL] [-query TEXT] [-max N] [-provider geo|sra|ena]
  sources sync ID                     fetch one source now
  sources auto [-enabled=BOOL] [-hours N]
                                      show or set the schedule the desktop app runs
  settings get                        current settings.json
  settings set [-storage DIR] [-font-size N] [-mcp-token TOKEN]
  mcp                                 MCP endpoint and bearer token
  serve [-addr HOST:PORT]             run the MCP server without a window, until interrupted
  sql STATEMENT                       run raw SQL (SELECT prints rows, others print affected)

Data directory: -data-dir, else $BIBLIOCELL_DATA_DIR, else the app's user data directory.
Output is JSON. Settings changes take effect on the next app start.
`

var cliCommands = map[string]bool{
	"status": true, "papers": true, "datasets": true, "sources": true,
	"settings": true, "mcp": true, "sql": true, "help": true, "evidence": true, "serve": true,
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
	// Settings require an absolute storage path, so the directory must be too.
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
		if c.store, err = library.Open(c.settings.Get().DatabasePath()); err != nil {
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
		return c.datasets(ctx, args)
	case "evidence":
		return c.evidence(ctx, args)
	case "serve":
		return c.serve(args)
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
		var filter library.PaperFilter
		flags := flag.NewFlagSet("papers list", flag.ContinueOnError)
		flags.StringVar(&filter.Query, "q", "", "text matched against title, DOI, authors, species, organs, tags")
		flags.StringVar(&filter.Screening, "screening", "", "pending, included, excluded, or uncertain")
		flags.StringVar(&filter.Source, "source", "", "only papers from this source")
		flags.StringVar(&filter.BatchID, "batch", "", "only papers from this import batch")
		flags.BoolVar(&filter.NeedsReview, "needs-review", false, "only papers a person has not settled")
		flags.IntVar(&filter.Limit, "limit", 0, "maximum papers to print, 0 for all")
		flags.IntVar(&filter.Offset, "offset", 0, "matching papers to skip")
		if err := flags.Parse(rest); err != nil {
			return err
		}
		papers, err := c.store.ListPapers(ctx)
		if err != nil {
			return err
		}
		matches, _ := library.FilterPapers(papers, filter)
		return c.print(matches, nil)
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
		batch := flags.String("batch", "", "batch to join; generated when empty")
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
		return c.print(c.store.ImportPapers(ctx, papers, *batch, *dryRun))
	case "update":
		if len(rest) == 0 {
			return errors.New("usage: papers update ID [-f FILE]")
		}
		var patch library.PaperPatch
		if err := c.readJSON(rest[1:], &patch); err != nil {
			return err
		}
		return c.print(c.store.UpdatePaper(ctx, rest[0], patch))
	case "attach":
		if len(rest) == 0 {
			return errors.New("usage: papers attach ID [-type fulltext|supplement] FILE...")
		}
		flags := flag.NewFlagSet("papers attach", flag.ContinueOnError)
		resourceType := flags.String("type", "fulltext", "fulltext or supplement")
		if err := flags.Parse(rest[1:]); err != nil {
			return err
		}
		if flags.NArg() == 0 {
			return errors.New("usage: papers attach ID [-type fulltext|supplement] FILE...")
		}
		files := []library.PaperFile{}
		for _, source := range flags.Args() {
			file, err := c.store.ImportPaperFile(ctx, rest[0], source, *resourceType)
			if err != nil {
				return err
			}
			files = append(files, file)
		}
		return c.print(files, nil)
	case "files":
		if len(rest) != 1 {
			return errors.New("usage: papers files ID")
		}
		all, err := c.store.ListPaperFiles(ctx)
		if err != nil {
			return err
		}
		files := []library.PaperFile{}
		for _, file := range all {
			if file.PaperID == rest[0] {
				files = append(files, file)
			}
		}
		return c.print(files, nil)
	case "undo":
		if len(rest) != 1 {
			return errors.New("usage: papers undo BATCH")
		}
		return c.print(c.store.UndoImport(ctx, rest[0]))
	}
	return errors.New("usage: papers list|get|add|import|update|attach|files|undo")
}

func (c *cli) datasets(ctx context.Context, args []string) error {
	sub, rest := split(args)
	switch sub {
	case "", "list":
		var filter library.DatasetFilter
		flags := flag.NewFlagSet("datasets list", flag.ContinueOnError)
		flags.StringVar(&filter.Query, "q", "", "text matched against title, accession, species, organs")
		flags.StringVar(&filter.Status, "status", "", "pending, identified, or verified")
		flags.IntVar(&filter.Limit, "limit", 0, "maximum datasets to print, 0 for all")
		flags.IntVar(&filter.Offset, "offset", 0, "matching datasets to skip")
		if err := flags.Parse(rest); err != nil {
			return err
		}
		datasets, err := c.store.ListDatasets(ctx)
		if err != nil {
			return err
		}
		matches, _ := library.FilterDatasets(datasets, filter)
		return c.print(matches, nil)
	case "get":
		if len(rest) != 1 {
			return errors.New("usage: datasets get ID")
		}
		return c.print(c.store.GetDataset(ctx, rest[0]))
	case "add":
		var input library.DatasetInput
		if err := c.readJSON(rest, &input); err != nil {
			return err
		}
		return c.print(c.store.AddDataset(ctx, input))
	case "update":
		if len(rest) == 0 {
			return errors.New("usage: datasets update ID [-f FILE]")
		}
		var patch library.DatasetPatch
		if err := c.readJSON(rest[1:], &patch); err != nil {
			return err
		}
		return c.print(c.store.UpdateDataset(ctx, rest[0], patch))
	case "link":
		if len(rest) == 0 {
			return errors.New("usage: datasets link ID [-f FILE]")
		}
		var link library.PaperLinkInput
		if err := c.readJSON(rest[1:], &link); err != nil {
			return err
		}
		return c.print(c.store.LinkDataset(ctx, rest[0], link))
	}
	return errors.New("usage: datasets list|get|add|update|link")
}

func (c *cli) evidence(ctx context.Context, args []string) error {
	sub, rest := split(args)
	switch sub {
	case "list":
		flags := flag.NewFlagSet("evidence list", flag.ContinueOnError)
		paper := flags.String("paper", "", "only claims about this paper")
		dataset := flags.String("dataset", "", "only claims about this dataset")
		if err := flags.Parse(rest); err != nil {
			return err
		}
		return c.print(c.store.ListEvidence(ctx, *paper, *dataset))
	case "add":
		var inputs []library.EvidenceInput
		if err := c.readJSON(rest, &inputs); err != nil {
			return err
		}
		return c.print(c.store.AddEvidence(ctx, inputs))
	}
	return errors.New("usage: evidence list|add")
}

// serve runs the MCP server without a window until the process is interrupted.
// The desktop app serves the same address, so only one of them can run on it.
func (c *cli) serve(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	address := flags.String("addr", library.MCPAddress, "loopback address to listen on")
	if err := flags.Parse(args); err != nil {
		return err
	}
	server, err := library.StartMCP(c.store, *address, c.settings.Get().MCPToken)
	if err != nil {
		return fmt.Errorf("start MCP server: %w", err)
	}
	if err := c.print(map[string]string{"endpoint": server.Endpoint}, nil); err != nil {
		return err
	}
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-stop.Done()
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	return server.Close(shutdown)
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
	case "auto":
		current, err := c.store.SourceAutomation(ctx)
		if err != nil {
			return err
		}
		flags := flag.NewFlagSet("sources auto", flag.ContinueOnError)
		enabled := flags.Bool("enabled", current.Enabled, "update enabled sources while the desktop app runs")
		hours := flags.Int("hours", current.IntervalHours, "hours between updates, 1-720")
		if err := flags.Parse(rest); err != nil {
			return err
		}
		return c.print(c.store.SaveSourceAutomation(ctx, *enabled, *hours))
	}
	return errors.New("usage: sources list|set|sync|auto")
}

func (c *cli) configure(args []string) error {
	sub, rest := split(args)
	switch sub {
	case "get":
		return c.print(c.settings.Get(), nil)
	case "set":
		config := c.settings.Get()
		flags := flag.NewFlagSet("settings set", flag.ContinueOnError)
		flags.StringVar(&config.StoragePath, "storage", config.StoragePath, "absolute folder for the SQLite database and paper files, used on next start")
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
