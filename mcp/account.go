package mcp

import (
	"context"
	"encoding/json"

	"github.com/teslashibe/mcptool"
	tp "github.com/teslashibe/titlepro247-go"
)

// GetAccountSummaryInput is the typed input for titlepro247_get_account_summary.
type GetAccountSummaryInput struct{}

func getAccountSummary(ctx context.Context, c *tp.Client, _ GetAccountSummaryInput) (any, error) {
	return c.GetAccountSummary(ctx)
}

// GetPathInput is the typed input for titlepro247_get_path.
type GetPathInput struct {
	Path string `json:"path" jsonschema:"description=Path under v3.titlepro247.com (e.g. /Account /Lists /Orders /PDV /Profile/Home/Index),required"`
}

func getPath(ctx context.Context, c *tp.Client, in GetPathInput) (any, error) {
	html, err := c.GetPath(ctx, in.Path)
	if err != nil {
		return nil, err
	}
	return map[string]any{"path": in.Path, "html": html}, nil
}

// SearchAddressInput is the typed input for titlepro247_search_address.
type SearchAddressInput struct {
	Address      string `json:"address" jsonschema:"description=Street line e.g. '27 Vista Way',required"`
	CityStateZip string `json:"city_state_zip" jsonschema:"description=City and/or state/ZIP e.g. 'Fairfax' or 'Fairfax, CA 94930'"`
}

func searchAddress(ctx context.Context, c *tp.Client, in SearchAddressInput) (any, error) {
	return c.SearchAddress(ctx, in.Address, in.CityStateZip)
}

// PDVAPIInput is the typed input for titlepro247_pdv_api.
type PDVAPIInput struct {
	Method string          `json:"method,omitempty" jsonschema:"description=GET or POST (default GET)"`
	Path   string          `json:"path" jsonschema:"description=Allowed read endpoint e.g. Areas/PDV/api/CompsData/PostCompsData or PDV/Home/StandardizeAddress (see API.md),required"`
	Body   json.RawMessage `json:"body,omitempty" jsonschema:"description=JSON request body for POST (e.g. comps criteria {fips,apn,...})"`
}

func pdvAPI(ctx context.Context, c *tp.Client, in PDVAPIInput) (any, error) {
	return c.CallPDVAPI(ctx, in.Method, in.Path, in.Body)
}

var pageTools = []mcptool.Tool{
	mcptool.Define[*tp.Client, SearchAddressInput](
		"titlepro247_search_address",
		"Look up a property by street address; returns matching parcels (owner, APN, location) from the PDV property database.",
		"SearchAddress",
		searchAddress,
	),
	mcptool.Define[*tp.Client, PDVAPIInput](
		"titlepro247_pdv_api",
		"Call an allowed TitlePro read endpoint (comps, history, liens, neighborhood). Order/cart/upload endpoints are blocked.",
		"CallPDVAPI",
		pdvAPI,
	),
	mcptool.Define[*tp.Client, GetAccountSummaryInput](
		"titlepro247_get_account_summary",
		"Fetch a normalized snapshot of the /Account dashboard page (URL, status, title, byte size).",
		"GetAccountSummary",
		getAccountSummary,
	),
	mcptool.Define[*tp.Client, GetPathInput](
		"titlepro247_get_path",
		"Fetch the raw HTML of any authenticated path under v3.titlepro247.com (e.g. /Lists, /Orders, /PDV, /Profile/Home/Index).",
		"GetPath",
		getPath,
	),
}
