package titlepro247

import (
	"context"
	"regexp"
	"strings"
)

var titleRe = regexp.MustCompile(`(?is)<title>\s*(.*?)\s*</title>`)

// GetAccountSummary fetches /Account and returns a normalized envelope.
func (c *Client) GetAccountSummary(ctx context.Context) (*AccountPage, error) {
	body, status, err := c.getBytes(ctx, "/Account", nil)
	if err != nil {
		return nil, err
	}
	p := &AccountPage{
		URL:          baseURL + "/Account",
		StatusCode:   status,
		ContentBytes: len(body),
	}
	if m := titleRe.FindStringSubmatch(string(body)); len(m) > 1 {
		p.Title = strings.TrimSpace(m[1])
	}
	return p, nil
}

// GetPath fetches the raw HTML of an arbitrary authenticated path
// under v3.titlepro247.com. Useful paths discovered from the dashboard:
//
//   - /Account              — landing
//   - /Lists                — saved farms & marketing lists
//   - /Orders               — order history
//   - /Cart                 — current cart
//   - /PDV                  — Property Detail Viewer
//   - /DocumentRetrieval    — title document retrieval
//   - /Home/Coverage        — county coverage map
//   - /Profile/Home/Index   — agent profile
//   - /Profile/Home/Info
//   - /Profile/Home/Packages
//   - /Profile/Home/Preferences
//
// This is the catch-all reader; agents call it directly to get any
// authenticated page until per-area typed parsers are written.
func (c *Client) GetPath(ctx context.Context, path string) (string, error) {
	body, _, err := c.getBytes(ctx, path, nil)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
