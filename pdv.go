package titlepro247

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// SearchAddress looks up a property by street address via the PDV (Property
// Detail Viewer) data API — the same endpoint the website's search box drives:
//
//	GET /Areas/PDV/api/PDVData/?keyword=...&location=...&searchText=...&pdvSearchType=1&...
//
// keyword is the street line ("27 Vista Way"); location is the city/state/zip
// ("Fairfax" or "Fairfax, CA 94930"). The server matches on searchText, a
// SQL-ish filter we build from the address (replicating the site's client-side
// query builder). Returns the parsed result rows.
//
// Auth: requires a live .SiteXPro_AUTH session (handled by ensureLoggedIn +
// self-heal) and an anti-forgery token (cookie + header pair) which we fetch
// from the PDV page first.
func (c *Client) SearchAddress(ctx context.Context, keyword, location string) (*PDVSearchResult, error) {
	res, err := c.searchAddressOnce(ctx, keyword, location)
	if isUnauthorized(err) && c.canRelogin() {
		c.invalidateAuth()
		if lerr := c.relogin(ctx); lerr != nil {
			return nil, lerr
		}
		res, err = c.searchAddressOnce(ctx, keyword, location)
	}
	return res, err
}

func (c *Client) searchAddressOnce(ctx context.Context, keyword, location string) (*PDVSearchResult, error) {
	if err := c.ensureLoggedIn(ctx); err != nil {
		return nil, err
	}
	token := c.antiForgeryToken(ctx) // best-effort; empty is tolerated

	q := url.Values{}
	q.Set("keyword", keyword)
	q.Set("location", location)
	q.Set("searchText", buildSearchText(keyword))
	q.Set("pdvSearchType", "1")
	q.Set("nationwide", "false")
	q.Set("fips", "")
	q.Set("ltype", "")
	q.Set("state", "")
	q.Set("zip", "")
	q.Set("city", "")
	q.Set("lid", "")
	q.Set("lvalue", "")
	q.Set("_search", "false")
	q.Set("rows", "25")
	q.Set("page", "1")
	q.Set("sidx", "3")
	q.Set("sord", "asc")

	raw, status, err := c.getJSONWithToken(ctx, "/Areas/PDV/api/PDVData/?"+q.Encode(), token)
	if err != nil {
		return nil, err
	}
	out := &PDVSearchResult{StatusCode: status, Keyword: keyword, Location: location}
	// The endpoint returns a jqGrid-style envelope. Keep the raw rows so the
	// caller (and the agent) always has the full payload, and surface a parsed
	// view when the shape matches.
	if err := json.Unmarshal(raw, &out.Grid); err != nil {
		out.Raw = string(raw)
		return out, nil
	}
	return out, nil
}

// PDVSearchResult is the envelope returned by SearchAddress.
type PDVSearchResult struct {
	Keyword    string   `json:"keyword"`
	Location   string   `json:"location"`
	StatusCode int      `json:"status_code"`
	Grid       *PDVGrid `json:"grid,omitempty"`
	Raw        string   `json:"raw,omitempty"`
}

// PDVGrid is the response envelope (page/total/records/rows). Each row is a
// property object with named fields — APN, Address, City, State, Zip,
// OwnerName, Use, Latitude/Longitude, FIPS, etc. We keep rows as flexible
// maps so every field the API returns reaches the agent without this client
// needing to track the full (and evolving) column set.
type PDVGrid struct {
	Page    int              `json:"page"`
	Total   int              `json:"total"`
	Records int              `json:"records"`
	Rows    []map[string]any `json:"rows"`
}

var (
	leadingNumRe = regexp.MustCompile(`^\d+`)
	sqlQuoteRe   = strings.NewReplacer("'", "''")
)

// buildSearchText replicates the site's client-side query builder. For
// "27 Vista Way" it produces:
//
//	((((StreetName LIKE 'vista%') AND (StreetType = 'way'))) OR
//	 (StreetName LIKE 'vista way%')) AND HouseNumberInt = '27'
//
// It splits a leading house number, then treats the final token as a possible
// street type (with a fallback OR branch matching the whole street string).
func buildSearchText(keyword string) string {
	kw := strings.TrimSpace(keyword)
	house := ""
	if m := leadingNumRe.FindString(kw); m != "" {
		house = m
		kw = strings.TrimSpace(kw[len(m):])
	}
	tokens := strings.Fields(strings.ToLower(kw))
	var clause string
	switch {
	case len(tokens) == 0:
		clause = ""
	case len(tokens) == 1:
		clause = fmt.Sprintf("(StreetName LIKE '%s%%')", esc(tokens[0]))
	default:
		name := esc(strings.Join(tokens[:len(tokens)-1], " "))
		typ := esc(tokens[len(tokens)-1])
		full := esc(strings.Join(tokens, " "))
		clause = fmt.Sprintf("((((StreetName LIKE '%s%%') AND (StreetType = '%s'))) OR (StreetName LIKE '%s%%'))", name, typ, full)
	}
	if house != "" {
		if clause != "" {
			clause += " AND "
		}
		clause += fmt.Sprintf("HouseNumberInt = '%s'", esc(house))
	}
	return clause
}

func esc(s string) string { return sqlQuoteRe.Replace(s) }
