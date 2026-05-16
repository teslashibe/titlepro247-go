package mcp

import (
	"context"

	tp "github.com/teslashibe/titlepro247-go"
	"github.com/teslashibe/mcptool"
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

var pageTools = []mcptool.Tool{
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
