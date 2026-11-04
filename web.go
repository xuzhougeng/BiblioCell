package main

// Web mode: `BiblioCell web` serves the library over HTTP for an intranet.
// Anyone who can reach the page browses the real database read-only; an
// admin who signs in with the configured password can also run source
// updates, review and edit records, and configure the AI models. The page is
// the same frontend the desktop window shows: in a browser it talks to Go
// through /api/call instead of the mygo bridge, and every method of the
// Library service is classified below as readable by anyone, admin-only, or
// desktop-only (a native dialog or the local shell is involved).

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"bibliocell/internal/agent"
	"bibliocell/internal/library"
	"bibliocell/internal/settings"
)

// webAccess says who may call a Library method from a browser.
type webAccess int

const (
	// webRead: anyone who can reach the page.
	webRead webAccess = iota
	// webAdmin: after signing in with the admin password.
	webAdmin
	// webDesktop: never on the web; the method needs the window, a native
	// dialog, the local shell or the app's own updater.
	webDesktop
)

// webMethodAccess classifies every exported method of Library. A method
// missing here is not callable from the web (TestWebMethodsAreClassified
// keeps the list complete).
var webMethodAccess = map[string]webAccess{
	"Status":                   webRead,
	"ListPapers":               webRead,
	"ListDatasets":             webRead,
	"ListPaperFiles":           webRead,
	"ListEvidence":             webRead,
	"ListScreeningSuggestions": webRead,
	"CalibrationReport":        webRead,
	"JudgePresets":             webRead,
	"CachedTranslation":        webRead,
	"LocateFulltextPage":       webRead,
	"GetSettings":              webRead, // credentials are stripped for viewers
	"MCPInfo":                  webRead, // disabled on the web, so nothing to hide
	"UpdateStatus":             webRead, // always "off" on the web

	"DeletePaper":             webAdmin,
	"DeleteDataset":           webAdmin,
	"SourceCheckPending":      webAdmin,
	"SavePaperEdits":          webAdmin,
	"SaveDatasetEdits":        webAdmin,
	"ReviewEvidence":          webAdmin,
	"VerifyDatasetLink":       webAdmin,
	"VerifyDatasetRepository": webAdmin,
	"MarkPaperReviewed":       webAdmin,
	"MarkDatasetReviewed":     webAdmin,
	"FetchFulltext":           webAdmin,
	"LookupDOI":               webAdmin,
	"ExtractPaperMetadata":    webAdmin,
	"ApplyPaperMetadata":      webAdmin,
	"ListSources":             webAdmin,
	"SaveSourceConfig":        webAdmin,
	"SourceQueryHistory":      webAdmin,
	"SyncSource":              webAdmin,
	"SourceAutomation":        webAdmin,
	"SaveSourceAutomation":    webAdmin,
	"SaveSettings":            webAdmin,
	"ScreenPaper":             webAdmin,
	"ScreenPaperWithModel":    webAdmin,
	"ScreenPaperInRun":        webAdmin,
	"JudgePaperInRun":         webAdmin,
	"AdoptJudgements":         webAdmin,
	"TranslateText":           webAdmin,
	"ListAgentConversations":  webAdmin,
	"GetAgentConversation":    webAdmin,
	"SaveAgentConversation":   webAdmin,
	"DeleteAgentConversation": webAdmin,
	"AgentChat":               webAdmin,

	"ImportPaperFiles":  webDesktop, // native file dialog
	"OpenPaperFile":     webDesktop, // opens with the system's application; the web serves /paperfile/ instead
	"RemovePaperFile":   webDesktop, // moves the file to the system trash
	"ExportLibrary":     webDesktop, // native save dialog; the browser downloads the file itself
	"ChooseStoragePath": webDesktop, // native folder dialog
	"Restart":           webDesktop,
	"RestartToUpdate":   webDesktop,
	"RetryUpdate":       webDesktop,
}

// webJSON encodes values the way the mygo bridge does, so the page sees the
// same shapes in both: nil slices as [], durations as nanoseconds.
var webJSON = json.JoinOptions(jsonv1.FormatDurationAsNano(true), jsontext.EscapeForJS(true))

// webMethod is one Library method, resolved for calls from the page.
type webMethod struct {
	name   string
	fn     reflect.Value
	ctx    bool
	params []reflect.Type
	result bool
	hasErr bool
	access webAccess
}

var (
	webContextType = reflect.TypeFor[context.Context]()
	webErrorType   = reflect.TypeFor[error]()
)

// webMethods resolves webMethodAccess against the service; a name that is
// not a method is a programming error.
func webMethods(service *Library) (map[string]*webMethod, error) {
	value := reflect.ValueOf(service)
	methods := map[string]*webMethod{}
	for name, access := range webMethodAccess {
		fn := value.MethodByName(name)
		if !fn.IsValid() {
			return nil, fmt.Errorf("web: Library.%s is not a method", name)
		}
		t := fn.Type()
		m := &webMethod{name: name, fn: fn, access: access}
		for i := range t.NumIn() {
			if i == 0 && t.In(0) == webContextType {
				m.ctx = true
				continue
			}
			m.params = append(m.params, t.In(i))
		}
		switch t.NumOut() {
		case 1:
			if t.Out(0) == webErrorType {
				m.hasErr = true
			} else {
				m.result = true
			}
		case 2:
			m.result, m.hasErr = true, true
		}
		methods[name] = m
	}
	return methods, nil
}

// call decodes the page's arguments, runs the method and returns its result.
// A panic in the method is reported as an error, like the mygo bridge does.
func (m *webMethod) call(ctx context.Context, args []jsontext.Value) (result any, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
			log.Printf("web: panic in Library.%s: %v\n%s", m.name, r, debug.Stack())
		}
	}()
	in := make([]reflect.Value, 0, len(m.params)+1)
	if m.ctx {
		in = append(in, reflect.ValueOf(ctx))
	}
	for i, t := range m.params {
		v := reflect.New(t)
		if i < len(args) {
			if err := json.Unmarshal(args[i], v.Interface(), webJSON); err != nil {
				return nil, fmt.Errorf("invalid argument %d: %w", i+1, err)
			}
		}
		in = append(in, v.Elem())
	}
	out := m.fn.Call(in)
	if m.hasErr {
		if e := out[len(out)-1]; !e.IsNil() {
			return nil, e.Interface().(error)
		}
	}
	if m.result {
		return out[0].Interface(), nil
	}
	return nil, nil
}

// eventHub fans Go events out to the browsers listening on /api/events.
type eventHub struct {
	mu   sync.Mutex
	subs map[chan webEvent]struct{}
}

type webEvent struct {
	name string
	data []byte
}

func newEventHub() *eventHub { return &eventHub{subs: map[chan webEvent]struct{}{}} }

// send delivers an event to every listener; a listener that cannot keep up
// loses it, which the page survives by rereading on its next refresh.
func (h *eventHub) send(name string, payload any) {
	if h == nil {
		return
	}
	data, err := json.Marshal(payload, webJSON)
	if err != nil {
		log.Printf("web: encode event %s: %v", name, err)
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- webEvent{name: name, data: data}:
		default:
		}
	}
}

func (h *eventHub) subscribe() (<-chan webEvent, func()) {
	ch := make(chan webEvent, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
	}
}

// webSessions holds the signed-in admins, in memory: a restart signs
// everyone out, which is fine for an intranet.
type webSessions struct {
	mu     sync.Mutex
	tokens map[string]time.Time
	ttl    time.Duration
}

const webSessionCookie = "bibliocell_admin"

func newWebSessions(ttl time.Duration) *webSessions {
	return &webSessions{tokens: map[string]time.Time{}, ttl: ttl}
}

func (s *webSessions) create() (string, error) {
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(secret[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for t, expiry := range s.tokens {
		if expiry.Before(now) {
			delete(s.tokens, t)
		}
	}
	s.tokens[token] = now.Add(s.ttl)
	return token, nil
}

func (s *webSessions) valid(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, ok := s.tokens[token]
	if !ok {
		return false
	}
	if expiry.Before(time.Now()) {
		delete(s.tokens, token)
		return false
	}
	return true
}

func (s *webSessions) drop(token string) {
	s.mu.Lock()
	delete(s.tokens, token)
	s.mu.Unlock()
}

// loginLimiter slows password guessing: a client that fails too often waits
// for the window to pass.
type loginLimiter struct {
	mu       sync.Mutex
	failures map[string]*loginFailures
	limit    int
	window   time.Duration
}

type loginFailures struct {
	count int
	since time.Time
}

func newLoginLimiter(limit int, window time.Duration) *loginLimiter {
	return &loginLimiter{failures: map[string]*loginFailures{}, limit: limit, window: window}
}

// blocked reports whether the client has used up its attempts.
func (l *loginLimiter) blocked(client string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.failures[client]
	if f == nil {
		return false
	}
	if time.Since(f.since) > l.window {
		delete(l.failures, client)
		return false
	}
	return f.count >= l.limit
}

func (l *loginLimiter) failed(client string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	f := l.failures[client]
	if f == nil || time.Since(f.since) > l.window {
		l.failures[client] = &loginFailures{count: 1, since: time.Now()}
		return
	}
	f.count++
}

func (l *loginLimiter) reset(client string) {
	l.mu.Lock()
	delete(l.failures, client)
	l.mu.Unlock()
}

// webServer is the HTTP front of web mode.
type webServer struct {
	library  *Library
	methods  map[string]*webMethod
	frontend fs.FS
	// password signs an admin in; empty leaves the site read-only for everyone.
	password string
	sessions *webSessions
	limiter  *loginLimiter
	events   *eventHub
	version  string
}

const (
	webSessionTTL      = 7 * 24 * time.Hour
	webLoginLimit      = 10
	webLoginWindow     = 15 * time.Minute
	webCallBodyLimit   = 32 << 20
	webEventsKeepalive = 25 * time.Second
)

// newWebServer wires the library to HTTP. The hub is the one the library
// announces on, so browsers hear source updates and agent progress.
func newWebServer(service *Library, frontend fs.FS, password, version string) (*webServer, error) {
	methods, err := webMethods(service)
	if err != nil {
		return nil, err
	}
	if service.events == nil {
		service.events = newEventHub()
	}
	return &webServer{
		library: service, methods: methods, frontend: frontend, password: password, version: version,
		sessions: newWebSessions(webSessionTTL),
		limiter:  newLoginLimiter(webLoginLimit, webLoginWindow),
		events:   service.events,
	}, nil
}

// handler routes the API, stored files and the frontend.
func (s *webServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/session", s.session)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.HandleFunc("POST /api/call", s.call)
	mux.HandleFunc("GET /api/events", s.eventStream)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		s.fail(w, http.StatusNotFound, "unknown API route")
	})
	mux.HandleFunc("GET /paperfile/{id}", s.paperFile)
	mux.Handle("/", s.static())
	return mux
}

type webSessionInfo struct {
	Web      bool   `json:"web"`
	Admin    bool   `json:"admin"`
	CanLogin bool   `json:"canLogin"`
	Version  string `json:"version"`
}

func (s *webServer) session(w http.ResponseWriter, r *http.Request) {
	s.reply(w, http.StatusOK, webSessionInfo{Web: true, Admin: s.isAdmin(r), CanLogin: s.password != "", Version: s.version})
}

// isAdmin reads the session cookie.
func (s *webServer) isAdmin(r *http.Request) bool {
	cookie, err := r.Cookie(webSessionCookie)
	return err == nil && s.sessions.valid(cookie.Value)
}

// sameSite rejects requests a browser marks as coming from another site.
// JSON bodies already keep plain forms out; this covers fetch from a page on
// another origin of the same intranet.
func sameSite(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "", "same-origin", "none":
		return true
	}
	return false
}

func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *webServer) login(w http.ResponseWriter, r *http.Request) {
	if !sameSite(r) {
		s.fail(w, http.StatusForbidden, "cross-site requests are not accepted")
		return
	}
	if s.password == "" {
		s.fail(w, http.StatusNotFound, "no admin password is configured; the site is read-only")
		return
	}
	client := clientAddress(r)
	if s.limiter.blocked(client) {
		s.fail(w, http.StatusTooManyRequests, "too many failed attempts; try again later")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.UnmarshalRead(http.MaxBytesReader(w, r.Body, 64<<10), &body); err != nil {
		s.fail(w, http.StatusBadRequest, "expected a JSON body with a password")
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(s.password)) != 1 {
		s.limiter.failed(client)
		time.Sleep(500 * time.Millisecond) // A guess costs time, however fast the network is.
		s.fail(w, http.StatusUnauthorized, "wrong password")
		return
	}
	s.limiter.reset(client)
	token, err := s.sessions.create()
	if err != nil {
		s.fail(w, http.StatusInternalServerError, "cannot start a session")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: webSessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: secureRequest(r), MaxAge: int(webSessionTTL / time.Second),
	})
	s.reply(w, http.StatusOK, webSessionInfo{Web: true, Admin: true, CanLogin: true, Version: s.version})
}

// secureRequest tells whether the page came over HTTPS, directly or through
// a reverse proxy that says so.
func secureRequest(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (s *webServer) logout(w http.ResponseWriter, r *http.Request) {
	if !sameSite(r) {
		s.fail(w, http.StatusForbidden, "cross-site requests are not accepted")
		return
	}
	if cookie, err := r.Cookie(webSessionCookie); err == nil {
		s.sessions.drop(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: webSessionCookie, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	s.reply(w, http.StatusOK, webSessionInfo{Web: true, CanLogin: s.password != "", Version: s.version})
}

// webCall is what the page sends: the generated client's method name and
// its positional arguments.
type webCall struct {
	Method string           `json:"method"`
	Args   []jsontext.Value `json:"args"`
}

type webReply struct {
	OK bool `json:"ok"`
	// Value is always present on success: an empty list must reach the page
	// as [], which omitempty would drop.
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

func (s *webServer) call(w http.ResponseWriter, r *http.Request) {
	if !sameSite(r) {
		s.fail(w, http.StatusForbidden, "cross-site requests are not accepted")
		return
	}
	if media, _, _ := strings.Cut(r.Header.Get("Content-Type"), ";"); strings.TrimSpace(media) != "application/json" {
		s.fail(w, http.StatusUnsupportedMediaType, "calls are JSON")
		return
	}
	var req webCall
	if err := json.UnmarshalRead(http.MaxBytesReader(w, r.Body, webCallBodyLimit), &req); err != nil {
		s.fail(w, http.StatusBadRequest, "malformed call: "+err.Error())
		return
	}
	name := strings.TrimPrefix(req.Method, "Library.")
	m := s.methods[name]
	if m == nil || m.access == webDesktop {
		s.fail(w, http.StatusNotFound, "this feature needs the desktop app")
		return
	}
	admin := s.isAdmin(r)
	if m.access == webAdmin && !admin {
		s.fail(w, http.StatusForbidden, "sign in as admin to make changes")
		return
	}
	args := req.Args
	if name == "SaveSettings" {
		// The server's folder and its MCP token are the operator's, set on the
		// command line or in settings.json; the page keeps everything else.
		var config settings.Config
		if len(args) > 0 {
			if err := json.Unmarshal(args[0], &config, webJSON); err != nil {
				s.fail(w, http.StatusBadRequest, "invalid settings: "+err.Error())
				return
			}
		}
		current := s.library.settings.Get()
		config.StoragePath, config.MCPToken = current.StoragePath, current.MCPToken
		encoded, err := json.Marshal(config, webJSON)
		if err != nil {
			s.fail(w, http.StatusBadRequest, "invalid settings: "+err.Error())
			return
		}
		args = []jsontext.Value{encoded}
	}
	result, err := m.call(r.Context(), args)
	if err != nil {
		s.reply(w, http.StatusOK, webReply{Error: err.Error()})
		return
	}
	if !admin {
		result = viewerView(name, result)
	}
	s.reply(w, http.StatusOK, webReply{OK: true, Value: result})
}

// viewerView strips what a viewer has no business seeing from a result:
// where the server keeps its files, and every credential.
func viewerView(name string, result any) any {
	switch value := result.(type) {
	case library.Status:
		value.StoragePath, value.DatabasePath, value.InboxPath = "", "", ""
		return value
	case settings.Config:
		return settings.Config{
			FontSize: value.FontSize, BatchScreenConcurrency: value.BatchScreenConcurrency,
			TranslationPrompt: value.TranslationPrompt, Models: []settings.ModelProfile{},
		}
	case MCPInfo:
		return MCPInfo{Tools: []string{}, Resources: []string{}}
	}
	return result
}

// eventStream sends Go events to an admin's page as server-sent events.
func (s *webServer) eventStream(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(r) {
		s.fail(w, http.StatusForbidden, "sign in as admin to follow updates")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.fail(w, http.StatusInternalServerError, "streaming is not supported here")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no") // nginx would otherwise hold the stream
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	events, cancel := s.events.subscribe()
	defer cancel()
	keepalive := time.NewTicker(webEventsKeepalive)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
		case event := <-events:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.name, event.data)
		}
		flusher.Flush()
	}
}

// paperFile serves a stored file to any reader, as paperfile:// does in the
// window: only files the library knows, never other paths.
func (s *webServer) paperFile(w http.ResponseWriter, r *http.Request) {
	s.library.serveStoredFile(w, r, r.PathValue("id"))
}

// static serves the built frontend. Paths without an extension that match no
// file get index.html, for the hash routes the page uses; hashed assets are
// cached for good, pages are revalidated.
func (s *webServer) static() http.Handler {
	files := http.FileServerFS(s.frontend)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			s.fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if info, err := fs.Stat(s.frontend, name); err != nil || info.IsDir() {
			if path.Ext(name) != "" {
				s.fail(w, http.StatusNotFound, "not found")
				return
			}
			name = "index.html"
		}
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if name == "index.html" {
			// FileServer answers /index.html with a redirect to ./, which would loop here.
			data, err := fs.ReadFile(s.frontend, name)
			if err != nil {
				s.fail(w, http.StatusInternalServerError, "cannot read index.html")
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/" + name
		files.ServeHTTP(w, r2)
	})
}

func (s *webServer) reply(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value, webJSON)
	if err != nil {
		log.Printf("web: encode reply: %v", err)
		data, status = []byte(`{"ok":false,"error":"cannot encode the reply"}`), http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func (s *webServer) fail(w http.ResponseWriter, status int, message string) {
	s.reply(w, status, webReply{Error: message})
}

// serveStoredFile answers with a stored file by its id; see servePaperFile.
func (l *Library) serveStoredFile(w http.ResponseWriter, r *http.Request, id string) {
	if l.store == nil {
		http.Error(w, "library database is not ready", http.StatusServiceUnavailable)
		return
	}
	p, err := l.store.PaperFilePath(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if strings.EqualFold(path.Ext(p), ".pdf") {
		w.Header().Set("Content-Type", "application/pdf")
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, p)
}

// announceSources tells every page a background update started or finished:
// the desktop window through mygo, the browsers of web mode through the hub.
func (l *Library) announceSources(automation library.SourceAutomation) {
	if err := sourcesChanged.Broadcast(automation); err != nil {
		log.Printf("announce source update: %v", err)
	}
	l.events.send("sourcesChanged", automation)
}

// announceAgent shows every page a step of the run in flight.
func (l *Library) announceAgent(event agent.Event) {
	if err := agentProgress.Broadcast(event); err != nil {
		log.Printf("announce agent progress: %v", err)
	}
	l.events.send("agentProgress", event)
}

// readAdminPassword takes the admin password from the environment or a file.
// An empty result leaves the site read-only for everyone.
func readAdminPassword(env string, file string, readFile func(string) ([]byte, error)) (string, error) {
	if file != "" {
		data, err := readFile(file)
		if err != nil {
			return "", fmt.Errorf("read admin password file: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return strings.TrimSpace(env), nil
}

// webFrontend picks the frontend files: a folder on disk when given, else the
// build embedded with `go build -tags webdist`, else ./dist from a plain
// `npm run build:web`.
func webFrontend(dir string, fsys fs.FS) (fs.FS, error) {
	if dir == "" {
		if fsys != nil {
			return fsys, nil
		}
		dir = "dist"
	}
	frontend := os.DirFS(dir)
	if _, err := fs.Stat(frontend, "index.html"); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("no frontend build in %s: run `npm run build:web`, or build the server with `go build -tags webdist`", dir)
		}
		return nil, err
	}
	return frontend, nil
}
