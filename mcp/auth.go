package mcp

import (
	"context"

	tp "github.com/teslashibe/titlepro247-go"
	"github.com/teslashibe/mcptool"
)

// LoginInput is the typed input for titlepro247_login.
type LoginInput struct{}

func login(ctx context.Context, c *tp.Client, _ LoginInput) (any, error) {
	return c.Login(ctx)
}

// GetMeInput is the typed input for titlepro247_get_me.
type GetMeInput struct{}

func getMe(ctx context.Context, c *tp.Client, _ GetMeInput) (any, error) {
	return c.GetMe(ctx)
}

var authTools = []mcptool.Tool{
	mcptool.Define[*tp.Client, LoginInput](
		"titlepro247_login",
		"Authenticate against v3.titlepro247.com using the configured username/password and cache .SiteXPro_AUTH.",
		"Login",
		login,
	),
	mcptool.Define[*tp.Client, GetMeInput](
		"titlepro247_get_me",
		"Confirm the cached session is alive and return display info from the /Account page.",
		"GetMe",
		getMe,
	),
}
