package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"bibliocell/internal/agent"
	"bibliocell/internal/library"
	"bibliocell/internal/settings"
)

const testAdminPassword = "correct horse battery staple"

// testWeb is a web server over a fresh library, with a stand-in frontend.
type testWeb struct {
	server  *webServer
	http    *httptest.Server
	library *library.Store
	dataDir string
}

func newTestWeb(t *testing.T, password string) *testWeb {
	t.Helper()
	dataDir := t.TempDir()
	settingsStore, err := openSettings(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	store, err := library.Open(settingsStore.Get().DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	frontend := fstest.MapFS{
		"index.html":    {Data: []byte("<!doctype html><title>BiblioCell</title>")},
		"assets/app.js": {Data: []byte("console.log('app')")},
	}
	service := &Library{store: store, settings: settingsStore, events: newEventHub()}
	server, err := newWebServer(service, frontend, password, "test")
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.handler())
	t.Cleanup(ts.Close)
	return &testWeb{server: server, http: ts, library: store, dataDir: dataDir}
}

// call invokes a Library method the way the page does and decodes the reply.
func (w *testWeb) call(t *testing.T, cookie *http.Cookie, method string, args ...any) (status int, reply webReply, raw json.RawMessage) {
	t.Helper()
	encodedArgs := make([]json.RawMessage, 0, len(args))
	for _, arg := range args {
		data, err := json.Marshal(arg)
		if err != nil {
			t.Fatal(err)
		}
		encodedArgs = append(encodedArgs, data)
	}
	body, err := json.Marshal(map[string]any{"method": method, "args": encodedArgs})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, w.http.URL+"/api/call", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var envelope struct {
		OK    bool            `json:"ok"`
		Value json.RawMessage `json:"value"`
		Error string          `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&envelope); err != nil {
		t.Fatalf("%s: decode reply: %v", method, err)
	}
	return res.StatusCode, webReply{OK: envelope.OK, Error: envelope.Error}, envelope.Value
}

// mustCall fails the test unless the call succeeded, and decodes its value.
func (w *testWeb) mustCall(t *testing.T, cookie *http.Cookie, target any, method string, args ...any) {
	t.Helper()
	status, reply, raw := w.call(t, cookie, method, args...)
	if status != http.StatusOK || !reply.OK {
		t.Fatalf("%s: status %d, reply %+v", method, status, reply)
	}
	if target != nil {
		if err := json.Unmarshal(raw, target); err != nil {
			t.Fatalf("%s: decode value %s: %v", method, raw, err)
		}
	}
}

func (w *testWeb) login(t *testing.T, password string) (*http.Response, *http.Cookie) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"password": password})
	res, err := http.Post(w.http.URL+"/api/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	for _, cookie := range res.Cookies() {
		if cookie.Name == webSessionCookie {
			return res, cookie
		}
	}
	return res, nil
}

func (w *testWeb) session(t *testing.T, cookie *http.Cookie) webSessionInfo {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, w.http.URL+"/api/session", nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var info webSessionInfo
	if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
		t.Fatal(err)
	}
	return info
}

func TestWebMethodsAreClassified(t *testing.T) {
	typ := reflect.TypeFor[*Library]()
	for i := range typ.NumMethod() {
		name := typ.Method(i).Name
		if _, ok := webMethodAccess[name]; !ok {
			t.Errorf("Library.%s is not classified in webMethodAccess: say whether viewers, admins or only the desktop may call it", name)
		}
	}
	for name := range webMethodAccess {
		if _, ok := typ.MethodByName(name); !ok {
			t.Errorf("webMethodAccess names Library.%s, which does not exist", name)
		}
	}
}

func TestWebViewerReadsButCannotWrite(t *testing.T) {
	w := newTestWeb(t, testAdminPassword)
	ctx := context.Background()
	paper, err := w.library.AddPaper(ctx, library.Paper{Title: "Viewer paper", DOI: "10.1000/web"})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.server.library.settings.Save(settings.Config{
		StoragePath: w.server.library.settings.Get().StoragePath, FontSize: 17, MCPToken: w.server.library.settings.Get().MCPToken,
		NCBIAPIKey: "secret-ncbi", ContactEmail: "lab@example.org",
		Models: []settings.ModelProfile{{ID: "m1", Name: "Claude", Config: agent.Config{Protocol: "anthropic", APIKey: "sk-secret", Model: "claude-opus-5-5"}}},
	}); err != nil {
		t.Fatal(err)
	}

	info := w.session(t, nil)
	if !info.Web || info.Admin || !info.CanLogin {
		t.Fatalf("anonymous session = %+v", info)
	}

	var papers []library.Paper
	w.mustCall(t, nil, &papers, "Library.ListPapers")
	if len(papers) != 1 || papers[0].ID != paper.ID {
		t.Fatalf("viewer list = %+v", papers)
	}

	// Credentials and server paths never reach a viewer.
	var config map[string]any
	w.mustCall(t, nil, &config, "Library.GetSettings")
	if config["fontSize"] != float64(17) {
		t.Errorf("viewer settings lost the font size: %v", config)
	}
	for _, key := range []string{"ncbiApiKey", "contactEmail", "mcpToken", "storagePath"} {
		if value, _ := config[key].(string); value != "" {
			t.Errorf("viewer settings expose %s=%q", key, value)
		}
	}
	if models, _ := config["models"].([]any); len(models) != 0 {
		t.Errorf("viewer settings expose models: %v", models)
	}
	var status map[string]any
	w.mustCall(t, nil, &status, "Library.Status")
	if status["paperCount"] != float64(1) || status["storagePath"] != "" || status["databasePath"] != "" {
		t.Errorf("viewer status = %v", status)
	}
	var mcp MCPInfo
	w.mustCall(t, nil, &mcp, "Library.MCPInfo")
	if mcp.Enabled || mcp.Token != "" {
		t.Errorf("viewer MCP info = %+v", mcp)
	}

	// Writes and the admin pages are refused, the desktop's dialogs do not exist.
	edit := library.PaperEdit{Before: &papers[0], After: papers[0]}
	edit.After.Notes = "sneaky"
	for name, args := range map[string][]any{
		"Library.SavePaperEdits":    {[]library.PaperEdit{edit}},
		"Library.ListTrash":         {},
		"Library.RestoreTrash":      {"paper", paper.ID},
		"Library.DeletePaper":       {paper.ID},
		"Library.ListSources":       {},
		"Library.SyncSource":        {"pubmed"},
		"Library.SaveSettings":      {settings.Config{}},
		"Library.AgentChat":         {[]any{}},
		"Library.MarkPaperReviewed": {paper.ID},
	} {
		status, reply, _ := w.call(t, nil, name, args...)
		if status != http.StatusForbidden || reply.OK {
			t.Errorf("%s as viewer: status %d, reply %+v", name, status, reply)
		}
	}
	for _, name := range []string{"Library.ImportPaperFiles", "Library.ChooseStoragePath", "Library.Restart", "Library.OpenPaperFile", "Library.NoSuchMethod"} {
		status, reply, _ := w.call(t, nil, name, "x", "y")
		if status != http.StatusNotFound || reply.OK {
			t.Errorf("%s on the web: status %d, reply %+v", name, status, reply)
		}
	}
	got, err := w.library.GetPaper(ctx, paper.ID)
	if err != nil || got.Notes != "" {
		t.Fatalf("viewer changed the paper: %+v %v", got, err)
	}
}

func TestWebAdminSignsInAndWrites(t *testing.T) {
	w := newTestWeb(t, testAdminPassword)
	ctx := context.Background()
	paper, err := w.library.AddPaper(ctx, library.Paper{Title: "Admin paper"})
	if err != nil {
		t.Fatal(err)
	}

	res, cookie := w.login(t, "wrong")
	if res.StatusCode != http.StatusUnauthorized || cookie != nil {
		t.Fatalf("wrong password: status %d, cookie %v", res.StatusCode, cookie)
	}
	res, cookie = w.login(t, testAdminPassword)
	if res.StatusCode != http.StatusOK || cookie == nil || !cookie.HttpOnly {
		t.Fatalf("right password: status %d, cookie %+v", res.StatusCode, cookie)
	}
	if info := w.session(t, cookie); !info.Admin {
		t.Fatalf("admin session = %+v", info)
	}

	var papers []library.Paper
	w.mustCall(t, cookie, &papers, "Library.ListPapers")
	edit := library.PaperEdit{Before: &papers[0], After: papers[0]}
	edit.After.Notes = "checked on the web"
	var conflicts []string
	w.mustCall(t, cookie, &conflicts, "Library.SavePaperEdits", []library.PaperEdit{edit})
	if len(conflicts) != 0 {
		t.Fatalf("conflicts = %v", conflicts)
	}
	got, err := w.library.GetPaper(ctx, paper.ID)
	if err != nil || got.Notes != "checked on the web" {
		t.Fatalf("admin edit not saved: %+v %v", got, err)
	}
	var sources []library.SourceStatus
	w.mustCall(t, cookie, &sources, "Library.ListSources")
	if len(sources) == 0 {
		t.Fatal("admin cannot list sources")
	}
	var config settings.Config
	w.mustCall(t, cookie, &config, "Library.GetSettings")
	if config.StoragePath == "" || config.MCPToken == "" {
		t.Fatalf("admin settings are redacted: %+v", config)
	}

	// The server's folder and MCP token are not the page's to change.
	saved := config
	saved.StoragePath = filepath.Join(t.TempDir(), "elsewhere")
	saved.MCPToken = strings.Repeat("x", 40)
	saved.FontSize = 19
	var result SettingsSaveResult
	w.mustCall(t, cookie, &result, "Library.SaveSettings", saved)
	if result.RestartRequired || result.Settings.StoragePath != config.StoragePath || result.Settings.MCPToken != config.MCPToken || result.Settings.FontSize != 19 {
		t.Fatalf("web SaveSettings = %+v", result)
	}

	// The same API used by the frontend archives and restores a record.
	w.mustCall(t, cookie, nil, "Library.DeletePaper", paper.ID)
	var trash []library.TrashEntry
	w.mustCall(t, cookie, &trash, "Library.ListTrash")
	if len(trash) != 1 || trash[0].ID != paper.ID {
		t.Fatalf("trash: %+v", trash)
	}
	w.mustCall(t, cookie, nil, "Library.RestoreTrash", "paper", paper.ID)
	if restored, err := w.library.GetPaper(ctx, paper.ID); err != nil || restored.Notes != "checked on the web" {
		t.Fatalf("restore: %+v %v", restored, err)
	}

	// A method error travels as the reply's error, not an HTTP failure.
	status, reply, _ := w.call(t, cookie, "Library.DeletePaper", "P-missing")
	if status != http.StatusOK || reply.OK || reply.Error == "" {
		t.Fatalf("missing paper: status %d, reply %+v", status, reply)
	}

	req, _ := http.NewRequest(http.MethodPost, w.http.URL+"/api/logout", nil)
	req.AddCookie(cookie)
	if res, err := http.DefaultClient.Do(req); err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("logout: %v %v", res, err)
	}
	if info := w.session(t, cookie); info.Admin {
		t.Fatal("session survives logout")
	}
	if status, _, _ := w.call(t, cookie, "Library.ListSources"); status != http.StatusForbidden {
		t.Fatalf("old cookie still works: %d", status)
	}
}

func TestWebWithoutPasswordStaysReadOnly(t *testing.T) {
	w := newTestWeb(t, "")
	if info := w.session(t, nil); info.CanLogin || info.Admin {
		t.Fatalf("session = %+v", info)
	}
	res, cookie := w.login(t, "")
	if res.StatusCode != http.StatusNotFound || cookie != nil {
		t.Fatalf("login without a password configured: %d %v", res.StatusCode, cookie)
	}
}

func TestWebLoginIsRateLimited(t *testing.T) {
	w := newTestWeb(t, testAdminPassword)
	w.server.limiter = newLoginLimiter(2, time.Minute)
	for range 2 {
		if res, _ := w.login(t, "nope"); res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("status %d", res.StatusCode)
		}
	}
	if res, _ := w.login(t, testAdminPassword); res.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("after the limit: status %d", res.StatusCode)
	}
}

func TestWebRejectsCrossSiteCalls(t *testing.T) {
	w := newTestWeb(t, testAdminPassword)
	for _, route := range []string{"/api/call", "/api/login", "/api/logout"} {
		req, _ := http.NewRequest(http.MethodPost, w.http.URL+route, strings.NewReader(`{"method":"Library.Status","args":[],"password":"x"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusForbidden {
			t.Errorf("%s from another site: status %d", route, res.StatusCode)
		}
	}
	// A plain form post cannot carry the JSON content type.
	res, err := http.Post(w.http.URL+"/api/call", "application/x-www-form-urlencoded", strings.NewReader("method=Library.Status"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("form post: status %d", res.StatusCode)
	}
}

func TestWebServesFrontendAndStoredFiles(t *testing.T) {
	w := newTestWeb(t, testAdminPassword)
	get := func(path string) (int, string, http.Header) {
		t.Helper()
		res, err := http.Get(w.http.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var body bytes.Buffer
		_, _ = body.ReadFrom(res.Body)
		return res.StatusCode, body.String(), res.Header
	}
	for _, path := range []string{"/", "/index.html", "/papers", "/some/route"} {
		if status, body, _ := get(path); status != http.StatusOK || !strings.Contains(body, "<title>BiblioCell</title>") {
			t.Errorf("GET %s: %d %q", path, status, body)
		}
	}
	if status, body, headers := get("/assets/app.js"); status != http.StatusOK || body != "console.log('app')" || !strings.Contains(headers.Get("Cache-Control"), "immutable") {
		t.Errorf("GET /assets/app.js: %d %q %v", status, body, headers)
	}
	if status, _, _ := get("/missing.js"); status != http.StatusNotFound {
		t.Errorf("GET /missing.js: %d", status)
	}
	if status, _, _ := get("/api/nothing"); status != http.StatusNotFound {
		t.Errorf("GET /api/nothing: %d", status)
	}

	ctx := context.Background()
	paper, err := w.library.AddPaper(ctx, library.Paper{Title: "With file"})
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "fulltext.pdf")
	if err := os.WriteFile(source, []byte("%PDF-1.4 test"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := w.library.ImportPaperFile(ctx, paper.ID, source, "fulltext")
	if err != nil {
		t.Fatal(err)
	}
	status, body, headers := get("/paperfile/" + file.ID)
	if status != http.StatusOK || body != "%PDF-1.4 test" || headers.Get("Content-Type") != "application/pdf" {
		t.Errorf("GET /paperfile/%s: %d %q %v", file.ID, status, body, headers)
	}
	if status, _, _ := get("/paperfile/R-missing"); status != http.StatusNotFound {
		t.Errorf("GET /paperfile/R-missing: %d", status)
	}
}

func TestWebEventsReachAdmins(t *testing.T) {
	w := newTestWeb(t, testAdminPassword)
	if res, err := http.Get(w.http.URL + "/api/events"); err != nil || res.StatusCode != http.StatusForbidden {
		t.Fatalf("viewer events: %v %v", res, err)
	}
	_, cookie := w.login(t, testAdminPassword)
	req, _ := http.NewRequest(http.MethodGet, w.http.URL+"/api/events", nil)
	req.AddCookie(cookie)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("admin events: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(res.Body)
	// Lines of the stream, blank separators left out.
	next := func() (string, error) {
		for {
			line, err := reader.ReadString('\n')
			if err != nil || line != "\n" {
				return line, err
			}
		}
	}
	if line, err := next(); err != nil || line != ": connected\n" {
		t.Fatalf("first line %q %v", line, err)
	}
	// Give the handler a moment to subscribe before announcing.
	deadline := time.Now().Add(2 * time.Second)
	for {
		w.server.events.mu.Lock()
		subscribed := len(w.server.events.subs) > 0
		w.server.events.mu.Unlock()
		if subscribed || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	w.server.library.announceSources(library.SourceAutomation{Enabled: true, IntervalHours: 6})
	lines := make(chan string, 4)
	go func() {
		for range 2 {
			line, err := next()
			if err != nil {
				return
			}
			lines <- line
		}
	}()
	var got []string
	for len(got) < 2 {
		select {
		case line := <-lines:
			got = append(got, line)
		case <-time.After(3 * time.Second):
			t.Fatalf("event not delivered; got %q", got)
		}
	}
	if got[0] != "event: sourcesChanged\n" || !strings.Contains(got[1], `"intervalHours":6`) {
		t.Fatalf("event = %q", got)
	}
}

func TestReadAdminPassword(t *testing.T) {
	password, err := readAdminPassword("from-env\n", "", nil)
	if err != nil || password != "from-env" {
		t.Fatalf("env: %q %v", password, err)
	}
	password, err = readAdminPassword("from-env", "secret.txt", func(name string) ([]byte, error) {
		if name != "secret.txt" {
			return nil, fmt.Errorf("unexpected file %s", name)
		}
		return []byte(" from-file \n"), nil
	})
	if err != nil || password != "from-file" {
		t.Fatalf("file: %q %v", password, err)
	}
	if _, err := readAdminPassword("", "missing", os.ReadFile); err == nil {
		t.Fatal("missing file accepted")
	}
}

func TestWebFrontendNeedsABuild(t *testing.T) {
	if _, err := webFrontend(t.TempDir(), nil); err == nil || !strings.Contains(err.Error(), "build:web") {
		t.Fatalf("empty folder: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<title>x</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := webFrontend(dir, nil); err != nil {
		t.Fatalf("folder with a build: %v", err)
	}
	embedded := fstest.MapFS{"index.html": {Data: []byte("x")}}
	if fsys, err := webFrontend("", embedded); err != nil || fsys == nil {
		t.Fatalf("embedded build: %v", err)
	}
}
