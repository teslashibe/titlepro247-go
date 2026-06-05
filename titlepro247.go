// Package titlepro247 is a Go client + MCP tool surface for
// v3.titlepro247.com (TitlePro247, an ICE/SiteX product). There is no
// public API; this is reverse-engineered from the website.
//
// # Authentication
//
// The website posts UserName + Password to /Index.aspx and receives a
// 302 → /Account plus a long-lived first-party cookie named
// .SiteXPro_AUTH. This package replays that flow exactly and caches
// the resulting cookie.
package titlepro247

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"
)

const (
	baseURL          = "https://v3.titlepro247.com"
	loginPath        = "/Index.aspx"
	defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	defaultRetries   = 3
	defaultRetryBase = 500 * time.Millisecond
	// defaultTimeout is the per-request HTTP timeout. v3.titlepro247.com
	// (ICE/SiteX) is a slow upstream whose /Index.aspx and /Account pages
	// routinely take >30s to answer headers, surfacing as "context deadline
	// exceeded (Client.Timeout exceeded while awaiting headers)". 60s gives
	// the upstream room to respond; tune with WithTimeout.
	defaultTimeout = 60 * time.Second
	// defaultMaxTotalWait caps the aggregate wall-clock time a single public
	// entry point (GetMe, Login) may spend across its layered retries. Without
	// it, the per-attempt 60s timeout stacks: GetMe self-heal (getMeRaw →
	// relogin → Login, each retrying ~4×60s) nesting with doRetried's own
	// ~4×60s budget can balloon to ~8 min when the upstream is hard-down. 90s
	// is a sane ceiling that still allows one slow attempt plus a retry; tune
	// with WithMaxTotalWait. Override with 0 (or any non-positive) to disable.
	defaultMaxTotalWait = 90 * time.Second
)

// Client talks to v3.titlepro247.com.
type Client struct {
	auth       Auth
	httpClient *http.Client
	userAgent  string
	maxRetries int
	retryBase  time.Duration
	minGap     time.Duration

	// maxTotalWait bounds the aggregate elapsed time across the layered
	// retries of a single GetMe/Login. <=0 disables the cap.
	maxTotalWait time.Duration

	// baseOverride, when non-empty, replaces the package baseURL for all
	// request construction. It is an unexported test seam (see withBaseURL);
	// production code never sets it, so the public API is unaffected.
	baseOverride string

	gapMu     sync.Mutex
	lastReqAt time.Time

	// loginMu serializes (re)login so concurrent requests on the same
	// client don't trigger a thundering herd of logins against the
	// single-session backend.
	loginMu sync.Mutex

	authMu sync.RWMutex

	// Residential proxy rotation. proxyFunc maps a sticky-session number
	// (starting at 1) to a proxy URL; when set, the client routes through
	// the returned proxy and, on a transport-level failure, advances to the
	// next session (a fresh residential IP) and retries — up to
	// maxProxyRotations times. nil proxyFunc = no proxying.
	proxyFunc         ProxyURLForSession
	maxProxyRotations int
	proxySession      int
	proxyMu           sync.Mutex
}

// ProxyURLForSession maps a sticky-session number to a proxy URL. Session
// numbering starts at 1 and increments on each rotation; a residential
// gateway allocates a distinct IP per session, so returning a URL whose
// session component reflects the argument yields a fresh IP on rotation.
// Return "" to signal "no proxy for this session" (rotation then stops).
type ProxyURLForSession func(session int) string

// Option configures a Client.
type Option func(*Client)

// WithUserAgent overrides the default browser User-Agent string.
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// WithRetry sets retry policy.
func WithRetry(maxRetries int, base time.Duration) Option {
	return func(c *Client) {
		c.maxRetries = maxRetries
		c.retryBase = base
	}
}

// WithTimeout sets the per-request HTTP timeout on the default client. A
// non-positive value is ignored (timeout left at its default). For a slow
// upstream, prefer a higher value; the default is already 60s. If a custom
// client is supplied via WithHTTPClient after this option, that client's own
// Timeout wins, so apply WithTimeout last (or set Timeout on the custom client).
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 && c.httpClient != nil {
			c.httpClient.Timeout = d
		}
	}
}

// WithMaxTotalWait caps the total wall-clock time a single GetMe or Login may
// spend across its layered per-attempt timeouts and retries. This prevents the
// worst case where a 60s per-request timeout stacks across nested retries
// (e.g. GetMe's self-heal driving a relogin that itself retries) into a
// multi-minute hang when the upstream is hard-down. A tighter caller-provided
// context deadline still wins (the earlier of the two applies). A non-positive
// value disables the cap. The default is 90s.
func WithMaxTotalWait(d time.Duration) Option {
	return func(c *Client) { c.maxTotalWait = d }
}

// withBaseURL overrides the upstream base URL. Unexported on purpose: it is a
// test-only seam (point the client at an httptest.Server) and is NOT part of
// the public API.
func withBaseURL(u string) Option {
	return func(c *Client) { c.baseOverride = u }
}

// base returns the effective upstream base URL (test override or the default).
func (c *Client) base() string {
	if c.baseOverride != "" {
		return c.baseOverride
	}
	return baseURL
}

// capContext derives a context bounded by maxTotalWait so per-attempt timeouts
// and retries cannot stack into minutes. context.WithTimeout already honors a
// tighter parent deadline (the earlier of the two wins), so a caller-supplied
// deadline is respected. When the cap is disabled (<=0) the parent is returned
// unchanged with a no-op cancel.
func (c *Client) capContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.maxTotalWait <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.maxTotalWait)
}

// WithHTTPClient overrides the default http.Client. Nil is ignored.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// WithMinRequestGap sets the minimum gap between consecutive requests.
func WithMinRequestGap(d time.Duration) Option {
	return func(c *Client) { c.minGap = d }
}

// WithResidentialProxy routes every request through a residential proxy and
// self-heals stale IPs. fn maps a sticky-session number (starting at 1) to a
// proxy URL; on a transport-level failure (connection timeout, refused, or a
// proxy 5xx) the client advances to the next session — a fresh residential IP
// from the provider's pool — and retries, up to maxRotations times per call.
//
// This exists because some upstreams (TitlePro247 / ICE SiteX) silently drop
// datacenter egress IPs, so a server-side login times out while the same
// credentials work from a residential IP. Passing a nil fn disables proxying.
func WithResidentialProxy(fn ProxyURLForSession, maxRotations int) Option {
	return func(c *Client) {
		c.proxyFunc = fn
		if maxRotations < 0 {
			maxRotations = 0
		}
		c.maxProxyRotations = maxRotations
	}
}

// New constructs a Client. Either (Username + Password) or AuthCookie
// must be supplied.
func New(auth Auth, opts ...Option) (*Client, error) {
	if auth.AuthCookie == "" && (auth.Username == "" || auth.Password == "") {
		return nil, ErrInvalidAuth
	}
	jar, _ := cookiejar.New(nil)
	c := &Client{
		auth:         auth,
		httpClient:   &http.Client{Timeout: defaultTimeout, Jar: jar},
		userAgent:    defaultUserAgent,
		maxRetries:   defaultRetries,
		retryBase:    defaultRetryBase,
		minGap:       400 * time.Millisecond,
		maxTotalWait: defaultMaxTotalWait,
	}
	for _, o := range opts {
		o(c)
	}
	// Route the initial requests through the first sticky session when a
	// residential proxy is configured. Rotation (on failure) bumps the
	// session in doRetried.
	if c.proxyFunc != nil {
		c.proxySession = 1
		c.applyProxy(1)
	}
	// A stored cookie (Auth.AuthCookie, e.g. the host's persisted session)
	// must live in the jar to be sent; a fresh login populates the jar itself.
	c.seedJar()
	return c, nil
}

// AuthSnapshot returns the cached auth credentials.
func (c *Client) AuthSnapshot() Auth {
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	return c.auth
}
