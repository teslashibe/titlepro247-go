package titlepro247

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Endpoint paths used by the typed comps/history/liens helpers. All three are
// already on pdvReadAllowlist; the helpers route through CallPDVAPI so they
// inherit the allowlist guard, anti-forgery token, and single-session
// self-heal for free.
const (
	pathGetFilter       = "Areas/PDV/api/PDVAPI/GetFilter"
	pathPostCompsData   = "Areas/PDV/api/CompsData/PostCompsData"
	pathPostHistoryData = "Areas/PDV/api/HistoryData/PostHistoryData"
	pathGetHistoryData  = "Areas/PDV/api/HistoryData/GetHistoryData"
	pathSearchLienAlert = "Areas/PDV/api/PDVAPI/SearchLienAlert"
)

// CompsResult is the envelope returned by GetComps. Filter holds the criteria
// payload fetched from GetFilter (Hypothesis A in #133); Comps holds the parsed
// PostCompsData response (sales[]/mls[]/listings[]).
type CompsResult struct {
	FIPS   string         `json:"fips"`
	APN    string         `json:"apn"`
	Filter any            `json:"filter,omitempty"`
	Comps  *PDVAPIResult  `json:"comps"`
	Body   map[string]any `json:"request_body"`
}

// GetComps retrieves comparable sales/MLS listings for a parcel identified by
// fips + apn (both from SearchAddress).
//
// Design (Hypothesis A, see #133): PostCompsData returns an empty result set
// with PID:0 / orderid:null when the criteria body is missing required filter
// fields (most notably BillingTypeID). The SiteX Angular front-end always sends
// the criteria object returned by GET .../GetFilter alongside {apn, fips}, so
// we replicate that: fetch the saved filter first, merge {apn, fips} into it,
// then POST. This is the verifiable-without-live-creds part. The actual
// non-emptiness of the result can only be confirmed with live credentials on a
// high-activity parcel (see #133 acceptance criteria).
//
// TODO(needs live HAR capture, #133): confirm the exact required fields. In
// particular BillingTypeID — its correct value for a standard residential comps
// request is unknown. We deliberately do NOT inject a magic value; if GetFilter
// supplies it, it flows through the merge. If GetFilter does not include it,
// the merged body still omits it and the response may remain empty — that gap
// is documented rather than guessed.
func (c *Client) GetComps(ctx context.Context, fips, apn string) (*CompsResult, error) {
	if strings.TrimSpace(fips) == "" || strings.TrimSpace(apn) == "" {
		return nil, fmt.Errorf("%w: both fips and apn are required", ErrInvalidParams)
	}

	out := &CompsResult{FIPS: fips, APN: apn}

	// Step 1: fetch the saved comps filter (default criteria the browser sends).
	// Best-effort: if it fails or returns a non-object, we fall back to a
	// minimal {apn, fips} body and document the gap.
	filterRes, ferr := c.CallPDVAPI(ctx, "GET", pathGetFilter, nil)
	criteria := map[string]any{}
	if ferr == nil && filterRes != nil {
		out.Filter = filterRes.Data
		if m, ok := filterRes.Data.(map[string]any); ok {
			for k, v := range m {
				criteria[k] = v
			}
		}
	}

	// Step 2: merge the parcel identity. apn/fips win over any filter defaults.
	criteria["apn"] = apn
	criteria["fips"] = fips
	out.Body = criteria

	bodyJSON, err := json.Marshal(criteria)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal comps criteria: %v", ErrInvalidParams, err)
	}

	// Step 3: POST the merged criteria.
	comps, err := c.CallPDVAPI(ctx, "POST", pathPostCompsData, bodyJSON)
	if err != nil {
		return nil, err
	}
	out.Comps = comps
	return out, nil
}

// HistoryResult is the envelope returned by GetHistory. Initiate holds the
// PostHistoryData response (the step that mints a session key); History holds
// the GetHistoryData/{key} response.
type HistoryResult struct {
	FIPS     string         `json:"fips"`
	APN      string         `json:"apn"`
	Body     map[string]any `json:"request_body"`
	Initiate *PDVAPIResult  `json:"initiate"`
	Key      string         `json:"key,omitempty"`
	History  *PDVAPIResult  `json:"history,omitempty"`
}

// GetHistory retrieves sale/transfer history for a parcel. This is a two-step
// flow (see #135): POST .../PostHistoryData to initiate a search and obtain a
// session {key}, then GET .../GetHistoryData/{key} to fetch the results. We
// chain both in one call so the key cannot expire between MCP invocations.
//
// TODO(needs live HAR capture, #133/#135): the documented PostHistoryData body
// is {SearchType, Keyword, Location, State, FIPS, City, Zip} and does NOT
// include apn directly. The mapping from {fips, apn} onto this body is a best
// guess: we send FIPS and place apn in Keyword. The correct SearchType value
// and whether apn belongs in Keyword are unconfirmed. We also do not know the
// exact field name of the returned key, so extractHistoryKey tries the common
// candidates and the helper returns the raw Initiate response either way.
func (c *Client) GetHistory(ctx context.Context, fips, apn string) (*HistoryResult, error) {
	if strings.TrimSpace(fips) == "" || strings.TrimSpace(apn) == "" {
		return nil, fmt.Errorf("%w: both fips and apn are required", ErrInvalidParams)
	}

	body := map[string]any{
		// TODO(live capture): confirm SearchType value + field mapping.
		"FIPS":    fips,
		"Keyword": apn,
	}
	out := &HistoryResult{FIPS: fips, APN: apn, Body: body}

	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal history body: %v", ErrInvalidParams, err)
	}

	initiate, err := c.CallPDVAPI(ctx, "POST", pathPostHistoryData, bodyJSON)
	if err != nil {
		return nil, err
	}
	out.Initiate = initiate

	key := extractHistoryKey(initiate.Data)
	out.Key = key
	if key == "" {
		// Could not determine the session key from the initiate response; return
		// what we have so the caller/agent can inspect it. See TODO above.
		return out, nil
	}

	history, err := c.CallPDVAPI(ctx, "GET", pathGetHistoryData+"/"+url.PathEscape(key), nil)
	if err != nil {
		return out, err
	}
	out.History = history
	return out, nil
}

// extractHistoryKey pulls the session key out of the PostHistoryData response.
// The exact field name is unconfirmed (needs a live capture), so we probe the
// common candidates. Returns "" when none match.
func extractHistoryKey(data any) string {
	m, ok := data.(map[string]any)
	if !ok {
		return ""
	}
	for _, candidate := range []string{"key", "Key", "searchKey", "SearchKey", "id", "Id", "ID"} {
		if v, ok := m[candidate]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
			if s := fmt.Sprintf("%v", v); s != "" && s != "<nil>" {
				return s
			}
		}
	}
	return ""
}

// LiensResult is the envelope returned by GetLiens.
type LiensResult struct {
	FIPS  string        `json:"fips"`
	APN   string        `json:"apn"`
	Query string        `json:"query"`
	Liens *PDVAPIResult `json:"liens"`
}

// GetLiens retrieves open lien/mortgage records for a parcel via the
// allowlisted GET .../SearchLienAlert endpoint (see #135).
//
// TODO(needs live HAR capture, #135): the exact query-parameter shape is
// undocumented ("query params not fully documented" in API.md). We send fips
// and apn as the most likely parameters; the real parameter names (e.g.
// parcelId, or a combined keyword) must be reverse-engineered from a live
// browser session before this can be confirmed to return records.
func (c *Client) GetLiens(ctx context.Context, fips, apn string) (*LiensResult, error) {
	if strings.TrimSpace(fips) == "" || strings.TrimSpace(apn) == "" {
		return nil, fmt.Errorf("%w: both fips and apn are required", ErrInvalidParams)
	}

	q := url.Values{}
	q.Set("fips", fips)
	q.Set("apn", apn)
	query := q.Encode()

	res, err := c.CallPDVAPI(ctx, "GET", pathSearchLienAlert+"?"+query, nil)
	if err != nil {
		return nil, err
	}
	return &LiensResult{FIPS: fips, APN: apn, Query: query, Liens: res}, nil
}
