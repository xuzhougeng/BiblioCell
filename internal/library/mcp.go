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

type listPapersInput struct {
	Query string `json:"query,omitempty" jsonschema:"Optional text matched against title, DOI, authors, species, organs, modalities, platforms, and tags"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum papers to return, from 1 to 500; defaults to 100"`
}

type listPapersOutput struct {
	Papers []Paper `json:"papers"`
	Total  int     `json:"total"`
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
	Screening         string         `json:"screening,omitempty" jsonschema:"pending, included, excluded, or uncertain; defaults to pending"`
	ScreeningReason   string         `json:"screeningReason,omitempty" jsonschema:"Required when screening is excluded"`
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
		Tags: input.Tags, Screening: input.Screening, ScreeningReason: input.ScreeningReason,
		Reading: input.Reading, Starred: input.Starred, Fulltext: "missing", Source: "MCP",
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
	Papers []paperInput `json:"papers" jsonschema:"Paper records to validate and insert as one atomic batch; maximum 1000"`
	DryRun bool         `json:"dryRun,omitempty" jsonschema:"When true, validate the complete batch and roll back without writing"`
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
	DemoDataIncluded bool                `json:"demoDataIncluded"`
}

// StartMCP starts a local Streamable HTTP MCP server for the real database.
func StartMCP(store *Store, token string) (*MCPServer, error) {
	return startMCP(store, MCPAddress, token)
}

func startMCP(store *Store, address, token string) (*MCPServer, error) {
	if len(token) < 32 {
		return nil, errors.New("a secure MCP write token is required")
	}
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
	}, &mcp.ServerOptions{Instructions: "Use describe_library to discover schema, your access role, and import rules. Unauthenticated clients are read-only. add_paper, import_papers, and update_paper require the local Bearer token. Demo data is never exposed."})

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
				"reading":           {"unread", "skimmed", "read"},
				"articleTypes":      {"original", "method", "review", "other"},
			},
			ImportRules: []string{
				"Write tools require Authorization: Bearer <MCP token>; unauthenticated access is read-only",
				"Read the paper JSON Schema before constructing records",
				"Set dryRun=true first to validate the complete batch without writing",
				"Imports are atomic: any invalid or duplicate record rolls back the entire batch",
				"An empty ID is generated; an existing ID is rejected rather than updated",
				"DOI values are normalized and must be unique across the library",
				"An excluded paper requires a non-empty screeningReason",
				"At most 1000 papers can be imported in one call",
				"Only real SQLite data is accessible; Demo records are never included",
			},
			AvailableTools:   []string{"describe_library", "library_status", "list_papers", "get_paper", "add_paper", "import_papers", "update_paper"},
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
		Name: "list_papers", Description: "List and search real papers; never returns Demo records",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input listPapersInput) (*mcp.CallToolResult, listPapersOutput, error) {
		papers, err := store.ListPapers(ctx)
		if err != nil {
			return nil, listPapersOutput{}, err
		}
		matches := FilterPapers(papers, input.Query, 0)
		limit := input.Limit
		if limit <= 0 {
			limit = 100
		}
		if limit > 500 {
			limit = 500
		}
		return nil, listPapersOutput{Total: len(matches), Papers: FilterPapers(matches, "", limit)}, nil
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
		Name: "import_papers", Description: "Validate and atomically import up to 1000 papers; dryRun=true checks without writing",
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
		result, err := store.ImportPapers(ctx, papers, input.DryRun)
		return nil, result, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "update_paper", Description: "Edit selected metadata fields of an existing paper; requires Bearer token write access and preserves omitted fields",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input updatePaperInput) (*mcp.CallToolResult, Paper, error) {
		if err := requireWrite(ctx); err != nil {
			return nil, Paper{}, err
		}
		paper, err := store.UpdatePaper(ctx, input.ID, input.Patch)
		return nil, paper, err
	})

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

// Close stops accepting MCP requests.
func (s *MCPServer) Close(ctx context.Context) error {
	if s == nil || s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}
