// Package proxy decides how BiblioCell's outbound requests reach the network:
// through the system's proxy (the default), directly, or through a proxy URL
// the person typed, with a per-item override that can force a direct
// connection for one model service or literature source.
//
// Every client shares one transport whose Proxy reads the current settings
// at each request, so a change applies to the next request without a
// restart. Go keys idle connections by their proxy, so direct and proxied
// connections are never mixed up.
package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/netproxy"
)

// Global modes.
const (
	// ModeSystem follows the system's proxy settings, PAC files included;
	// HTTP_PROXY and the other environment variables win when set.
	ModeSystem = "system"
	// ModeDirect connects directly and ignores the environment too.
	ModeDirect = "direct"
	// ModeCustom sends every request through ProxySettings.URL.
	ModeCustom = "custom"
)

// Per-item overrides.
const (
	// Inherit follows the global setting.
	Inherit = "inherit"
	// Direct connects directly whatever the global setting says.
	Direct = "direct"
)

// ProxySettings is the global proxy preference. The zero value is ModeSystem.
type ProxySettings struct {
	Mode string `json:"mode"`
	// URL is the proxy for ModeCustom: http://, https:// or socks5://, with
	// an optional user:password@.
	URL string `json:"url"`
}

// Normalize trims what the person typed and fills the default mode.
func Normalize(settings ProxySettings) ProxySettings {
	settings.Mode = strings.ToLower(strings.TrimSpace(settings.Mode))
	if settings.Mode == "" {
		settings.Mode = ModeSystem
	}
	settings.URL = strings.TrimSpace(settings.URL)
	return settings
}

// Validate rejects an unknown mode and, in custom mode, a URL Go's transport
// cannot use as a proxy.
func Validate(settings ProxySettings) error {
	settings = Normalize(settings)
	switch settings.Mode {
	case ModeSystem, ModeDirect:
		return nil
	case ModeCustom:
		_, err := ParseURL(settings.URL)
		return err
	default:
		return fmt.Errorf("代理模式 %q 无效：请选择 system、direct 或 custom", settings.Mode)
	}
}

// ParseURL checks a custom proxy address and returns it parsed.
func ParseURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("请填写代理地址，例如 http://127.0.0.1:7890")
	}
	if strings.ContainsFunc(raw, func(r rune) bool { return r < 33 || r == 127 }) {
		return nil, errors.New("代理地址不能包含空格或控制字符")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Opaque != "" {
		return nil, errors.New("代理地址格式无效，应形如 http://127.0.0.1:7890 或 socks5://user:pass@host:1080")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	switch parsed.Scheme {
	case "http", "https", "socks5":
	default:
		return nil, fmt.Errorf("不支持的代理协议 %q：请使用 http://、https:// 或 socks5://", parsed.Scheme)
	}
	if parsed.Hostname() == "" {
		return nil, errors.New("代理地址缺少主机名")
	}
	if port := parsed.Port(); port != "" {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return nil, errors.New("代理端口应为 1 到 65535")
		}
	} else if strings.HasSuffix(parsed.Host, ":") {
		return nil, errors.New("代理端口应为 1 到 65535")
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("代理地址只填协议、主机和端口，不要带路径或参数")
	}
	parsed.Path = ""
	return parsed, nil
}

// NormalizeOverride maps an empty or unknown-case override to its canonical
// form, so configurations saved before the field existed inherit.
func NormalizeOverride(override string) string {
	override = strings.ToLower(strings.TrimSpace(override))
	if override == "" {
		return Inherit
	}
	return override
}

// ValidateOverride rejects anything but inherit and direct.
func ValidateOverride(override string) error {
	switch NormalizeOverride(override) {
	case Inherit, Direct:
		return nil
	default:
		return fmt.Errorf("代理选项 %q 无效：请选择 inherit 或 direct", override)
	}
}

// Resolve returns the proxy for req under settings and an item's override,
// or nil to connect directly. Loopback hosts are always direct.
func Resolve(settings ProxySettings, override string, req *http.Request) (*url.URL, error) {
	if NormalizeOverride(override) == Direct || isLoopback(req.URL.Hostname()) {
		return nil, nil
	}
	settings = Normalize(settings)
	switch settings.Mode {
	case ModeDirect:
		return nil, nil
	case ModeCustom:
		// Saved settings were validated; a bad URL here would mean a hand
		// edited file, and failing the request says so better than going direct.
		return ParseURL(settings.URL)
	default:
		return systemProxy(req)
	}
}

// systemProxy is replaced by tests.
var systemProxy = netproxy.Proxy

func isLoopback(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// current reads the global settings; Use replaces it.
var current atomic.Pointer[func() ProxySettings]

// Use makes get the source of the global settings, read at each request.
// Until it is called, requests follow the system's proxy.
func Use(get func() ProxySettings) {
	current.Store(&get)
}

func currentSettings() ProxySettings {
	if get := current.Load(); get != nil {
		return (*get)()
	}
	return ProxySettings{}
}

type overrideKey struct{}

// WithOverride marks the requests made with ctx as belonging to an item with
// this override, for clients shared between items.
func WithOverride(ctx context.Context, override string) context.Context {
	return context.WithValue(ctx, overrideKey{}, NormalizeOverride(override))
}

func overrideOf(ctx context.Context) string {
	override, _ := ctx.Value(overrideKey{}).(string)
	return override
}

var shared = sync.OnceValue(func() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = func(req *http.Request) (*url.URL, error) {
		return Resolve(currentSettings(), overrideOf(req.Context()), req)
	}
	return transport
})

// Transport returns the shared transport for an item's override. Requests
// whose context carries WithOverride use that override instead.
func Transport(override string) http.RoundTripper {
	if NormalizeOverride(override) == Direct {
		return directTransport{}
	}
	return shared()
}

// Client returns a client with timeout over Transport(override).
func Client(timeout time.Duration, override string) *http.Client {
	return &http.Client{Timeout: timeout, Transport: Transport(override)}
}

// directTransport marks each request direct and hands it to the shared
// transport, so it keeps its connection pool.
type directTransport struct{}

func (directTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return shared().RoundTrip(req.WithContext(WithOverride(req.Context(), Direct)))
}

func (directTransport) CloseIdleConnections() { shared().CloseIdleConnections() }
