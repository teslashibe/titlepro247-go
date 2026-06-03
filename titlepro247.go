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
)

// Client talks to v3.titlepro247.com.
type Client struct {
	auth       Auth
	httpClient *http.Client
	userAgent  string
	maxRetries int
	retryBase  time.Duration
	minGap     time.Duration

	gapMu     sync.Mutex
	lastReqAt time.Time

	// loginMu serializes (re)login so concurrent requests on the same
	// client don't trigger a thundering herd of logins against the
	// single-session backend.
	loginMu sync.Mutex

	authMu sync.RWMutex
}

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

// New constructs a Client. Either (Username + Password) or AuthCookie
// must be supplied.
func New(auth Auth, opts ...Option) (*Client, error) {
	if auth.AuthCookie == "" && (auth.Username == "" || auth.Password == "") {
		return nil, ErrInvalidAuth
	}
	jar, _ := cookiejar.New(nil)
	c := &Client{
		auth:       auth,
		httpClient: &http.Client{Timeout: 30 * time.Second, Jar: jar},
		userAgent:  defaultUserAgent,
		maxRetries: defaultRetries,
		retryBase:  defaultRetryBase,
		minGap:     400 * time.Millisecond,
	}
	for _, o := range opts {
		o(c)
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
