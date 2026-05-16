// Package mcp exposes titlepro247-go as a set of MCP tools.
package mcp

import "github.com/teslashibe/mcptool"

// Provider implements [mcptool.Provider] for titlepro247-go.
type Provider struct{}

// Platform returns "titlepro247".
func (Provider) Platform() string { return "titlepro247" }

// Tools returns every MCP tool, in registration order.
func (Provider) Tools() []mcptool.Tool {
	out := make([]mcptool.Tool, 0, len(authTools)+len(pageTools))
	out = append(out, authTools...)
	out = append(out, pageTools...)
	return out
}
