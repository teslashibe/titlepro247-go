package titlepro247

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Login posts UserName + Password to /Index.aspx exactly the way the
// site's loginform does:
//
//	POST /Index.aspx
//	Content-Type: application/x-www-form-urlencoded
//	UserName=...&Password=...&RememberMe=false&View=
//
// On success the server responds 302 → /Account and sets the long-
// lived .SiteXPro_AUTH cookie. We disable redirect-following so the
// cookie capture happens before any follow-up GET races.
func (c *Client) Login(ctx context.Context) (*User, error) {
	c.authMu.RLock()
	user := c.auth.Username
	pass := c.auth.Password
	c.authMu.RUnlock()
	if user == "" || pass == "" {
		return nil, fmt.Errorf("%w: username/password required", ErrInvalidAuth)
	}

	form := url.Values{}
	form.Set("UserName", user)
	form.Set("Password", pass)
	form.Set("RememberMe", "false")
	form.Set("View", "")

	// Login bypasses doRetried (it disables redirects to capture the cookie
	// cleanly), so it carries its own residential-proxy rotation: a login
	// against a dead/banned datacenter egress IP times out at the transport
	// layer, and rotating to a fresh sticky session is exactly what recovers
	// it. Bounded by the configured rotation budget (no-op when unproxied).
	buildReq := func() (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+loginPath,
			strings.NewReader(form.Encode()))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidParams, err)
		}
		c.setCommonHeaders(r, "application/x-www-form-urlencoded")
		return r, nil
	}

	var resp *http.Response
	transient := 0
	for rotations := 0; ; rotations++ {
		req, err := buildReq()
		if err != nil {
			return nil, err
		}
		noRedir := *c.httpClient
		noRedir.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
		c.waitForGap(ctx)
		r, derr := noRedir.Do(req)
		if derr == nil {
			resp = r
			break
		}
		// Transport failure (timeout / refused): rotate the egress IP and
		// retry. When no proxy is configured or the budget is spent,
		// rotateProxy returns false.
		if c.rotateProxy(rotations) {
			continue
		}
		// No proxy (or its budget is spent): the slow TitlePro247 upstream
		// often just times out awaiting headers. Retry a bounded number of
		// times with backoff before surfacing the login error.
		if transient < c.maxRetries {
			transient++
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("%w: %v", ErrLoginFailed, ctx.Err())
			case <-time.After(c.backoff(transient)):
			}
			continue
		}
		return nil, fmt.Errorf("%w: %v", ErrLoginFailed, derr)
	}
	defer resp.Body.Close()
	c.captureAuthCookie(resp)

	switch resp.StatusCode {
	case http.StatusFound, http.StatusSeeOther, http.StatusMovedPermanently:
		loc := strings.ToLower(resp.Header.Get("Location"))
		if !strings.Contains(loc, "/account") {
			return nil, fmt.Errorf("%w: unexpected redirect to %q", ErrLoginFailed, resp.Header.Get("Location"))
		}
	case http.StatusOK:
		return nil, fmt.Errorf("%w: 200 (likely bad username/password)", ErrLoginFailed)
	default:
		return nil, fmt.Errorf("%w: HTTP %d", ErrLoginFailed, resp.StatusCode)
	}

	c.authMu.RLock()
	cookie := c.auth.AuthCookie
	c.authMu.RUnlock()
	if cookie == "" {
		return nil, fmt.Errorf("%w: server did not set .SiteXPro_AUTH", ErrLoginFailed)
	}
	// Login posts with redirects disabled to capture the cookie cleanly, so
	// the 302's .SiteXPro_AUTH lands in c.auth but not always in the jar.
	// Since requests now carry auth via the jar (no manual Cookie header),
	// seed it explicitly so the freshly minted session actually travels.
	c.seedJar()
	// The 302 → /Account plus a freshly set .SiteXPro_AUTH cookie is the
	// authoritative success signal. We deliberately do NOT gate success on a
	// follow-up GetMe: under the single-session backend a competing login can
	// briefly bounce /Account to the sign-in form, which would make a
	// genuinely-successful login report failure (and, because callers persist
	// the cookie only on success, never cache it — perpetuating the churn).
	// GetMe is best-effort here, purely to enrich DisplayName.
	u := &User{LoggedIn: true}
	// Best-effort enrichment only. Use getMeRaw (NOT GetMe) so a logged-out
	// bounce here can't trigger relogin → recurse back into Login.
	if me, err := c.getMeRaw(ctx); err == nil && me != nil && me.DisplayName != "" {
		u.DisplayName = me.DisplayName
	}
	return u, nil
}

// userNameRe pulls the first occurrence of a known display name from
// the dashboard. The TitlePro247 dashboard renders the agent's first
// name plain-text inside the header chrome; the most reliable anchor
// today is the welcome line.
var welcomeNameRe = regexp.MustCompile(`(?is)<title>\s*([^<]+?)\s*</title>`)

// GetMe fetches /Account and confirms the cached session is alive,
// self-healing an evicted session: if the stored cookie has been bumped
// (single-session backend) it drops it, logs in fresh, and retries once.
// This matters because the agent typically calls get_me first — without
// self-heal a stale persisted cookie would leave it permanently "expired".
//
// It delegates to getMeRaw, which Login also uses (best-effort, for
// DisplayName). Login must NOT call the self-healing GetMe or it would
// recurse (GetMe → relogin → Login → GetMe → …); the relogin path calls
// getMeRaw instead.
func (c *Client) GetMe(ctx context.Context) (*User, error) {
	u, err := c.getMeRaw(ctx)
	if errors.Is(err, ErrUnauthorized) && c.canRelogin() {
		c.invalidateAuth()
		if lerr := c.relogin(ctx); lerr != nil {
			return nil, lerr
		}
		u, err = c.getMeRaw(ctx)
	}
	return u, err
}

// getMeRaw fetches /Account once (no self-heal). Liveness is detected by the
// ABSENCE of the login form rather than the presence of a "logout" link: a
// logged-out request to /Account 302s to /Home?ReturnUrl=/Account, which
// renders the sign-in form (a UserName field); the authenticated Account page
// has no such field. (The old "contains 'logout'" heuristic was brittle and
// false-negatived valid sessions.)
func (c *Client) getMeRaw(ctx context.Context) (*User, error) {
	body, _, err := c.getBytes(ctx, "/Account", nil)
	if err != nil {
		return nil, fmt.Errorf("GetMe: %w", err)
	}
	html := string(body)
	low := strings.ToLower(html)
	hasLoginForm := strings.Contains(low, `name="username"`) || strings.Contains(low, `id="username"`)
	loggedIn := !hasLoginForm

	u := &User{LoggedIn: loggedIn}
	if m := welcomeNameRe.FindStringSubmatch(html); len(m) > 1 {
		u.DisplayName = strings.TrimSpace(m[1])
	}
	if !loggedIn {
		return u, fmt.Errorf("%w: /Account redirected to the sign-in form", ErrUnauthorized)
	}
	return u, nil
}
