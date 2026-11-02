package library

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const MCPAddress = "127.0.0.1:37841"
const paperSchemaURI = "bibliocell://schema/paper"
const databaseSchemaURI = "bibliocell://schema/sql"

// MCPServer is the loopback-only endpoint used by external AI clients.
type MCPServer struct {
	Endpoint string
	server   *http.Server
}

type emptyInput struct{}

type deletePaperOutput struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

type listPapersOutput struct {
	Papers []Paper `json:"papers"`
	Total  int     `json:"total"`
}

type listDatasetsOutput struct {
	Datasets []Dataset `json:"datasets"`
	Total    int       `json:"total"`
}

// mcpPageLimit applies the page size agents get when they do not ask for one.
func mcpPageLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	return min(limit, 500)
}

type paperInput struct {
	ID                string         `json:"id,omitempty" jsonschema:"Optional stable ID; generated when empty. Imports reject an existing ID"`
	Title             string         `json:"title" jsonschema:"Paper title; required and must not be blank"`
	DOI               string         `json:"doi,omitempty" jsonschema:"Bare DOI or DOI URL; normalized to lowercase and unique across papers"`
	PMID              string         `json:"pmid,omitempty" jsonschema:"PubMed identifier"`
	Authors           string         `json:"authors,omitempty" jsonschema:"Authors separated by semicolons"`
	Year              *int           `json:"year,omitempty" jsonschema:"Publication year from 1800 to 2200"`
	Journal           string         `json:"journal,omitempty"`
	PublicationStatus string         `json:"publicationStatus,omitempty" jsonschema:"unknown, preprint, published, or unpublished; defaults to unknown"`
	ArticleTypes      []string       `json:"articleTypes,omitempty" jsonschema:"Article types; each item is original, method, review, other, or a Chinese display label; a paper can have several"`
	Species           []SpeciesEntry `json:"species,omitempty" jsonschema:"Studied species; each entry needs a scientific name and an optional display label; a paper can cover several"`
	Organs            []string       `json:"organs,omitempty" jsonschema:"Organs or tissues such as root or leaf; a paper can cover several"`
	Modalities        []string       `json:"modalities,omitempty" jsonschema:"Modalities such as scRNA-seq, snRNA-seq, or scATAC-seq; a paper can use several"`
	Platforms         []string       `json:"platforms,omitempty" jsonschema:"Sequencing platforms such as 10x Genomics Chromium or BD Rhapsody; a paper can use several"`
	Abstract          string         `json:"abstract,omitempty"`
	Notes             string         `json:"notes,omitempty" jsonschema:"Personal research notes"`
	Tags              []string       `json:"tags,omitempty" jsonschema:"Personal topic tags"`
	ProposedScreening string         `json:"proposedScreening,omitempty" jsonschema:"Suggest included, excluded, or uncertain; screening itself stays pending until a person decides"`
	ProposedReason    string         `json:"proposedReason,omitempty" jsonschema:"Why; required when proposing excluded"`
	BatchID           string         `json:"batchId,omitempty" jsonschema:"Import batch to join, so undo_import removes this paper with it"`
	Reading           string         `json:"reading,omitempty" jsonschema:"unread, skimmed, or read; defaults to unread"`
	Starred           bool           `json:"starred,omitempty"`
}

func (input paperInput) paper() Paper {
	return Paper{
		ID: input.ID, Title: input.Title, DOI: input.DOI, PMID: input.PMID,
		Authors: input.Authors, Year: input.Year, Journal: input.Journal,
		PublicationStatus: input.PublicationStatus, ArticleTypes: input.ArticleTypes,
		Species: input.Species, Organs: input.Organs, Modalities: input.Modalities,
		Platforms: input.Platforms, Abstract: input.Abstract, Notes: input.Notes,
		Tags: input.Tags, ProposedScreening: input.ProposedScreening, ProposedReason: input.ProposedReason,
		BatchID: input.BatchID, Reading: input.Reading, Starred: input.Starred, Fulltext: "missing", Source: "MCP",
	}
}

type getPaperInput struct {
	ID string `json:"id" jsonschema:"Stable paper ID returned by list_papers or add_paper"`
}

type updatePaperInput struct {
	ID    string     `json:"id" jsonschema:"Stable ID of the existing paper to edit"`
	Patch PaperPatch `json:"patch" jsonschema:"Only supplied fields are changed; omitted metadata is preserved"`
}

type mcpAccessKey struct{}

func requireWrite(ctx context.Context) error {
	if canWrite, _ := ctx.Value(mcpAccessKey{}).(bool); canWrite {
		return nil
	}
	return errors.New("write access requires Authorization: Bearer <MCP token>; unauthenticated clients are read-only")
}

func mcpAuth(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		canWrite := false
		if authorization := r.Header.Get("Authorization"); authorization != "" {
			parts := strings.SplitN(authorization, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") ||
				subtle.ConstantTimeCompare([]byte(strings.TrimSpace(parts[1])), []byte(token)) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="bibliocell-library"`)
				http.Error(w, "invalid MCP bearer token", http.StatusUnauthorized)
				return
			}
			canWrite = true
		}
		ctx := context.WithValue(r.Context(), mcpAccessKey{}, canWrite)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type importPapersInput struct {
	Papers  []paperInput `json:"papers" jsonschema:"Paper records to validate and insert as one atomic batch; maximum 1000"`
	DryRun  bool         `json:"dryRun,omitempty" jsonschema:"When true, validate the complete batch and roll back without writing"`
	BatchID string       `json:"batchId,omitempty" jsonschema:"Batch to join; reuse it across calls of one run to undo them together. Generated when empty"`
}

type undoImportInput struct {
	BatchID string `json:"batchId" jsonschema:"The batchId an import returned"`
}

type addEvidenceInput struct {
	Evidence []EvidenceInput `json:"evidence" jsonschema:"Claims to record as one atomic batch; maximum 1000"`
}

type evidenceOutput struct {
	Evidence []Evidence `json:"evidence"`
}

type listEvidenceInput struct {
	PaperID   string `json:"paperId,omitempty" jsonschema:"Only evidence about this paper"`
	DatasetID string `json:"datasetId,omitempty" jsonschema:"Only evidence about this dataset"`
}

type getDatasetInput struct {
	ID string `json:"id" jsonschema:"Stable dataset ID returned by list_datasets or add_dataset"`
}

type updateDatasetInput struct {
	ID    string       `json:"id" jsonschema:"Stable ID of the dataset to edit"`
	Patch DatasetPatch `json:"patch" jsonschema:"Only supplied fields change"`
}

type linkDatasetInput struct {
	DatasetID string `json:"datasetId" jsonschema:"Stable ID of the dataset"`
	PaperLinkInput
}

type attachFileInput struct {
	PaperID string `json:"paperId" jsonschema:"Stable ID of the paper"`
	Name    string `json:"name" jsonschema:"File name relative to inboxPath from describe_library"`
	Type    string `json:"type,omitempty" jsonschema:"fulltext or supplement; defaults to fulltext"`
}

type listFilesInput struct {
	PaperID string `json:"paperId" jsonschema:"Stable ID of the paper"`
}

type fetchFulltextInput struct {
	PaperID string `json:"paperId" jsonschema:"Paper that should receive the PDF"`
}

type listFilesOutput struct {
	Files []PaperFile `json:"files"`
}

type describeLibraryOutput struct {
	SchemaVersion    int                 `json:"schemaVersion"`
	AccessRole       string              `json:"accessRole"`
	CanWrite         bool                `json:"canWrite"`
	PaperSchema      string              `json:"paperSchema"`
	DatabaseSchema   string              `json:"databaseSchema"`
	ImportTool       string              `json:"importTool"`
	RequiredFields   []string            `json:"requiredFields"`
	Enums            map[string][]string `json:"enums"`
	ImportRules      []string            `json:"importRules"`
	AvailableTools   []string            `json:"availableTools"`
	InboxPath        string              `json:"inboxPath"`
	DemoDataIncluded bool                `json:"demoDataIncluded"`
}

// addWriteTool registers a tool that requires the bearer token.
func addWriteTool[In, Out any](server *mcp.Server, name, description string, handler func(context.Context, In) (Out, error)) {
	mcp.AddTool(server, &mcp.Tool{Name: name, Description: description}, func(ctx context.Context, _ *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		if err := requireWrite(ctx); err != nil {
			var none Out
			return nil, none, err
		}
		output, err := handler(ctx, input)
		return nil, output, err
	})
}

// NewMCPServer builds the MCP server over the real database. It is the single
// tool surface for every AI client: external ones reach it through StartMCP,
// the built-in agent connects in-process through ConnectLocalMCP.
func NewMCPServer(store *Store, credentials func() SourceCredentials) (*mcp.Server, error) {
	paperSchema, err := jsonschema.For[paperInput](nil)
	if err != nil {
		return nil, err
	}
	paperSchemaJSON, err := json.MarshalIndent(paperSchema, "", "  ")
	if err != nil {
		return nil, err
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name: "bibliocell-library", Title: "BiblioCell", Version: "0.1.0",
	}, &mcp.ServerOptions{Instructions: "Use describe_library to discover schema, your access role, and import rules. Unauthenticated clients are read-only. add_paper, import_papers, update_paper, and delete_paper require the local Bearer token. Screening and verification are a person's decisions: agents propose through proposedScreening, record claims with add_evidence, and a person confirms in the desktop app. Reuse one batchId across a run so undo_import can remove it. Demo data is never exposed."})

	server.AddResource(&mcp.Resource{
		URI: paperSchemaURI, Name: "paper-import.schema.json", Title: "Paper import JSON Schema",
		Description: "JSON Schema for one paper accepted by add_paper and the papers array of import_papers",
		MIMEType:    "application/schema+json", Size: int64(len(paperSchemaJSON)),
	}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI: paperSchemaURI, MIMEType: "application/schema+json", Text: string(paperSchemaJSON),
		}}}, nil
	})
	server.AddResource(&mcp.Resource{
		URI: databaseSchemaURI, Name: "schema.sql", Title: "BiblioCell SQLite DDL",
		Description: "Complete SQLite tables, constraints, indexes, views, and schema migration version",
		MIMEType:    "application/sql", Size: int64(len(schemaSQL)),
	}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI: databaseSchemaURI, MIMEType: "application/sql", Text: schemaSQL,
		}}}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "describe_library", Description: "Discover schema resources, paper fields, enums, and atomic import rules before adding data",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, describeLibraryOutput, error) {
		canWrite, _ := ctx.Value(mcpAccessKey{}).(bool)
		role := "read-only"
		if canWrite {
			role = "read-write"
		}
		return nil, describeLibraryOutput{
			SchemaVersion: SchemaVersion, AccessRole: role, CanWrite: canWrite,
			PaperSchema: paperSchemaURI, DatabaseSchema: databaseSchemaURI,
			ImportTool: "import_papers", RequiredFields: []string{"title"},
			Enums: map[string][]string{
				"publicationStatus": {"unknown", "preprint", "published", "unpublished"},
				"screening":         {"pending", "included", "excluded", "uncertain"},
				"proposedScreening": {"included", "excluded", "uncertain"},
				"reading":           {"unread", "skimmed", "read"},
				"articleTypes":      {"original", "method", "review", "other"},
				"datasetStatus":     {"pending", "identified", "verified"},
				"datasetRole":       {"generated", "reused", "reprocessed", "benchmarked", "unknown"},
				"evidenceStatus":    {"unreviewed", "verified", "rejected"},
				"fileType":          {"fulltext", "supplement"},
			},
			ImportRules: []string{
				"Write tools require Authorization: Bearer <MCP token>; unauthenticated access is read-only",
				"Read the paper JSON Schema before constructing records",
				"Set dryRun=true first to validate the complete batch without writing",
				"Imports are atomic: any invalid record rolls back the entire batch",
				"A known paper (same ID, DOI, PMID, or title) is skipped and listed in skipped, so rerunning a batch is safe; it is never updated",
				"add_dataset matches by accession or title the same way and only adds new links",
				"An empty ID is generated",
				"DOI values are normalized and must be unique across the library",
				"delete_paper permanently removes one paper even if reviewed, with notes, evidence and stored copies; datasets and original files remain",
				"undo_import removes one batch and its stored files, except records a person has reviewed, starred, read, or verified",
				"Record what you extract with add_evidence: a short excerpt and its source, per field, for the person checking it",
				"Only a person verifies evidence, datasets, repository records, and paper links; list_evidence shows their verdicts",
				"Drop a file into inboxPath, then call attach_file with its name; it moves into the library",
				"fetch_fulltext stores an open-access PDF by DOI or PMID; when it fails, leave the paper for a person rather than scraping publisher pages",
				"screening and reviewedAt are read-only: a person decides in the desktop app; send proposedScreening with proposedReason instead",
				"proposedScreening=excluded requires a non-empty proposedReason",
				"Once reviewedAt is set, update_paper accepts only proposedScreening and proposedReason",
				"At most 1000 papers can be imported in one call",
				"Only real SQLite data is accessible; Demo records are never included",
			},
			AvailableTools: []string{
				"describe_library", "library_status", "list_papers", "get_paper", "add_paper", "import_papers", "update_paper", "delete_paper", "undo_import",
				"list_datasets", "get_dataset", "add_dataset", "update_dataset", "link_dataset",
				"add_evidence", "list_evidence", "attach_file", "fetch_fulltext", "list_files",
			},
			InboxPath:        store.inboxPath(),
			DemoDataIncluded: false,
		}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "library_status", Description: "Get real SQLite database status and record counts",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, Status, error) {
		status, err := store.Status()
		return nil, status, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_papers", Description: "List and search real papers by text, decision, source, or batch, one page at a time; never returns Demo records",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input PaperFilter) (*mcp.CallToolResult, listPapersOutput, error) {
		papers, err := store.ListPapers(ctx)
		if err != nil {
			return nil, listPapersOutput{}, err
		}
		input.Limit = mcpPageLimit(input.Limit)
		matches, total := FilterPapers(papers, input)
		return nil, listPapersOutput{Total: total, Papers: matches}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_paper", Description: "Read one real paper and all editable metadata by ID; unauthenticated read access is allowed",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input getPaperInput) (*mcp.CallToolResult, Paper, error) {
		paper, err := store.GetPaper(ctx, input.ID)
		return nil, paper, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "add_paper", Description: "Add one paper to the real SQLite library; requires Bearer token write access",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input paperInput) (*mcp.CallToolResult, Paper, error) {
		if err := requireWrite(ctx); err != nil {
			return nil, Paper{}, err
		}
		paper, err := store.AddPaper(ctx, input.paper())
		return nil, paper, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "import_papers", Description: "Validate and atomically import up to 1000 papers, skipping known ones and returning a batchId; dryRun=true checks without writing",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input importPapersInput) (*mcp.CallToolResult, ImportResult, error) {
		if err := requireWrite(ctx); err != nil {
			return nil, ImportResult{}, err
		}
		if len(input.Papers) == 0 {
			return nil, ImportResult{}, errors.New("papers must contain at least one record")
		}
		if len(input.Papers) > 1000 {
			return nil, ImportResult{}, errors.New("a single import cannot exceed 1000 papers")
		}
		papers := make([]Paper, len(input.Papers))
		for i := range input.Papers {
			papers[i] = input.Papers[i].paper()
		}
		result, err := store.ImportPapers(ctx, papers, input.BatchID, input.DryRun)
		return nil, result, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "update_paper", Description: "Edit selected metadata of an unreviewed paper or suggest a screening decision; requires Bearer token write access and preserves omitted fields. A paper a person reviewed accepts only proposals",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input updatePaperInput) (*mcp.CallToolResult, Paper, error) {
		if err := requireWrite(ctx); err != nil {
			return nil, Paper{}, err
		}
		paper, err := store.UpdatePaper(ctx, input.ID, input.Patch)
		return nil, paper, err
	})

	addWriteTool(server, "delete_paper", "Permanently delete one paper (including reviewed papers), notes, evidence and stored attachment copies; unlink but preserve datasets and original imported files. Cannot be undone. Requires Bearer token write access",
		func(ctx context.Context, input getPaperInput) (deletePaperOutput, error) {
			if err := store.DeletePaper(ctx, input.ID); err != nil {
				return deletePaperOutput{}, err
			}
			return deletePaperOutput{ID: input.ID, Deleted: true}, nil
		})

	addWriteTool(server, "undo_import", "Remove what one import batch created, keeping records a person has worked on; requires Bearer token write access",
		func(ctx context.Context, input undoImportInput) (UndoResult, error) {
			return store.UndoImport(ctx, input.BatchID)
		})

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_datasets", Description: "List and search real datasets, one page at a time; unauthenticated read access is allowed",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input DatasetFilter) (*mcp.CallToolResult, listDatasetsOutput, error) {
		datasets, err := store.ListDatasets(ctx)
		if err != nil {
			return nil, listDatasetsOutput{}, err
		}
		input.Limit = mcpPageLimit(input.Limit)
		matches, total := FilterDatasets(datasets, input)
		return nil, listDatasetsOutput{Total: total, Datasets: matches}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_dataset", Description: "Read one real dataset with its repositories and paper links; unauthenticated read access is allowed",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input getDatasetInput) (*mcp.CallToolResult, Dataset, error) {
		dataset, err := store.GetDataset(ctx, input.ID)
		return nil, dataset, err
	})

	addWriteTool(server, "add_dataset", "Create a dataset with its repositories and paper links, or find it by accession or title and add new links; requires Bearer token write access",
		func(ctx context.Context, input DatasetInput) (DatasetResult, error) {
			return store.AddDataset(ctx, input)
		})

	addWriteTool(server, "update_dataset", "Edit selected fields of a dataset a person has not verified; requires Bearer token write access",
		func(ctx context.Context, input updateDatasetInput) (Dataset, error) {
			return store.UpdateDataset(ctx, input.ID, input.Patch)
		})

	addWriteTool(server, "link_dataset", "Record that a paper relates to a dataset, with evidence for a person to verify; requires Bearer token write access",
		func(ctx context.Context, input linkDatasetInput) (Dataset, error) {
			return store.LinkDataset(ctx, input.DatasetID, input.PaperLinkInput)
		})

	addWriteTool(server, "add_evidence", "Record claims about paper or dataset fields, each with an excerpt and source, for a person to check; requires Bearer token write access",
		func(ctx context.Context, input addEvidenceInput) (evidenceOutput, error) {
			if len(input.Evidence) == 0 || len(input.Evidence) > 1000 {
				return evidenceOutput{}, errors.New("evidence must contain 1 to 1000 records")
			}
			evidence, err := store.AddEvidence(ctx, input.Evidence)
			return evidenceOutput{Evidence: evidence}, err
		})

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_evidence", Description: "List recorded claims and a person's verdicts on them; unauthenticated read access is allowed",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input listEvidenceInput) (*mcp.CallToolResult, evidenceOutput, error) {
		evidence, err := store.ListEvidence(ctx, input.PaperID, input.DatasetID)
		return nil, evidenceOutput{Evidence: evidence}, err
	})

	addWriteTool(server, "fetch_fulltext", "Find an open-access PDF for a paper through PMC and Unpaywall and store it; returns the existing file if it has full text; requires Bearer token write access",
		func(ctx context.Context, input fetchFulltextInput) (FulltextResult, error) {
			return store.FetchFulltext(ctx, input.PaperID, credentials)
		})
	addWriteTool(server, "attach_file", "Move a file from the inbox into the library and attach it to a paper; requires Bearer token write access",
		func(ctx context.Context, input attachFileInput) (PaperFile, error) {
			if input.Type == "" {
				input.Type = "fulltext"
			}
			return store.AttachInboxFile(ctx, input.PaperID, input.Name, input.Type)
		})

	mcp.AddTool(server, &mcp.Tool{
		Name: "list_files", Description: "List the files attached to a paper, with paths relative to storagePath; unauthenticated read access is allowed",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input listFilesInput) (*mcp.CallToolResult, listFilesOutput, error) {
		files, err := store.ListPaperFiles(ctx)
		if err != nil {
			return nil, listFilesOutput{}, err
		}
		output := listFilesOutput{Files: []PaperFile{}}
		for _, file := range files {
			if file.PaperID == input.PaperID {
				output.Files = append(output.Files, file)
			}
		}
		return nil, output, nil
	})

	return server, nil
}

// StartMCP starts a local Streamable HTTP MCP server for the real database. It
// serves loopback addresses only: the bearer token travels unencrypted.
func StartMCP(store *Store, address, token string, credentials func() SourceCredentials) (*MCPServer, error) {
	if len(token) < 32 {
		return nil, errors.New("a secure MCP write token is required")
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("the MCP server listens on loopback addresses only")
	}
	server, err := NewMCPServer(store, credentials)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 8 << 20,
	})
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpAuth(handler, token))
	httpServer := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = httpServer.Serve(listener) }()
	return &MCPServer{Endpoint: "http://" + listener.Addr().String() + "/mcp", server: httpServer}, nil
}

// LocalMCP is an in-process, read-write MCP session over the real database,
// used by the built-in agent. Nothing crosses the network.
type LocalMCP struct {
	Session *mcp.ClientSession
	cancel  context.CancelFunc
}

// ConnectLocalMCP connects a client to the MCP server through memory. The
// session has write access: the agent runs inside the app on the person's
// behalf, so the bearer token has nothing to protect.
func ConnectLocalMCP(ctx context.Context, store *Store, credentials func() SourceCredentials) (*LocalMCP, error) {
	server, err := NewMCPServer(store, credentials)
	if err != nil {
		return nil, err
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverCtx, cancel := context.WithCancel(context.WithValue(ctx, mcpAccessKey{}, true))
	if _, err := server.Connect(serverCtx, serverTransport, nil); err != nil {
		cancel()
		return nil, err
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "bibliocell-agent", Version: "0.1.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	return &LocalMCP{Session: session, cancel: cancel}, nil
}

// Close ends the in-process session.
func (l *LocalMCP) Close() error {
	if l == nil {
		return nil
	}
	err := l.Session.Close()
	l.cancel()
	return err
}

// Close stops accepting MCP requests.
func (s *MCPServer) Close(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}
