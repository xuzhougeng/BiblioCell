package library

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func agentStore(t *testing.T) (*Store, string) {
	t.Helper()
	storage := t.TempDir()
	store, err := Open(filepath.Join(storage, "bibliocell.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store, storage
}

func decidePaper(t *testing.T, store *Store, paper Paper, status string) Paper {
	t.Helper()
	ctx := context.Background()
	decided := paper
	decided.Screening = status
	if _, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &paper, After: decided}}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetPaper(ctx, paper.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestImportSkipsKnownPapersSoRerunsAreSafe(t *testing.T) {
	ctx := context.Background()
	store, _ := agentStore(t)
	batch := func() []Paper {
		return []Paper{
			{Title: "By DOI", DOI: "10.1000/doi"},
			{Title: "By PMID", PMID: "111"},
			{Title: "No identifiers"},
		}
	}
	first, err := store.ImportPapers(ctx, batch(), "run-1", false)
	if err != nil || first.Count != 3 || first.BatchID != "run-1" {
		t.Fatalf("first import = %+v, %v", first, err)
	}
	second, err := store.ImportPapers(ctx, append(batch(), Paper{Title: "New one"}, Paper{Title: "new ONE "}), "run-2", false)
	if err != nil {
		t.Fatal(err)
	}
	if second.Count != 1 || len(second.Skipped) != 4 || second.Skipped[0].ExistingID != first.Papers[0].ID {
		t.Fatalf("second import = %+v", second)
	}

	if _, err := store.ImportPapers(ctx, []Paper{{Title: "Valid"}, {Title: "Broken", DOI: "nope"}}, "", false); err == nil {
		t.Fatal("invalid record accepted")
	}
	if papers, err := store.ListPapers(ctx); err != nil || len(papers) != 4 {
		t.Fatalf("invalid batch was not rolled back: %d, %v", len(papers), err)
	}
	if _, err := store.AddPaper(ctx, Paper{Title: "By DOI", DOI: "10.1000/DOI"}); !errors.Is(err, ErrDuplicatePaper) {
		t.Errorf("AddPaper(known) error = %v", err)
	}
	dry, err := store.ImportPapers(ctx, []Paper{{Title: "Only a rehearsal"}}, "", true)
	if err != nil || dry.Count != 1 {
		t.Fatalf("dry run = %+v, %v", dry, err)
	}
	if papers, _ := store.ListPapers(ctx); len(papers) != 4 {
		t.Errorf("dry run wrote %d papers", len(papers))
	}
}

func TestFilterPapersPagesThroughEveryMatch(t *testing.T) {
	ctx := context.Background()
	store, _ := agentStore(t)
	bulk := make([]Paper, 7)
	for i := range bulk {
		bulk[i] = Paper{Title: "Bulk paper " + string(rune('A'+i)), Source: "MCP"}
	}
	if _, err := store.ImportPapers(ctx, bulk, "bulk", false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddPaper(ctx, Paper{Title: "From the terminal", Source: "CLI"}); err != nil {
		t.Fatal(err)
	}
	papers, err := store.ListPapers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for offset := 0; ; offset += 3 {
		page, total := FilterPapers(papers, PaperFilter{Limit: 3, Offset: offset})
		if total != 8 {
			t.Fatalf("total = %d", total)
		}
		if len(page) == 0 {
			break
		}
		for _, paper := range page {
			seen[paper.ID] = true
		}
	}
	if len(seen) != 8 {
		t.Errorf("paging reached %d of 8 papers", len(seen))
	}
	for _, check := range []struct {
		filter PaperFilter
		want   int
	}{
		{PaperFilter{Source: "CLI"}, 1},
		{PaperFilter{BatchID: "bulk"}, 7},
		{PaperFilter{Screening: "pending"}, 8},
		{PaperFilter{Screening: "included"}, 0},
		{PaperFilter{NeedsReview: true}, 8},
		{PaperFilter{Offset: 99}, 8},
	} {
		if _, total := FilterPapers(papers, check.filter); total != check.want {
			t.Errorf("%+v matched %d, want %d", check.filter, total, check.want)
		}
	}
	reviewed := decidePaper(t, store, papers[0], "included")
	if _, total := FilterPapers([]Paper{reviewed}, PaperFilter{NeedsReview: true}); total != 0 {
		t.Error("a decided paper still needs review")
	}
	reviewed.ProposedScreening = "excluded"
	if _, total := FilterPapers([]Paper{reviewed}, PaperFilter{NeedsReview: true}); total != 1 {
		t.Error("an open proposal does not need review")
	}
}

func TestEvidenceRecordsClaimsForAPerson(t *testing.T) {
	ctx := context.Background()
	store, _ := agentStore(t)
	paper, err := store.AddPaper(ctx, Paper{Title: "Root atlas"})
	if err != nil {
		t.Fatal(err)
	}
	claim := EvidenceInput{
		PaperID: paper.ID, Field: "organs", Value: "root", SourceURL: "https://example.org/a",
		Location: "Methods", Excerpt: "root tips were dissected", Extractor: "test-agent",
	}
	added, err := store.AddEvidence(ctx, []EvidenceInput{claim})
	if err != nil || len(added) != 1 || added[0].Status != "unreviewed" || added[0].Location != "Methods" {
		t.Fatalf("AddEvidence() = %+v, %v", added, err)
	}
	if err := store.ReviewEvidence(ctx, added[0].ID, "verified"); err != nil {
		t.Fatal(err)
	}
	again, err := store.AddEvidence(ctx, []EvidenceInput{claim})
	if err != nil || len(again) != 1 || again[0].ID != added[0].ID || again[0].Status != "verified" {
		t.Fatalf("rerun reset the verdict: %+v, %v", again, err)
	}
	var method string
	if err := store.db.QueryRow("SELECT extraction_method FROM field_evidence WHERE id = ?", added[0].ID).Scan(&method); err != nil || method != "model" {
		t.Errorf("extraction_method = %q, %v", method, err)
	}

	for name, bad := range map[string]EvidenceInput{
		"no target":    {Field: "organs", Excerpt: "x", SourceURL: "https://example.org"},
		"two targets":  {PaperID: paper.ID, DatasetID: "DS-x", Field: "organs", Excerpt: "x", SourceURL: "https://example.org"},
		"no excerpt":   {PaperID: paper.ID, Field: "organs", SourceURL: "https://example.org"},
		"no source":    {PaperID: paper.ID, Field: "organs", Excerpt: "x"},
		"no field":     {PaperID: paper.ID, Excerpt: "x", SourceURL: "https://example.org"},
		"ghost paper":  {PaperID: "P-ghost", Field: "organs", Excerpt: "x", SourceURL: "https://example.org"},
		"ghost source": {PaperID: paper.ID, Field: "organs", Excerpt: "x", FileID: "R-ghost"},
	} {
		if _, err := store.AddEvidence(ctx, []EvidenceInput{claim, bad}); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := store.ReviewEvidence(ctx, added[0].ID, "conflict"); err == nil {
		t.Error("unsupported verdict accepted")
	}
	if listed, err := store.ListEvidence(ctx, paper.ID, ""); err != nil || len(listed) != 1 {
		t.Errorf("ListEvidence() = %+v, %v", listed, err)
	}
	if listed, err := store.ListEvidence(ctx, "P-other", ""); err != nil || len(listed) != 0 {
		t.Errorf("ListEvidence(other) = %+v, %v", listed, err)
	}
}

func TestEvidenceFromARemovedFileGoesWithIt(t *testing.T) {
	ctx := context.Background()
	store, _ := agentStore(t)
	paper, err := store.AddPaper(ctx, Paper{Title: "Paper with a file"})
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.ImportPaperFile(ctx, paper.ID, writeSource(t, "paper.pdf", "pdf"), "fulltext")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddEvidence(ctx, []EvidenceInput{{PaperID: paper.ID, Field: "organs", Excerpt: "x", FileID: file.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := store.RemovePaperFile(ctx, file.ID); err != nil {
		t.Fatal(err)
	}
	if listed, err := store.ListEvidence(ctx, "", ""); err != nil || len(listed) != 0 {
		t.Errorf("evidence outlived its file: %+v, %v", listed, err)
	}
}

func TestAgentDatasetsStayUnverified(t *testing.T) {
	ctx := context.Background()
	store, _ := agentStore(t)
	paper, err := store.AddPaper(ctx, Paper{Title: "Linked paper"})
	if err != nil {
		t.Fatal(err)
	}
	input := DatasetInput{
		Title: "Root atlas data", Organs: []string{"root"},
		Repositories: []RepositoryInput{{Repository: "GEO", Accession: "GSE1", EvidenceText: "deposited as GSE1"}},
		Papers:       []PaperLinkInput{{PaperID: paper.ID, Role: "generated", EvidenceText: "we generated"}},
	}
	first, err := store.AddDataset(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Created || first.Dataset.Status != "pending" || first.Dataset.BatchID == "" || first.Dataset.ReviewedAt != "" {
		t.Fatalf("AddDataset() = %+v", first)
	}
	if len(first.Dataset.PaperLinks) != 1 || first.Dataset.PaperLinks[0].Verified || first.Dataset.Repositories[0].Verified {
		t.Fatalf("agent claims came in verified: %+v", first.Dataset)
	}

	other, err := store.AddPaper(ctx, Paper{Title: "Reusing paper"})
	if err != nil {
		t.Fatal(err)
	}
	rerun := input
	rerun.Title = "Named differently"
	rerun.Papers = append(rerun.Papers, PaperLinkInput{PaperID: other.ID, Role: "reused", EvidenceURL: "https://example.org"})
	second, err := store.AddDataset(ctx, rerun)
	if err != nil {
		t.Fatal(err)
	}
	if second.Created || second.Dataset.ID != first.Dataset.ID || second.Dataset.Title != input.Title || len(second.Dataset.PaperLinks) != 2 {
		t.Fatalf("rerun = %+v", second)
	}
	if datasets, err := store.ListDatasets(ctx); err != nil || len(datasets) != 1 {
		t.Fatalf("rerun duplicated the dataset: %d, %v", len(datasets), err)
	}

	if _, err := store.AddDataset(ctx, DatasetInput{Title: "Claims verified", Status: "verified", Repositories: []RepositoryInput{{Repository: "geo", Accession: "GSE2"}}}); err == nil {
		t.Error("agent verified a new dataset")
	}
	verified := "verified"
	if _, err := store.UpdateDataset(ctx, first.Dataset.ID, DatasetPatch{Status: &verified}); err == nil {
		t.Error("agent verified a dataset")
	}
	if _, err := store.LinkDataset(ctx, first.Dataset.ID, PaperLinkInput{PaperID: paper.ID, Role: "reused"}); err == nil {
		t.Error("a link without evidence was accepted")
	}
	if _, err := store.LinkDataset(ctx, first.Dataset.ID, PaperLinkInput{PaperID: "P-ghost", EvidenceText: "x"}); !errors.Is(err, ErrPaperNotFound) {
		t.Errorf("ghost paper link error = %v", err)
	}

	identified, title := "identified", "Better title"
	updated, err := store.UpdateDataset(ctx, first.Dataset.ID, DatasetPatch{
		Status: &identified, Title: &title,
		AddRepositories: []RepositoryInput{{Repository: "geo", Accession: "GSE1"}, {Repository: "sra", Accession: "SRP9"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "identified" || updated.Title != title || len(updated.Repositories) != 2 {
		t.Fatalf("UpdateDataset() = %+v", updated)
	}

	// A person's checks survive what the agent does next.
	record := updated.Repositories[0]
	if err := store.VerifyDatasetLink(ctx, paper.ID, updated.ID, "generated", true); err != nil {
		t.Fatal(err)
	}
	if err := store.VerifyDatasetRepository(ctx, updated.ID, record.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LinkDataset(ctx, updated.ID, PaperLinkInput{PaperID: paper.ID, Role: "generated", EvidenceText: "rewritten"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateDataset(ctx, updated.ID, DatasetPatch{Title: &title}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetDataset(ctx, updated.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Repositories[0].Verified {
		t.Error("agent edit cleared a verified repository")
	}
	for _, link := range got.PaperLinks {
		if link.PaperID == paper.ID && (!link.Verified || link.EvidenceText != "we generated") {
			t.Errorf("agent changed a verified link: %+v", link)
		}
	}
	if err := store.VerifyDatasetLink(ctx, "P-ghost", updated.ID, "generated", true); err == nil {
		t.Error("verifying a missing link succeeded")
	}

	final := got
	final.Status = "verified"
	if conflicts, err := store.SaveDatasetEdits(ctx, []DatasetEdit{{Before: &got, After: final}}); err != nil || len(conflicts) != 0 {
		t.Fatalf("person verifying = %v, %v", conflicts, err)
	}
	reviewed, err := store.GetDataset(ctx, updated.ID)
	if err != nil || reviewed.Status != "verified" || reviewed.ReviewedAt == "" {
		t.Fatalf("reviewed dataset = %+v, %v", reviewed, err)
	}
	if _, err := store.UpdateDataset(ctx, updated.ID, DatasetPatch{Title: &title}); !errors.Is(err, ErrDatasetReviewed) {
		t.Errorf("edit of a verified dataset error = %v", err)
	}
	reopened := reviewed
	reopened.Status = "pending"
	if _, err := store.SaveDatasetEdits(ctx, []DatasetEdit{{Before: &reviewed, After: reopened}}); err != nil {
		t.Fatal(err)
	}
	if got, err := store.GetDataset(ctx, updated.ID); err != nil || got.ReviewedAt != "" {
		t.Errorf("reset dataset = %+v, %v", got, err)
	}
}

func TestSaveDatasetEditsKeepsWhatAgentsWrote(t *testing.T) {
	ctx := context.Background()
	store, _ := agentStore(t)
	added, err := store.AddDataset(ctx, DatasetInput{Title: "Shared dataset", Availability: "public"})
	if err != nil {
		t.Fatal(err)
	}
	seen := added.Dataset
	organs := []string{"leaf"}
	if _, err := store.UpdateDataset(ctx, seen.ID, DatasetPatch{Organs: &organs}); err != nil {
		t.Fatal(err)
	}

	edited := seen
	edited.Notes = "a person's note"
	conflicts, err := store.SaveDatasetEdits(ctx, []DatasetEdit{{Before: &seen, After: edited}})
	if err != nil || len(conflicts) != 0 {
		t.Fatalf("SaveDatasetEdits() = %v, %v", conflicts, err)
	}
	got, err := store.GetDataset(ctx, seen.ID)
	if err != nil || len(got.Organs) != 1 || got.Notes != "a person's note" || got.Availability != "public" {
		t.Fatalf("merged dataset = %+v, %v", got, err)
	}

	clashing := seen
	clashing.Organs = []string{"root"}
	conflicts, err = store.SaveDatasetEdits(ctx, []DatasetEdit{{Before: &seen, After: clashing}})
	if err != nil || len(conflicts) != 1 || conflicts[0] != seen.Title {
		t.Fatalf("conflict = %v, %v", conflicts, err)
	}
	if again, err := store.GetDataset(ctx, seen.ID); err != nil || again.Organs[0] != "leaf" {
		t.Fatalf("a conflicting edit was saved: %+v, %v", again, err)
	}
}

func TestUndoImportRemovesOnlyUntouchedRecords(t *testing.T) {
	ctx := context.Background()
	store, storage := agentStore(t)
	result, err := store.ImportPapers(ctx, []Paper{{Title: "Wrong paper"}, {Title: "Reviewed paper"}, {Title: "Starred paper"}}, "run", false)
	if err != nil {
		t.Fatal(err)
	}
	wrong, reviewed, starred := result.Papers[0], result.Papers[1], result.Papers[2]
	decidePaper(t, store, reviewed, "included")
	star := starred
	star.Starred = true
	if _, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &starred, After: star}}); err != nil {
		t.Fatal(err)
	}
	file, err := store.ImportPaperFile(ctx, wrong.ID, writeSource(t, "wrong.pdf", "pdf"), "fulltext")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddEvidence(ctx, []EvidenceInput{{PaperID: wrong.ID, Field: "organs", Excerpt: "x", FileID: file.ID}}); err != nil {
		t.Fatal(err)
	}
	gone, err := store.AddDataset(ctx, DatasetInput{Title: "Wrong dataset", BatchID: "run", Papers: []PaperLinkInput{{PaperID: reviewed.ID, EvidenceText: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	kept, err := store.AddDataset(ctx, DatasetInput{Title: "Checked dataset", BatchID: "run", Repositories: []RepositoryInput{{Repository: "geo", Accession: "GSE7"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.VerifyDatasetRepository(ctx, kept.Dataset.ID, kept.Dataset.Repositories[0].ID, true); err != nil {
		t.Fatal(err)
	}

	undone, err := store.UndoImport(ctx, "run")
	if err != nil {
		t.Fatal(err)
	}
	if undone.PapersRemoved != 1 || undone.DatasetsRemoved != 1 || len(undone.Kept) != 3 {
		t.Fatalf("UndoImport() = %+v", undone)
	}
	if _, err := store.GetPaper(ctx, wrong.ID); !errors.Is(err, ErrPaperNotFound) {
		t.Errorf("wrong paper survived: %v", err)
	}
	if _, err := store.GetDataset(ctx, gone.Dataset.ID); !errors.Is(err, ErrDatasetNotFound) {
		t.Errorf("wrong dataset survived: %v", err)
	}
	if _, err := store.GetPaper(ctx, reviewed.ID); err != nil {
		t.Errorf("reviewed paper removed: %v", err)
	}
	if names := storedNames(t, storage); len(names) != 0 {
		t.Errorf("stored copy survived: %v", names)
	}
	if files, err := store.ListPaperFiles(ctx); err != nil || len(files) != 0 {
		t.Errorf("file record survived: %+v, %v", files, err)
	}
	if evidence, err := store.ListEvidence(ctx, "", ""); err != nil || len(evidence) != 0 {
		t.Errorf("evidence survived: %+v, %v", evidence, err)
	}
	if _, err := store.UndoImport(ctx, "no-such-run"); !errors.Is(err, ErrBatchNotFound) {
		t.Errorf("unknown batch error = %v", err)
	}
}

func TestAttachInboxFileStaysInsideTheInbox(t *testing.T) {
	ctx := context.Background()
	store, storage := agentStore(t)
	paper, err := store.AddPaper(ctx, Paper{Title: "Paper from the inbox"})
	if err != nil {
		t.Fatal(err)
	}
	inbox := filepath.Join(storage, InboxDir)
	writeFile := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(filepath.Join(inbox, "run", "全文.PDF"), "pdf")
	file, err := store.AttachInboxFile(ctx, paper.ID, "run/全文.PDF", "fulltext")
	if err != nil {
		t.Fatal(err)
	}
	if file.Title != "全文.PDF" || !strings.HasSuffix(file.LocalPath, ".pdf") {
		t.Errorf("attached file = %+v", file)
	}
	if _, err := os.Stat(filepath.Join(inbox, "run", "全文.PDF")); !os.IsNotExist(err) {
		t.Errorf("inbox copy remained: %v", err)
	}

	outside := filepath.Join(storage, "secret.txt")
	writeFile(outside, "secret")
	if err := os.Symlink(outside, filepath.Join(inbox, "link.txt")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../secret.txt", outside, "link.txt", "missing.pdf"} {
		if _, err := store.AttachInboxFile(ctx, paper.ID, name, "supplement"); err == nil {
			t.Errorf("AttachInboxFile(%q) succeeded", name)
		}
	}
	writeFile(filepath.Join(inbox, "keep.pdf"), "kept")
	if _, err := store.AttachInboxFile(ctx, "P-ghost", "keep.pdf", "fulltext"); !errors.Is(err, ErrPaperNotFound) {
		t.Errorf("ghost paper error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(inbox, "keep.pdf")); err != nil {
		t.Errorf("a failed attach lost the inbox file: %v", err)
	}
	if files, err := store.ListPaperFiles(ctx); err != nil || len(files) != 1 {
		t.Errorf("files = %+v, %v", files, err)
	}
}

func TestStartMCPRefusesNonLoopbackAddresses(t *testing.T) {
	store, _ := agentStore(t)
	for _, address := range []string{"0.0.0.0:0", "192.168.1.2:0", "example.org:0", "nonsense"} {
		if server, err := StartMCP(store, address, testMCPToken); err == nil {
			server.Close(context.Background())
			t.Errorf("StartMCP(%q) succeeded", address)
		}
	}
}

func TestMCPAgentWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, storage := agentStore(t)
	server, err := StartMCP(store, "127.0.0.1:0", testMCPToken)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close(ctx)
	connect := func(client *mcp.Client, transport *mcp.StreamableClientTransport) *mcp.ClientSession {
		t.Helper()
		session, err := client.Connect(ctx, transport, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { session.Close() })
		return session
	}
	agent := connect(mcp.NewClient(&mcp.Implementation{Name: "agent", Version: "1"}, nil),
		&mcp.StreamableClientTransport{Endpoint: server.Endpoint, HTTPClient: authenticatedHTTPClient()})
	reader := connect(mcp.NewClient(&mcp.Implementation{Name: "reader", Version: "1"}, nil),
		&mcp.StreamableClientTransport{Endpoint: server.Endpoint})

	call := func(session *mcp.ClientSession, name string, arguments map[string]any, output any) {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
		if err != nil || result.IsError {
			t.Fatalf("%s: %v %+v", name, err, result)
		}
		if output != nil {
			data, _ := json.Marshal(result.StructuredContent)
			if err := json.Unmarshal(data, output); err != nil {
				t.Fatal(err)
			}
		}
	}

	var described describeLibraryOutput
	call(agent, "describe_library", nil, &described)
	if described.InboxPath != filepath.Join(storage, InboxDir) {
		t.Errorf("inboxPath = %q", described.InboxPath)
	}
	tools, err := agent.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != len(described.AvailableTools) {
		t.Errorf("%d tools registered, describe_library lists %d", len(tools.Tools), len(described.AvailableTools))
	}

	var imported ImportResult
	arguments := map[string]any{"batchId": "run-1", "papers": []map[string]any{{"title": "Workflow paper", "doi": "10.1000/flow"}}}
	call(agent, "import_papers", arguments, &imported)
	call(agent, "import_papers", arguments, &imported)
	if imported.Count != 0 || len(imported.Skipped) != 1 {
		t.Fatalf("rerun = %+v", imported)
	}
	paperID := imported.Skipped[0].ExistingID

	if err := os.WriteFile(filepath.Join(storage, InboxDir, "paper.pdf"), []byte("pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	var file PaperFile
	call(agent, "attach_file", map[string]any{"paperId": paperID, "name": "paper.pdf"}, &file)
	var files listFilesOutput
	call(reader, "list_files", map[string]any{"paperId": paperID}, &files)
	if len(files.Files) != 1 || files.Files[0].ID != file.ID || files.Files[0].Type != "fulltext" {
		t.Fatalf("list_files = %+v", files)
	}

	var added DatasetResult
	call(agent, "add_dataset", map[string]any{
		"title": "Workflow dataset", "batchId": "run-1",
		"repositories": []map[string]any{{"repository": "geo", "accession": "GSE9"}},
	}, &added)
	var linked Dataset
	call(agent, "link_dataset", map[string]any{"datasetId": added.Dataset.ID, "paperId": paperID, "role": "generated", "evidenceText": "we deposited"}, &linked)
	if len(linked.PaperLinks) != 1 || linked.PaperLinks[0].Verified {
		t.Fatalf("link_dataset = %+v", linked)
	}
	var evidence evidenceOutput
	call(agent, "add_evidence", map[string]any{"evidence": []map[string]any{
		{"paperId": paperID, "field": "organs", "value": "root", "excerpt": "root tips", "fileId": file.ID},
		{"datasetId": added.Dataset.ID, "field": "accession", "value": "GSE9", "excerpt": "GEO: GSE9", "sourceUrl": "https://example.org"},
	}}, &evidence)
	if len(evidence.Evidence) != 2 {
		t.Fatalf("add_evidence = %+v", evidence)
	}

	if err := store.ReviewEvidence(ctx, evidence.Evidence[0].ID, "rejected"); err != nil {
		t.Fatal(err)
	}
	var verdicts evidenceOutput
	call(reader, "list_evidence", map[string]any{"paperId": paperID}, &verdicts)
	if len(verdicts.Evidence) != 1 || verdicts.Evidence[0].Status != "rejected" {
		t.Fatalf("list_evidence = %+v", verdicts)
	}

	var papers listPapersOutput
	call(reader, "list_papers", map[string]any{"batchId": "run-1", "needsReview": true}, &papers)
	if papers.Total != 1 {
		t.Fatalf("list_papers = %+v", papers)
	}
	var datasets listDatasetsOutput
	call(reader, "list_datasets", map[string]any{"query": "GSE9"}, &datasets)
	if datasets.Total != 1 {
		t.Fatalf("list_datasets = %+v", datasets)
	}
	var fetched Dataset
	call(reader, "get_dataset", map[string]any{"id": added.Dataset.ID}, &fetched)
	if fetched.Title != "Workflow dataset" {
		t.Fatalf("get_dataset = %+v", fetched)
	}

	for name, arguments := range map[string]map[string]any{
		"undo_import":    {"batchId": "run-1"},
		"add_dataset":    {"title": "x"},
		"update_dataset": {"id": added.Dataset.ID, "patch": map[string]any{"title": "x"}},
		"link_dataset":   {"datasetId": added.Dataset.ID, "paperId": paperID, "evidenceText": "x"},
		"add_evidence":   {"evidence": []map[string]any{{"paperId": paperID, "field": "x", "excerpt": "x", "sourceUrl": "https://example.org"}}},
		"attach_file":    {"paperId": paperID, "name": "paper.pdf"},
	} {
		result, err := reader.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
		if err == nil && !result.IsError {
			t.Errorf("%s worked without a token", name)
		}
	}
	for _, name := range []string{"review_evidence", "verify_dataset_link", "verify_dataset_repository"} {
		if result, err := agent.CallTool(ctx, &mcp.CallToolParams{Name: name}); err == nil && !result.IsError {
			t.Errorf("%s is open to agents", name)
		}
	}

	var undone UndoResult
	call(agent, "undo_import", map[string]any{"batchId": "run-1"}, &undone)
	if undone.PapersRemoved != 0 || undone.DatasetsRemoved != 1 || len(undone.Kept) != 1 {
		t.Fatalf("undo_import = %+v", undone) // The rejected evidence means a person worked on the paper.
	}
}
