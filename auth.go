package titlepro247

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+loginPath,
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidParams, err)
	}
	c.setCommonHeaders(req, "application/x-www-form-urlencoded")

	noRedir := *c.httpClient
	noRedir.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	c.waitForGap(ctx)
	resp, err := noRedir.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLoginFailed, err)
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
	return c.GetMe(ctx)
}

// userNameRe pulls the first occurrence of a known display name from
// the dashboard. The TitlePro247 dashboard renders the agent's first
// name plain-text inside the header chrome; the most reliable anchor
// today is the welcome line.
var welcomeNameRe = regexp.MustCompile(`(?is)<title>\s*([^<]+?)\s*</title>`)

// GetMe fetches /Account and confirms the cached cookie is alive.
func (c *Client) GetMe(ctx context.Context) (*User, error) {
	body, _, err := c.getBytes(ctx, "/Account", nil)
	if err != nil {
		return nil, fmt.Errorf("GetMe: %w", err)
	}
	html := string(body)
	loggedIn := strings.Contains(strings.ToLower(html), "logout")

	u := &User{LoggedIn: loggedIn}
	if m := welcomeNameRe.FindStringSubmatch(html); len(m) > 1 {
		u.DisplayName = strings.TrimSpace(m[1])
	}
	if !loggedIn {
		return u, fmt.Errorf("%w: /Account does not show a Logout link", ErrUnauthorized)
	}
	return u, nil
}
