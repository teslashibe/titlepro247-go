// Command titlepro247-mcp is a stdio MCP server.
//
// Config: ~/.titlepro247-mcp/config.json
//
//	{
//	  "username": "...",
//	  "password": "..."
//	}
//
// Env override: TITLEPRO247_USERNAME, TITLEPRO247_PASSWORD,
// TITLEPRO247_AUTH_COOKIE.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	tp "github.com/teslashibe/titlepro247-go"
	tpmcp "github.com/teslashibe/titlepro247-go/mcp"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type configFile struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	AuthCookie string `json:"auth_cookie,omitempty"`
}

func defaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".titlepro247-mcp", "config.json")
}

func loadAuth() (tp.Auth, error) {
	var cfg configFile
	data, err := os.ReadFile(defaultConfigPath())
	if err != nil && !os.IsNotExist(err) {
		return tp.Auth{}, fmt.Errorf("read config: %w", err)
	}
	if data != nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return tp.Auth{}, fmt.Errorf("parse config: %w", err)
		}
	}
	if v := os.Getenv("TITLEPRO247_USERNAME"); v != "" {
		cfg.Username = v
	}
	if v := os.Getenv("TITLEPRO247_PASSWORD"); v != "" {
		cfg.Password = v
	}
	if v := os.Getenv("TITLEPRO247_AUTH_COOKIE"); v != "" {
		cfg.AuthCookie = v
	}
	if cfg.AuthCookie == "" && (cfg.Username == "" || cfg.Password == "") {
		return tp.Auth{}, fmt.Errorf(
			"titlepro247 credentials not found. Set TITLEPRO247_USERNAME+TITLEPRO247_PASSWORD "+
				"(or TITLEPRO247_AUTH_COOKIE), or fill %s.", defaultConfigPath())
	}
	return tp.Auth{
		Username:   cfg.Username,
		Password:   cfg.Password,
		AuthCookie: cfg.AuthCookie,
	}, nil
}

func main() {
	log.SetOutput(os.Stderr)
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "titlepro247-mcp:", err)
		os.Exit(1)
	}
}

func run() error {
	auth, err := loadAuth()
	if err != nil {
		return err
	}
	client, err := tp.New(auth)
	if err != nil {
		return fmt.Errorf("init client: %w", err)
	}
	s := server.NewMCPServer("titlepro247-mcp", "0.1.0", server.WithToolCapabilities(true))
	for _, t := range (tpmcp.Provider{}).Tools() {
		t := t
		rawSchema, err := json.Marshal(t.InputSchema)
		if err != nil {
			return fmt.Errorf("marshal schema for %s: %w", t.Name, err)
		}
		tool := mcp.NewToolWithRawSchema(t.Name, t.Description, rawSchema)
		s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			raw, err := json.Marshal(req.Params.Arguments)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			result, invokeErr := t.Invoke(ctx, client, raw)
			if invokeErr != nil {
				return mcp.NewToolResultError(invokeErr.Error()), nil
			}
			out, err := json.Marshal(result)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			return mcp.NewToolResultText(string(out)), nil
		})
	}
	return server.ServeStdio(s)
}
