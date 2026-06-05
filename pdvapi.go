package titlepro247

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// PDVAPIResult is the envelope returned by CallPDVAPI: the parsed JSON when
// the response is JSON, otherwise the raw body.
type PDVAPIResult struct {
	Method     string `json:"method"`
	Path       string `json:"path"`
	StatusCode int    `json:"status_code"`
	Data       any    `json:"data,omitempty"`
	Raw        string `json:"raw,omitempty"`
	// EmptyBody is true when the server answered (often HTTP 200) with a
	// completely empty response body. Several SiteX endpoints silently return
	// an empty 200 when called with the wrong method/shape (e.g. POSTing to a
	// GET+querystring endpoint like PDV/Home/StandardizeAddress, see #134).
	// We surface this explicitly so the caller/agent sees *something* instead
	// of a result that — because Raw/Data are omitempty — only carries
	// method/path/status_code and looks like a silent success.
	EmptyBody bool `json:"empty_body,omitempty"`
}

// pdvReadAllowlist is the set of authenticated READ/search endpoints an agent
// may call. Everything else (orders, cart, uploads, deletes, state mutations)
// is refused so a tool call can never place a paid order or corrupt account
// state. Matching is by exact path prefix (after normalizing a leading slash).
//
// See API.md for the full surface and the blocked (mutating) endpoints.
var pdvReadAllowlist = []string{
	"Areas/PDV/api/PDVData/GetPhotoLogo",
	"Areas/PDV/api/PDVAPI/Autocomplete",
	"Areas/PDV/api/PDVAPI/SearchLienAlert",
	"Areas/PDV/api/PDVAPI/GetUserInfo",
	"Areas/PDV/api/PDVAPI/GetZoneList",
	"Areas/PDV/api/PDVAPI/GetFilter",
	"Areas/PDV/api/PDVAPI/GetResultsByShape",
	"Areas/PDV/api/PDVAPI/GetCoverSheet",
	"Areas/PDV/api/PDVAPI/GetOpenOrderDetails",
	"Areas/PDV/api/CompsData/PostCompsData",
	"Areas/PDV/api/HistoryData/GetHistoryData",
	"Areas/PDV/api/HistoryData/PostHistoryData",
	"Areas/Lists/api/ListsData/ReadNeighborhoodReport",
	"Areas/Lists/api/ListsData/ReadStats",
	"Areas/Lists/api/ListsData/SearchLocations",
	"Areas/Lists/api/ListsData/GetPins",
	"Areas/Lists/api/ListsData/GetFilters",
	"Areas/Orders/api/OrdersData/GetStatus",
	"Areas/DocumentRetrieval/api/DocumentData/GetAvailableDocuments",
	"PDV/Home/StandardizeAddress",
	"PDV/Home/GetUserProducts",
	"PDV/Home/GetUserInfo",
}

// normalizeAPIPath strips a leading slash and any query string for allowlist
// matching, and returns the request path (leading slash, query preserved).
func normalizeAPIPath(path string) (matchKey, reqPath string) {
	p := strings.TrimSpace(path)
	p = strings.TrimPrefix(p, "/")
	reqPath = "/" + p
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	return p, reqPath
}

func pdvPathAllowed(matchKey string) bool {
	for _, a := range pdvReadAllowlist {
		if matchKey == a || strings.HasPrefix(matchKey, a+"/") {
			return true
		}
	}
	return false
}

// CallPDVAPI calls an allowlisted authenticated SiteX/PDV API endpoint and
// returns the parsed response. method is "GET" or "POST"; body (POST only) is
// marshaled to JSON. Only read/search endpoints are permitted (see
// pdvReadAllowlist); order/cart/upload/mutation endpoints are refused with
// ErrForbidden so an agent can't spend money or change account state.
//
// Auth, anti-forgery token, the XHR header, and single-session self-heal are
// handled exactly like SearchAddress.
func (c *Client) CallPDVAPI(ctx context.Context, method, path string, body json.RawMessage) (*PDVAPIResult, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	if method != http.MethodGet && method != http.MethodPost {
		return nil, fmt.Errorf("%w: method must be GET or POST", ErrInvalidParams)
	}
	matchKey, reqPath := normalizeAPIPath(path)
	if !pdvPathAllowed(matchKey) {
		return nil, fmt.Errorf("%w: endpoint %q is not an allowed read endpoint (order/cart/mutation endpoints are blocked; see API.md)", ErrForbidden, matchKey)
	}

	res, err := c.callPDVAPIOnce(ctx, method, reqPath, body)
	if isUnauthorized(err) && c.canRelogin() {
		c.invalidateAuth()
		if lerr := c.relogin(ctx); lerr != nil {
			return nil, lerr
		}
		res, err = c.callPDVAPIOnce(ctx, method, reqPath, body)
	}
	return res, err
}

func (c *Client) callPDVAPIOnce(ctx context.Context, method, reqPath string, body json.RawMessage) (*PDVAPIResult, error) {
	if err := c.ensureLoggedIn(ctx); err != nil {
		return nil, err
	}
	token := c.antiForgeryToken(ctx)

	var raw []byte
	var status int
	var err error
	if method == http.MethodGet {
		raw, status, err = c.getJSONWithToken(ctx, reqPath, token)
	} else {
		raw, status, err = c.postJSONWithToken(ctx, reqPath, token, body)
	}
	if err != nil {
		return nil, err
	}
	out := &PDVAPIResult{Method: method, Path: reqPath, StatusCode: status}
	if len(bytes.TrimSpace(raw)) == 0 {
		// Empty 200 is a common silent failure mode (wrong method/shape).
		// Make it visible instead of letting omitempty drop it. See #134.
		out.EmptyBody = true
		out.Raw = "(empty body)"
		return out, nil
	}
	var parsed any
	if json.Unmarshal(raw, &parsed) == nil {
		out.Data = parsed
	} else {
		out.Raw = string(raw)
	}
	return out, nil
}

// postJSONWithToken is the POST counterpart of getJSONWithToken.
func (c *Client) postJSONWithToken(ctx context.Context, reqPath, token string, body json.RawMessage) ([]byte, int, error) {
	if len(body) == 0 {
		body = json.RawMessage("{}")
	}
	build := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+reqPath, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrRequestFailed, err)
		}
		c.setCommonHeaders(req, "application/json")
		req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		if token != "" {
			req.Header.Set("__RequestVerificationToken", token)
		}
		return req, nil
	}
	req, err := build()
	if err != nil {
		return nil, 0, err
	}
	raw, status, err := c.execute(ctx, req)
	if err != nil {
		return nil, status, err
	}
	if isAuthDeniedBody(raw) {
		return nil, status, ErrUnauthorized
	}
	return raw, status, nil
}
