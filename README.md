# titlepro247-go

Private Go client + MCP server for [v3.titlepro247.com](https://v3.titlepro247.com)
(TitlePro247, ICE/SiteX). There is no public API — reverse-engineered.

## Authentication

`POST /Index.aspx` (`application/x-www-form-urlencoded`) with
`UserName=...&Password=...&RememberMe=false&View=`. On success the
server responds 302 → `/Account` and sets a long-lived first-party
`.SiteXPro_AUTH` cookie (~500 chars, encrypted blob). Subsequent
requests carry that cookie.

## Quick start (library)

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    tp "github.com/teslashibe/titlepro247-go"
)

func main() {
    c, err := tp.New(tp.Auth{
        Username: os.Getenv("TITLEPRO247_USERNAME"),
        Password: os.Getenv("TITLEPRO247_PASSWORD"),
    })
    if err != nil { log.Fatal(err) }

    me, err := c.Login(context.Background())
    if err != nil { log.Fatal(err) }
    fmt.Printf("logged in=%v title=%q\n", me.LoggedIn, me.DisplayName)
}
```

## Supported operations (v0.1)

| Area  | Methods                                          |
|-------|--------------------------------------------------|
| Auth  | `Login`, `GetMe`, `AuthSnapshot`                 |
| Pages | `GetAccountSummary`, `GetPath`                   |

Useful paths to feed `GetPath`:

- `/Account` — landing
- `/Lists` — saved farms / marketing lists
- `/Orders` — order history
- `/Cart` — current cart
- `/PDV` — Property Detail Viewer
- `/DocumentRetrieval` — title doc retrieval
- `/Home/Coverage` — county coverage map
- `/Profile/Home/Index` / `/Info` / `/Packages` / `/Preferences`

## TODO

- [ ] Typed parsers for `/Lists`, `/Orders`, `/PDV` once we have
      sample HTML to model from.
- [ ] Capture the AJAX endpoints used by the Property Detail Viewer
      and Farm Builder (likely under `/api/` JSON).
- [ ] Add typed search wrappers for owner / address / APN lookups.

## MCP server

```bash
go install github.com/teslashibe/titlepro247-go/cmd/titlepro247-mcp@latest
```

Register with Cursor:

```json
{
  "mcpServers": {
    "titlepro247": { "command": "/Users/you/go/bin/titlepro247-mcp" }
  }
}
```

## License

Private. Internal use only.
