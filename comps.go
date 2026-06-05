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
// Flow: POST .../GetFilter (the saved criteria the browser merges in) → merge
// {apn, fips} → POST .../PostCompsData.
//
// VERIFIED UPSTREAM LIMITATION (#133, confirmed via live probing on parcel
// 06037/4238-005-027, Venice CA): PostCompsData returns HTTP 200 with
// sales:[]/mls:[] and PID:0/orderid:null for *every* body shape tried — bare
// {apn,fips}, with BillingTypeID 0/1, with radius/monthsBack/maxComps, and with
// the GetFilter criteria merged in. GetFilter itself (POST) returns an empty
// body for this account, so there is no criteria object to merge. The PID:0 in
// both the parcel search row and the comps response indicates SiteX never
// resolves an *order context* for the parcel; comps are served only against a
// real order (PID), which is minted by the blocked order/cart endpoints. So
// comparable sales are NOT retrievable through the read-only surface for this
// account tier. This helper issues the correct calls and returns the (empty)
// response verbatim; it does not fabricate data. Closing #133 fully requires
// either a sanctioned order-seeding endpoint or a higher account tier.
//
// NOTE: GetFilter is a POST endpoint (a GET returns HTTP 405 "resource does not
// support http method 'GET'"); earlier docs/this helper using GET were wrong.
func (c *Client) GetComps(ctx context.Context, fips, apn string) (*CompsResult, error) {
	if strings.TrimSpace(fips) == "" || strings.TrimSpace(apn) == "" {
		return nil, fmt.Errorf("%w: both fips and apn are required", ErrInvalidParams)
	}

	out := &CompsResult{FIPS: fips, APN: apn}

	// Step 1: fetch the saved comps filter (default criteria the browser sends).
	// POST, not GET. Best-effort: if it fails or returns a non-object/empty
	// body, we fall back to a minimal {apn, fips} body.
	filterRes, ferr := c.CallPDVAPI(ctx, "POST", pathGetFilter, nil)
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

// GetHistory retrieves sale/transfer history for a parcel. Two-step flow
// (#135): POST .../PostHistoryData initiates the search, then
// GET .../GetHistoryData/{key} fetches results. We chain both in one call.
//
// VERIFIED (#135, live probing): the documented body shape
// {SearchType, Keyword, Location, State, FIPS, City, Zip} is correct — with it,
// PostHistoryData returns HTTP 200 {"success":true} (a bare {fips,apn} body
// returns HTTP 500). However the initiate response contains ONLY {success:true}
// — no session key — and GetHistoryData/{key} returns HTTP 400 "request is
// invalid" for every key we can derive (apn, fips+apn, etc.). The real key is
// an opaque server token surfaced only to the browser, so transfer-history
// RESULTS are not retrievable through the read-only surface. This helper
// performs the (successful) initiate step and returns it; result retrieval is
// gated upstream and documented rather than guessed.
func (c *Client) GetHistory(ctx context.Context, fips, apn string) (*HistoryResult, error) {
	if strings.TrimSpace(fips) == "" || strings.TrimSpace(apn) == "" {
		return nil, fmt.Errorf("%w: both fips and apn are required", ErrInvalidParams)
	}

	// Documented + verified body shape (SearchType 1 = APN search).
	body := map[string]any{
		"SearchType": 1,
		"Keyword":    apn,
		"FIPS":       fips,
		"Location":   "",
		"State":      "",
		"City":       "",
		"Zip":        "",
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

// GetLiens checks the lien-alert status for a parcel via the allowlisted
// .../SearchLienAlert endpoint (see #135).
//
// VERIFIED (#135, live probing): SearchLienAlert is a POST endpoint (a GET
// returns HTTP 405). POSTing {fips, apn} returns HTTP 200 with an alert summary,
// e.g. {"Alerts":"1","IsTPUser":true,"Status":"1"} — i.e. a count/flag of
// lien alerts on the parcel, not itemized lien records. The itemized-detail
// endpoint (GetLienAlert) is intentionally outside pdvReadAllowlist. So this
// helper returns the lien-alert summary (presence + count); full lien detail is
// not available on the read-only surface.
func (c *Client) GetLiens(ctx context.Context, fips, apn string) (*LiensResult, error) {
	if strings.TrimSpace(fips) == "" || strings.TrimSpace(apn) == "" {
		return nil, fmt.Errorf("%w: both fips and apn are required", ErrInvalidParams)
	}

	body, err := json.Marshal(map[string]any{"fips": fips, "apn": apn})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal liens body: %v", ErrInvalidParams, err)
	}
	res, err := c.CallPDVAPI(ctx, "POST", pathSearchLienAlert, body)
	if err != nil {
		return nil, err
	}
	return &LiensResult{FIPS: fips, APN: apn, Query: "POST {fips,apn}", Liens: res}, nil
}
