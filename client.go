package titlepro247

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// getBytes fetches a path under baseURL and returns raw body.
func (c *Client) getBytes(ctx context.Context, path string, query url.Values) ([]byte, int, error) {
	full := baseURL + path
	if len(query) > 0 {
		full += "?" + query.Encode()
	}
	return c.makeRequest(ctx, http.MethodGet, full, nil, "")
}

func (c *Client) makeRequest(ctx context.Context, method, rawURL string, body []byte, contentType string) ([]byte, int, error) {
	if err := c.ensureLoggedIn(ctx); err != nil {
		return nil, 0, err
	}
	return c.doRetried(ctx, method, rawURL, body, contentType)
}

func (c *Client) doRetried(ctx context.Context, method, rawURL string, body []byte, contentType string) ([]byte, int, error) {
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			wait := c.backoff(attempt)
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(wait):
			}
		}
		raw, status, err := c.doRequest(ctx, method, rawURL, body, contentType)
		if err == nil {
			return raw, status, nil
		}
		lastErr = err
		if errors.Is(err, ErrRateLimited) {
			continue
		}
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode >= 500 {
			continue
		}
		return nil, status, err
	}
	return nil, 0, lastErr
}

func (c *Client) ensureLoggedIn(ctx context.Context) error {
	c.authMu.RLock()
	has := c.auth.AuthCookie != ""
	c.authMu.RUnlock()
	if has {
		return nil
	}
	c.loginOnce.Do(func() {
		_, c.loginErr = c.Login(ctx)
	})
	return c.loginErr
}

func (c *Client) doRequest(ctx context.Context, method, rawURL string, body []byte, contentType string) ([]byte, int, error) {
	c.waitForGap(ctx)
	if ctx.Err() != nil {
		return nil, 0, ctx.Err()
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrRequestFailed, err)
	}
	c.setCommonHeaders(req, contentType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrRequestFailed, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("%w: reading body: %v", ErrRequestFailed, err)
	}
	c.captureAuthCookie(resp)

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent,
		http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
		http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return raw, resp.StatusCode, nil
	case http.StatusUnauthorized:
		return nil, resp.StatusCode, ErrUnauthorized
	case http.StatusForbidden:
		return nil, resp.StatusCode, ErrForbidden
	case http.StatusNotFound:
		return nil, resp.StatusCode, ErrNotFound
	case http.StatusTooManyRequests:
		c.gapMu.Lock()
		if earliest := time.Now().Add(60 * time.Second); c.lastReqAt.Before(earliest) {
			c.lastReqAt = earliest
		}
		c.gapMu.Unlock()
		return nil, resp.StatusCode, fmt.Errorf("%w: 429", ErrRateLimited)
	default:
		return nil, resp.StatusCode, &HTTPError{StatusCode: resp.StatusCode, Body: truncate(string(raw), 256)}
	}
}

func (c *Client) setCommonHeaders(req *http.Request, contentType string) {
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", baseURL+"/")
	req.Header.Set("Origin", baseURL)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if cookie := c.cookieString(); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
}

func (c *Client) cookieString() string {
	var parts []string
	c.authMu.RLock()
	if c.auth.AuthCookie != "" {
		parts = append(parts, ".SiteXPro_AUTH="+c.auth.AuthCookie)
	}
	c.authMu.RUnlock()
	if c.httpClient != nil && c.httpClient.Jar != nil {
		u, _ := url.Parse(baseURL)
		for _, ck := range c.httpClient.Jar.Cookies(u) {
			if ck.Name == ".SiteXPro_AUTH" {
				continue
			}
			if ck.Value == "" {
				continue
			}
			parts = append(parts, ck.Name+"="+ck.Value)
		}
	}
	return strings.Join(parts, "; ")
}

func (c *Client) captureAuthCookie(resp *http.Response) {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	for _, ck := range resp.Cookies() {
		if ck.Name == ".SiteXPro_AUTH" && ck.Value != "" {
			c.auth.AuthCookie = ck.Value
		}
	}
}

func (c *Client) backoff(attempt int) time.Duration {
	return time.Duration(math.Pow(2, float64(attempt-1))) * c.retryBase
}

func (c *Client) waitForGap(ctx context.Context) {
	c.gapMu.Lock()
	now := time.Now()
	next := c.lastReqAt.Add(c.minGap)
	if now.After(next) {
		next = now
	}
	c.lastReqAt = next
	c.gapMu.Unlock()
	if wait := time.Until(next); wait > 0 {
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
