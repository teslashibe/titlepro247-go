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
	Path   string          `json:"path" jsonschema:"description=Allowed read endpoint. POST endpoints take a JSON body (e.g. Areas/PDV/api/CompsData/PostCompsData). GET endpoints take a querystring NOT a body (e.g. PDV/Home/StandardizeAddress?address=...&lastline=...). See API.md.,required"`
	Body   json.RawMessage `json:"body,omitempty" jsonschema:"description=JSON body for POST endpoints only (e.g. comps criteria {fips,apn,...}). Ignored by GET endpoints — put GET args in the path querystring."`
}

func pdvAPI(ctx context.Context, c *tp.Client, in PDVAPIInput) (any, error) {
	return c.CallPDVAPI(ctx, in.Method, in.Path, in.Body)
}

// StandardizeAddressInput is the typed input for titlepro247_standardize_address.
type StandardizeAddressInput struct {
	Address  string `json:"address" jsonschema:"description=Street line e.g. '1524 Abbot Kinney Blvd',required"`
	LastLine string `json:"lastline" jsonschema:"description=USPS last line e.g. 'Venice, CA 90291'"`
}

func standardizeAddress(ctx context.Context, c *tp.Client, in StandardizeAddressInput) (any, error) {
	return c.StandardizeAddress(ctx, in.Address, in.LastLine)
}

// ParcelInput is the typed input shared by the comps/history/liens tools — all
// keyed off fips + apn from titlepro247_search_address.
type ParcelInput struct {
	FIPS string `json:"fips" jsonschema:"description=County FIPS code from search_address e.g. '06037',required"`
	APN  string `json:"apn" jsonschema:"description=Assessor parcel number from search_address e.g. '4238-005-027',required"`
}

func getComps(ctx context.Context, c *tp.Client, in ParcelInput) (any, error) {
	return c.GetComps(ctx, in.FIPS, in.APN)
}

func getHistory(ctx context.Context, c *tp.Client, in ParcelInput) (any, error) {
	return c.GetHistory(ctx, in.FIPS, in.APN)
}

func getLiens(ctx context.Context, c *tp.Client, in ParcelInput) (any, error) {
	return c.GetLiens(ctx, in.FIPS, in.APN)
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
	mcptool.Define[*tp.Client, StandardizeAddressInput](
		"titlepro247_standardize_address",
		"Normalize/parse a free-form address via PDV (GET address+lastline). Returns the standardized address payload.",
		"StandardizeAddress",
		standardizeAddress,
	),
	mcptool.Define[*tp.Client, ParcelInput](
		"titlepro247_get_comps",
		"Get comparable sales/MLS listings for a parcel by fips+apn. Merges the saved GetFilter criteria, then posts comps.",
		"GetComps",
		getComps,
	),
	mcptool.Define[*tp.Client, ParcelInput](
		"titlepro247_get_history",
		"Get sale/transfer history for a parcel by fips+apn. Chains the 2-step PostHistoryData then GetHistoryData flow.",
		"GetHistory",
		getHistory,
	),
	mcptool.Define[*tp.Client, ParcelInput](
		"titlepro247_get_liens",
		"Get open lien/mortgage records for a parcel by fips+apn via the read-only SearchLienAlert endpoint.",
		"GetLiens",
		getLiens,
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
