package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func request(t *testing.T, raw string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func fakeSystem(t *testing.T, proxyURL string) {
	t.Helper()
	previous := systemProxy
	systemProxy = func(*http.Request) (*url.URL, error) { return url.Parse(proxyURL) }
	t.Cleanup(func() { systemProxy = previous })
}

func TestResolveModesAndOverrides(t *testing.T) {
	fakeSystem(t, "http://system.example:3128")
	custom := ProxySettings{Mode: ModeCustom, URL: "socks5://user:pa%20ss@custom.example:1080"}
	cases := []struct {
		name     string
		settings ProxySettings
		override string
		want     string
	}{
		{"unset follows the system", ProxySettings{}, "", "http://system.example:3128"},
		{"system inherit", ProxySettings{Mode: ModeSystem}, Inherit, "http://system.example:3128"},
		{"system item direct", ProxySettings{Mode: ModeSystem}, Direct, ""},
		{"direct inherit", ProxySettings{Mode: ModeDirect}, Inherit, ""},
		{"direct item direct", ProxySettings{Mode: ModeDirect}, Direct, ""},
		{"custom inherit", custom, Inherit, "socks5://user:pa%20ss@custom.example:1080"},
		{"custom empty override inherits", custom, "", "socks5://user:pa%20ss@custom.example:1080"},
		{"custom item direct", custom, Direct, ""},
		{"mode is case-insensitive", ProxySettings{Mode: " Custom ", URL: "http://p.example:8080"}, "INHERIT", "http://p.example:8080"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Resolve(c.settings, c.override, request(t, "https://api.example.com/v1"))
			if err != nil {
				t.Fatal(err)
			}
			if (got == nil) != (c.want == "") || (got != nil && got.String() != c.want) {
				t.Fatalf("Resolve = %v, want %q", got, c.want)
			}
		})
	}
}

func TestResolveLoopbackIsAlwaysDirect(t *testing.T) {
	fakeSystem(t, "http://system.example:3128")
	for _, settings := range []ProxySettings{{}, {Mode: ModeCustom, URL: "http://p.example:8080"}} {
		for _, host := range []string{"http://localhost:11434/api", "http://127.0.0.1:8080", "http://[::1]:9000", "http://app.localhost/"} {
			got, err := Resolve(settings, Inherit, request(t, host))
			if err != nil || got != nil {
				t.Fatalf("%s under %+v: got %v, %v; want direct", host, settings, got, err)
			}
		}
	}
}

func TestResolveSystemModeKeepsEnvironment(t *testing.T) {
	// In system mode netproxy decides, and it lets the environment win.
	previous := systemProxy
	t.Cleanup(func() { systemProxy = previous })
	systemProxy = previous
	t.Setenv("HTTPS_PROXY", "http://env.example:8888")
	t.Setenv("NO_PROXY", "")
	got, err := Resolve(ProxySettings{Mode: ModeSystem}, Inherit, request(t, "https://api.example.com/"))
	if err != nil || got == nil || got.Host != "env.example:8888" {
		t.Fatalf("system mode with HTTPS_PROXY: got %v, %v", got, err)
	}
	got, err = Resolve(ProxySettings{Mode: ModeDirect}, Inherit, request(t, "https://api.example.com/"))
	if err != nil || got != nil {
		t.Fatalf("direct mode must ignore HTTPS_PROXY: got %v, %v", got, err)
	}
}

func TestValidate(t *testing.T) {
	valid := []ProxySettings{
		{},
		{Mode: "system"},
		{Mode: "direct", URL: "ignored when not custom"},
		{Mode: "custom", URL: "http://127.0.0.1:7890"},
		{Mode: "custom", URL: "https://proxy.example"},
		{Mode: "custom", URL: "socks5://user:secret@10.0.0.2:1080"},
		{Mode: "custom", URL: " HTTP://proxy.example:8080/ "},
	}
	for _, settings := range valid {
		if err := Validate(settings); err != nil {
			t.Errorf("Validate(%+v) = %v, want nil", settings, err)
		}
	}
	invalid := []ProxySettings{
		{Mode: "pac"},
		{Mode: "custom"},
		{Mode: "custom", URL: "127.0.0.1:7890"},
		{Mode: "custom", URL: "proxy.example"},
		{Mode: "custom", URL: "ftp://proxy.example:21"},
		{Mode: "custom", URL: "socks4://proxy.example:1080"},
		{Mode: "custom", URL: "http://:8080"},
		{Mode: "custom", URL: "http://proxy.example:0"},
		{Mode: "custom", URL: "http://proxy.example:70000"},
		{Mode: "custom", URL: "http://proxy.example:"},
		{Mode: "custom", URL: "http://proxy.example:abc"},
		{Mode: "custom", URL: "http://proxy.example:8080/path"},
		{Mode: "custom", URL: "http://proxy.example:8080?x=1"},
		{Mode: "custom", URL: "http://proxy example:8080"},
	}
	for _, settings := range invalid {
		if err := Validate(settings); err == nil {
			t.Errorf("Validate(%+v) = nil, want an error", settings)
		}
	}
}

func TestValidateOverride(t *testing.T) {
	for _, value := range []string{"", "inherit", "direct", " Direct "} {
		if err := ValidateOverride(value); err != nil {
			t.Errorf("ValidateOverride(%q) = %v", value, err)
		}
	}
	for _, value := range []string{"system", "custom", "proxy"} {
		if err := ValidateOverride(value); err == nil {
			t.Errorf("ValidateOverride(%q) = nil, want an error", value)
		}
	}
	if got := NormalizeOverride(""); got != Inherit {
		t.Fatalf("NormalizeOverride(\"\") = %q, want inherit", got)
	}
}

// TestTransportReadsSettingsPerRequest sends real requests through a fake
// proxy: the shared transport picks up a settings change at the next request,
// a direct client skips the proxy, and so does a context marked direct.
func TestTransportReadsSettingsPerRequest(t *testing.T) {
	var proxied []string
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxied = append(proxied, r.URL.String())
		io.WriteString(w, "via proxy")
	}))
	defer proxyServer.Close()

	settings := ProxySettings{Mode: ModeDirect}
	Use(func() ProxySettings { return settings })
	t.Cleanup(func() { current.Store(nil) })

	// The target is not loopback, so only the fake proxy can answer it.
	const target = "http://paper.invalid/records"
	get := func(client *http.Client, ctx context.Context) (string, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return string(body), nil
	}
	inherit := Client(5*time.Second, Inherit)
	direct := Client(5*time.Second, Direct)

	settings = ProxySettings{Mode: ModeCustom, URL: proxyServer.URL}
	if body, err := get(inherit, context.Background()); err != nil || body != "via proxy" {
		t.Fatalf("custom mode: %q, %v", body, err)
	}
	if len(proxied) != 1 || proxied[0] != target {
		t.Fatalf("proxy saw %v", proxied)
	}
	// A direct item must not reach the proxy: .invalid never resolves,
	// so the request fails instead of being answered by it.
	if _, err := get(direct, context.Background()); err == nil {
		t.Fatal("direct client reached the proxy")
	}
	if _, err := get(inherit, WithOverride(context.Background(), Direct)); err == nil {
		t.Fatal("context marked direct reached the proxy")
	}
	settings = ProxySettings{Mode: ModeDirect}
	if _, err := get(inherit, context.Background()); err == nil {
		t.Fatal("direct mode still used the proxy after the settings changed")
	}
	if len(proxied) != 1 {
		t.Fatalf("proxy saw %d requests, want 1", len(proxied))
	}
}
