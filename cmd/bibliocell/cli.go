package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/egoist/mygo"

	"bibliocell"
	"bibliocell/internal/agent"
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
  papers delete ID                    move a paper to the recycle bin; restore from Settings
  papers add [-f FILE]                add one paper from JSON (stdin by default)
  papers import [-f FILE] [-batch ID] [-dry-run]
                                      atomically import a JSON array, skipping known papers
  papers update ID [-f FILE]          apply a JSON patch (stdin by default)
  papers attach ID [-type fulltext|supplement] FILE...
                                      copy files into the storage folder
  papers files ID                     files attached to a paper
  papers lookup DOI|PMID              fetch a bibliographic record from Crossref / Europe PMC, saving nothing
  papers duplicates [-limit N] [-offset N]
                                      groups of papers with matching titles, with a suggested one to keep
  papers published [-limit N] [-offset N]
                                      ask Crossref which preprints have a journal version, saving nothing
  papers merge KEEP DUPLICATE -reason TEXT
                                      fold DUPLICATE (e.g. the preprint) into KEEP and remove it
  papers merged ID                    papers merged into ID
  papers fetch ID                     download an open-access PDF through PMC or Unpaywall
  papers undo BATCH                   remove an import batch, keeping what a person worked on
  datasets list [-q TEXT] [-status S] [-limit N] [-offset N]
                                      datasets with repositories and papers
  datasets get ID                     one dataset by ID
  datasets add [-f FILE]              add a dataset from JSON, or add links to a known one
  datasets update ID [-f FILE]        apply a JSON patch
  datasets link ID [-f FILE]          link a paper from JSON, with evidence
  datasets manifest ID               show file manifest, samples and reference versions
  datasets prepare ID                create the dataset's storage directories
  datasets discover ID GSE            discover GEO series/sample supplementary files
  datasets export-manifest ID         write manifest.json in the dataset directory
  datasets save-file ID [-f FILE]      register/update a file from JSON (no download)
  datasets verify-file ID FILE_ID     read a registered local file and check SHA-256
  evidence list [-paper ID] [-dataset ID]
                                      recorded claims and their verdicts
  evidence add [-f FILE]              record a JSON array of claims
  export KIND [-o FILE] [-q TEXT] [-screening S] [-status S] [-group-by species|organ|modality] [-title TEXT]
                                      write an export; KIND is bibtex, papers-csv, report, datasets-csv,
                                      datasets-summary, download-script, or accessions. Without -o the
                                      file goes to stdout; with -o its path and record count print as JSON
  sources list                        connector configurations and last runs
  sources set ID [-enabled=BOOL] [-query TEXT] [-max N] [-provider geo|sra|ena]
  sources sync ID                     fetch one source now
  sources auto [-enabled=BOOL] [-hours N]
                                      show or set the schedule the desktop app runs
  settings get                        current settings.json
  settings set [-storage DIR] [-font-size N] [-mcp-token TOKEN] [-ncbi-api-key KEY] [-contact-email EMAIL]
               [-ai-protocol anthropic|openai] [-ai-base-url URL] [-ai-api-key KEY] [-ai-model NAME] [-ai-max-steps N] [-ai-effort LEVEL]
  mcp                                 MCP endpoint and bearer token
  serve [-addr HOST:PORT]             run the MCP server without a window, until interrupted
  web [-addr HOST:PORT] [-dist DIR] [-admin-password-file FILE]
                                      serve the library as a website: read-only for everyone,
                                      editable after signing in with the admin password
                                      ($BIBLIOCELL_ADMIN_PASSWORD or the file); runs the
                                      scheduled source updates while it is up
  sql STATEMENT                       run raw SQL (SELECT prints rows, others print affected)
  db version                          database schema version, applied and pending migrations
  db migrate                          upgrade the database to this build's schema (backs up first)
  agent [-max-steps N] [-system TEXT] PROMPT
                                      run the built-in agent once with the configured model;
                                      tool calls go to stderr, the result to stdout

Data directory: -data-dir, else $BIBLIOCELL_DATA_DIR, else the app's user data directory.
Output is JSON. Settings changes take effect on the next app start.
`

var cliCommands = map[string]bool{
	"status": true, "papers": true, "datasets": true, "sources": true,
	"settings": true, "mcp": true, "sql": true, "help": true, "evidence": true, "serve": true, "agent": true, "db": true, "export": true, "web": true,
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
	// `db` inspects or migrates the file itself: Open would migrate it first.
	if command != "settings" && command != "db" {
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
	case "export":
		return c.export(ctx, args)
	case "serve":
		return c.serve(args)
	case "web":
		return c.web(args)
	case "agent":
		return c.agent(ctx, args)
	case "sources":
		return c.sources(ctx, args)
	case "settings":
		return c.configure(args)
	case "mcp":
		return c.print(map[string]string{
			"endpoint": "http://" + library.MCPAddress + "/mcp", "token": c.settings.Get().MCPToken,
		}, nil)
	case "db":
		return c.db(args)
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
		flags.StringVar(&filter.Screening, "screening", "", "pending, included, excluded, uncertain, or irrelevant")
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
	case "delete":
		if len(rest) != 1 || strings.TrimSpace(rest[0]) == "" {
			return errors.New("usage: papers delete ID")
		}
		if err := c.store.DeletePaper(ctx, rest[0]); err != nil {
			return err
		}
		return c.print(map[string]any{"id": rest[0], "deleted": true}, nil)
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
	case "duplicates", "published":
		flags := flag.NewFlagSet("papers "+sub, flag.ContinueOnError)
		flags.SetOutput(c.stderr)
		limit := flags.Int("limit", 0, "page size")
		offset := flags.Int("offset", 0, "items to skip")
		if err := flags.Parse(rest); err != nil {
			return err
		}
		if sub == "duplicates" {
			return c.print(c.store.FindDuplicates(ctx, cmp.Or(*limit, 50), *offset))
		}
		return c.print(c.store.FindPublishedVersions(ctx, cmp.Or(*limit, 20), *offset, sourceCredentials(c.settings)))
	case "merge":
		flags := flag.NewFlagSet("papers merge", flag.ContinueOnError)
		flags.SetOutput(c.stderr)
		reason := flags.String("reason", "", "why the papers are the same work")
		if len(rest) < 2 {
			return errors.New("usage: papers merge KEEP DUPLICATE -reason TEXT")
		}
		if err := flags.Parse(rest[2:]); err != nil {
			return err
		}
		return c.print(c.store.MergePapers(ctx, library.MergeInput{KeepID: rest[0], MergeID: rest[1], Reason: *reason, Via: "CLI"}))
	case "merged":
		if len(rest) != 1 {
			return errors.New("usage: papers merged ID")
		}
		return c.print(c.store.ListMergedPapers(ctx, rest[0]))
	case "lookup":
		if len(rest) != 1 || strings.TrimSpace(rest[0]) == "" {
			return errors.New("usage: papers lookup DOI|PMID")
		}
		return c.print(library.LookupIdentifier(ctx, rest[0], sourceCredentials(c.settings)))
	case "fetch":
		if len(rest) != 1 {
			return errors.New("usage: papers fetch ID")
		}
		return c.print(c.store.FetchFulltext(ctx, rest[0], sourceCredentials(c.settings)))
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
	return errors.New("usage: papers list|get|add|import|update|delete|attach|lookup|duplicates|published|merge|merged|fetch|files|undo")
}

func (c *cli) datasets(ctx context.Context, args []string) error {
	sub, rest := split(args)
	switch sub {
	case "manifest", "prepare", "export-manifest":
		if len(rest) != 1 {
			return fmt.Errorf("usage: datasets %s ID", sub)
		}
		switch sub {
		case "manifest":
			return c.print(c.store.DatasetManifest(ctx, rest[0]))
		case "prepare":
			return c.print(c.store.PrepareDatasetDirectory(ctx, rest[0]))
		default:
			return c.print(c.store.ExportDatasetManifest(ctx, rest[0]))
		}
	case "discover", "verify-file":
		if len(rest) != 2 {
			return fmt.Errorf("usage: datasets %s ID ACCESSION_OR_FILE_ID", sub)
		}
		if sub == "discover" {
			return c.print(c.store.DiscoverGEOFiles(ctx, rest[0], rest[1]))
		}
		return c.print(c.store.VerifyDatasetAsset(ctx, rest[0], rest[1]))
	case "save-file":
		if len(rest) == 0 {
			return errors.New("usage: datasets save-file ID [-f FILE]")
		}
		var asset library.DatasetAsset
		if err := c.readJSON(rest[1:], &asset); err != nil {
			return err
		}
		asset.DatasetID = rest[0]
		return c.print(c.store.SaveDatasetAsset(ctx, asset))
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

// export writes one export file from the real library. Paper filters narrow
// the papers (and the report); -status narrows the datasets. Dataset formats
// still receive every paper so links resolve to DOIs, and paper formats
// receive every dataset so the accession column is complete.
func (c *cli) export(ctx context.Context, args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return errors.New("usage: export KIND [-o FILE] [-q TEXT] [-screening S] [-status S] [-group-by DIM] [-title TEXT]; KIND is one of " + strings.Join(library.ExportKinds, ", "))
	}
	req := library.ExportRequest{Kind: args[0]}
	var paperFilter library.PaperFilter
	var datasetFilter library.DatasetFilter
	flags := flag.NewFlagSet("export", flag.ContinueOnError)
	out := flags.String("o", "", "file to write; stdout when omitted")
	flags.StringVar(&paperFilter.Query, "q", "", "text matched against papers (paper formats) or datasets (dataset formats)")
	flags.StringVar(&paperFilter.Screening, "screening", "", "only papers with this decision: pending, included, excluded, uncertain, or irrelevant (left out unless asked for)")
	flags.StringVar(&datasetFilter.Status, "status", "", "only datasets with this status: pending, identified, or verified")
	flags.StringVar(&req.GroupBy, "group-by", "species", "datasets-summary dimension: species, organ, or modality")
	flags.StringVar(&req.Title, "title", "", "report title")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	papers, err := c.store.ListPapers(ctx)
	if err != nil {
		return err
	}
	datasets, err := c.store.ListDatasets(ctx)
	if err != nil {
		return err
	}
	req.Papers, req.Datasets = papers, datasets
	switch req.Kind {
	case library.ExportDatasetsCSV, library.ExportDatasetSummary, library.ExportDownloadScript, library.ExportAccessions:
		datasetFilter.Query = paperFilter.Query
		req.Datasets, _ = library.FilterDatasets(datasets, datasetFilter)
	default:
		req.Papers, _ = library.FilterPapers(papers, paperFilter)
		if datasetFilter.Status != "" {
			req.Datasets, _ = library.FilterDatasets(datasets, datasetFilter)
		}
	}
	file, err := library.Export(req, time.Now())
	if err != nil {
		return err
	}
	if *out == "" {
		_, err := io.WriteString(c.stdout, file.Content)
		return err
	}
	if err := os.WriteFile(*out, []byte(file.Content), 0o644); err != nil {
		return err
	}
	path, _ := filepath.Abs(*out)
	return c.print(map[string]any{"path": path, "count": file.Count}, nil)
}

// serve runs the MCP server without a window until the process is interrupted.
// The desktop app serves the same address, so only one of them can run on it.
func (c *cli) serve(args []string) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	address := flags.String("addr", library.MCPAddress, "loopback address to listen on")
	if err := flags.Parse(args); err != nil {
		return err
	}
	server, err := library.StartMCP(c.store, *address, c.settings.Get().MCPToken, sourceCredentials(c.settings))
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

// web serves the library over HTTP until the process is interrupted. The
// address defaults to loopback: pass -addr 0.0.0.0:8080 to serve the LAN.
func (c *cli) web(args []string) error {
	flags := flag.NewFlagSet("web", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	address := flags.String("addr", "127.0.0.1:8080", "address to listen on")
	dist := flags.String("dist", "", "frontend build folder (default: the embedded build, else ./dist)")
	passwordFile := flags.String("admin-password-file", "", "file holding the admin password (default: $BIBLIOCELL_ADMIN_PASSWORD)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	password, err := readAdminPassword(os.Getenv("BIBLIOCELL_ADMIN_PASSWORD"), *passwordFile, os.ReadFile)
	if err != nil {
		return err
	}
	frontend, err := webFrontend(*dist, bibliocell.WebDist)
	if err != nil {
		return err
	}
	service := &Library{store: c.store, settings: c.settings, events: newEventHub()}
	c.store.OnPapersHarvested(service.checkHarvested)
	server, err := newWebServer(service, frontend, password, mygo.App.Version())
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", *address, err)
	}
	httpServer := &http.Server{Handler: server.handler(), ReadHeaderTimeout: 10 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpServer.Serve(listener) }()
	// The website is the library's long-running host, so the schedule runs here.
	stopUpdates := c.store.StartSourceUpdates(sourceCredentials(c.settings), func() {
		automation, err := c.store.SourceAutomation(context.Background())
		if err != nil {
			fmt.Fprintf(c.stderr, "read source automation: %v\n", err)
			return
		}
		service.announceSources(automation)
	})
	defer stopUpdates()
	if password == "" {
		fmt.Fprintln(c.stderr, "no admin password: set BIBLIOCELL_ADMIN_PASSWORD or -admin-password-file to allow signing in; the site is read-only")
	}
	if err := c.print(map[string]any{"url": "http://" + listener.Addr().String() + "/", "admin": password != ""}, nil); err != nil {
		return err
	}
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	select {
	case <-stop.Done():
	case err := <-serveErr:
		return err
	}
	shutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	return httpServer.Shutdown(shutdown)
}

// agent runs one conversation turn against the configured model, with the
// MCP tools connected in-process.
func (c *cli) agent(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("agent", flag.ContinueOnError)
	maxSteps := flags.Int("max-steps", 0, "completions per run, 0 for the configured value")
	system := flags.String("system", "", "system prompt replacing the built-in one")
	if err := flags.Parse(args); err != nil {
		return err
	}
	prompt := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if prompt == "" {
		return errors.New("usage: agent [-max-steps N] [-system TEXT] PROMPT")
	}
	profile, err := c.settings.Get().ModelFor(settings.TaskChat)
	if err != nil {
		return fmt.Errorf("AI settings: %w (run settings set -ai-model ...)", err)
	}
	config := agent.Normalize(profile.Config)
	provider, err := agent.NewProvider(config)
	if err != nil {
		return fmt.Errorf("AI settings: %w (run settings set -ai-model ...)", err)
	}
	local, err := library.ConnectLocalMCP(ctx, c.store, sourceCredentials(c.settings))
	if err != nil {
		return err
	}
	defer local.Close()
	if *maxSteps <= 0 {
		*maxSteps = config.MaxSteps
	}
	result, err := agent.Run(ctx, provider, agent.MCPTools{Session: local.Session}, []agent.Message{{Role: "user", Content: prompt}}, agent.Options{
		System: *system, MaxSteps: *maxSteps,
		OnEvent: func(event agent.Event) {
			switch event.Type {
			case "tool_call":
				fmt.Fprintf(c.stderr, "[%d] %s %s\n", event.Step, event.ToolCall.Name, event.ToolCall.Arguments)
			case "tool_result":
				status := "ok"
				if event.IsError {
					status = "error"
				}
				fmt.Fprintf(c.stderr, "[%d] %s -> %s, %d bytes\n", event.Step, event.ToolCall.Name, status, len(event.Output))
			}
		},
	})
	if err != nil {
		return err
	}
	return c.print(result, nil)
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
		return c.print(c.store.SyncSource(ctx, rest[0], sourceCredentials(c.settings)))
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
			MaxResults: current.MaxResults, Provider: current.Provider, Proxy: current.Proxy,
		}
		flags := flag.NewFlagSet("sources set", flag.ContinueOnError)
		flags.BoolVar(&config.Enabled, "enabled", config.Enabled, "run this source when syncing")
		flags.StringVar(&config.Query, "query", config.Query, "search expression")
		flags.IntVar(&config.MaxResults, "max", config.MaxResults, "results per sync, 1-100")
		flags.StringVar(&config.Provider, "provider", config.Provider, "geo, sra, or ena for sequence-repositories")
		flags.StringVar(&config.Proxy, "proxy", config.Proxy, "inherit to follow the global proxy setting, or direct")
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
		flags.StringVar(&config.NCBIAPIKey, "ncbi-api-key", config.NCBIAPIKey, "NCBI API key for PubMed, GEO and SRA")
		flags.StringVar(&config.ContactEmail, "contact-email", config.ContactEmail, "contact email for Crossref's polite pool")
		flags.StringVar(&config.Proxy.Mode, "proxy-mode", config.Proxy.Mode, "outbound proxy: system (default, follows the system's settings), direct, or custom")
		flags.StringVar(&config.Proxy.URL, "proxy-url", config.Proxy.URL, "proxy for -proxy-mode custom: http://, https:// or socks5://[user:pass@]host:port")
		// The -ai-* flags edit the chat model's profile, creating "default" when there is none.
		index := -1
		for i, profile := range config.Models {
			if profile.ID == config.Tasks.Chat || (config.Tasks.Chat == "" && i == 0) {
				index = i
				break
			}
		}
		ai := settings.ModelProfile{ID: "default", Name: "默认模型"}
		if index >= 0 {
			ai = config.Models[index]
		}
		flags.StringVar(&ai.Name, "ai-name", ai.Name, "display name of the chat model profile")
		flags.StringVar(&ai.Config.Protocol, "ai-protocol", ai.Config.Protocol, "anthropic (Messages API) or openai (Chat Completions)")
		flags.StringVar(&ai.Config.BaseURL, "ai-base-url", ai.Config.BaseURL, "model service root, empty for the protocol's public endpoint")
		flags.StringVar(&ai.Config.APIKey, "ai-api-key", ai.Config.APIKey, "model service API key")
		flags.StringVar(&ai.Config.Model, "ai-model", ai.Config.Model, "model name")
		flags.IntVar(&ai.Config.MaxTokens, "ai-max-tokens", ai.Config.MaxTokens, "output tokens per completion, 0 for the default")
		flags.IntVar(&ai.Config.MaxSteps, "ai-max-steps", ai.Config.MaxSteps, "completions per run, 0 for 20")
		flags.StringVar(&ai.Config.Effort, "ai-effort", ai.Config.Effort, "thinking effort: low, medium, high, xhigh, max; empty for the service default")
		flags.StringVar(&ai.Config.Proxy, "ai-proxy", ai.Config.Proxy, "inherit to follow the global proxy setting, or direct")
		if err := flags.Parse(rest); err != nil {
			return err
		}
		if index >= 0 {
			config.Models[index] = ai
		} else if ai.Config != (agent.Config{}) {
			config.Models = append(config.Models, ai)
		}
		if err := c.settings.Save(config); err != nil {
			return err
		}
		return c.print(c.settings.Get(), nil)
	}
	return errors.New("usage: settings get|set")
}

// readJSON decodes the -f FILE flag or stdin into target.
// db reports or upgrades the schema version without going through Open first.
func (c *cli) db(args []string) error {
	path := c.settings.Get().DatabasePath()
	sub, _ := split(args)
	switch sub {
	case "version":
		return c.print(library.Inspect(path))
	case "migrate":
		before, err := library.Inspect(path)
		if err != nil {
			return err
		}
		upgrade, err := library.Migrate(path)
		if err != nil {
			return err
		}
		after, err := library.Inspect(path)
		if err != nil {
			return err
		}
		return c.print(map[string]any{
			"databasePath": path, "before": before.Version, "after": after.Version, "migrated": upgrade != nil,
			"backup": func() string {
				if upgrade != nil {
					return upgrade.Backup
				}
				return ""
			}(),
		}, nil)
	}
	return errors.New("usage: db version | db migrate")
}

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
