# TitlePro247 / SiteX (v3.titlepro247.com) — reverse-engineered API

There is no public API. This documents the endpoints the website's Angular
front-end calls, discovered by traversing the `/PDV` page's script bundles
(`/bundles/reportsTab`, `/bundles/quicksearch`, `/bundles/sitex`) and probing
the live service with an authenticated session.

## Auth

1. `POST /Index.aspx` with form `UserName, Password, RememberMe=false, View=`
   → `302 /Account` and sets the long-lived `.SiteXPro_AUTH` cookie.
2. JSON/XHR endpoints additionally require an anti-forgery token: `GET /PDV`
   sets a `__RequestVerificationToken` cookie and embeds the matching hidden
   field; send that value as the `__RequestVerificationToken` request header
   plus `X-Requested-With: XMLHttpRequest`.
3. Cookies must be carried by a cookie jar (NOT a duplicated manual header) —
   the SiteX `/Areas/PDV` backend rejects a doubled `.SiteXPro_AUTH`.
4. Single active session per account: a new login elsewhere (e.g. a browser)
   evicts this one. The client self-heals (re-login + retry) on the soft-200
   `{"Message":"Authorization has been denied for this request."}` body.

All paths below are relative to `https://v3.titlepro247.com/`.

## Read / search endpoints (free, no order placed)

| Method | Path | Purpose | Body / params |
|--------|------|---------|---------------|
| GET  | `Areas/PDV/api/PDVData/` | Address → parcel rows (APN, owner, use, lat/long, FIPS) | `keyword, location, searchText, pdvSearchType=1, rows, page, …` |
| GET  | `Areas/PDV/api/PDVAPI/Autocomplete/?term=` | Address autocomplete | `term` |
| POST | `Areas/PDV/api/CompsData/PostCompsData` | Subject + nearby **sales/MLS comps** (SqFt, SalePrice, SaleDate) | criteria `{fips, apn, …}` → `{standard, distressed, listings, sales[], mls[], fips, apn, mapurl}`. **VERIFIED (#133): returns empty `sales[]`/`mls[]` with `PID:0`/`orderid:null` for every body shape (bare, +BillingTypeID, +radius/monthsBack, +GetFilter criteria). The parcel has no order context (PID 0); comps are served only against a real order minted by the blocked order endpoints, so comps are not retrievable on the read-only surface for this account tier.** |
| POST | `Areas/PDV/api/HistoryData/PostHistoryData/` | Step 1 of history: initiate a transfer/history search, returns a session key | `{SearchType, Keyword, Location, State, FIPS, City, Zip}` → `{success, key?}`. **The `{fips, apn}`→body mapping and the returned key field name are unconfirmed (needs live HAR capture).** |
| GET  | `Areas/PDV/api/HistoryData/GetHistoryData/{key}` | Step 2 of history: fetch results for the key from step 1 | path key. The typed `GetHistory` helper chains both steps in one call so the key can't expire between invocations. |
| POST | `Areas/PDV/api/PDVAPI/SearchLienAlert` | Lien alert status | **VERIFIED (#135): POST (GET → 405).** `{fips, apn}` → `{"Alerts":"1","IsTPUser":true,"Status":"1"}` — an alert count/flag, not itemized liens. The typed `GetLiens` helper wraps this. |
| POST | `Areas/PDV/api/PDVAPI/GetUserInfo` | Current user info | **POST (GET → 405).** |
| GET  | `Areas/PDV/api/PDVAPI/GetZoneList` | Zoning list | — |
| POST | `Areas/PDV/api/PDVAPI/GetFilter` | Saved comps filter | **POST (GET → 405).** Returns an empty body for this account. |
| GET  | `Areas/PDV/api/PDVAPI/GetResultsByShape` | Map-shape parcel results | query |
| GET  | `Areas/PDV/api/PDVAPI/GetCoverSheet/{id}` | Report cover sheet | path id |
| GET  | `Areas/PDV/api/PDVAPI/GetOpenOrderDetails/{id}` | Open order details | path id |
| GET  | `Areas/Lists/api/ListsData/ReadNeighborhoodReport` | Neighborhood report | query |
| GET  | `Areas/Lists/api/ListsData/ReadStats` | Neighborhood stats | query |
| GET  | `Areas/Lists/api/ListsData/SearchLocations` | Location search | query |
| GET  | `Areas/Lists/api/ListsData/GetPins` | Map pins | query |
| GET  | `Areas/Orders/api/OrdersData/GetStatus/{id}` | Order status | path id |
| GET  | `Areas/DocumentRetrieval/api/DocumentData/GetAvailableDocuments` | Available title docs | query |
| GET  | `PDV/Home/StandardizeAddress` | Normalize an address | **GET + querystring** `?address=...&lastline=...` (NOT POST + body — a POST with a JSON body returns HTTP 200 with an empty body, #134). Use the typed `StandardizeAddress` helper. |
| GET  | `PDV/Home/GetUserProducts` | Products the account can order | — |

## Typed convenience methods (client + MCP tools)

Rather than hand-crafting paths/bodies through `CallPDVAPI` / `titlepro247_pdv_api`,
prefer the typed helpers, which validate inputs and chain multi-step flows:

| Method / MCP tool | Wraps | Notes |
|---|---|---|
| `StandardizeAddress(ctx, address, lastline)` / `titlepro247_standardize_address` | `GET PDV/Home/StandardizeAddress` | GET + querystring (fixes #134). |
| `GetComps(ctx, fips, apn)` / `titlepro247_get_comps` | `POST GetFilter` → `POST PostCompsData` | Issues the correct calls; returns empty read-only (verified order-context gate, #133). |
| `GetHistory(ctx, fips, apn)` / `titlepro247_get_history` | `POST PostHistoryData` → `GET GetHistoryData/{key}` | Chains both steps in one call (#135). |
| `GetLiens(ctx, fips, apn)` / `titlepro247_get_liens` | `POST SearchLienAlert` | Returns lien-alert count/flag `{Alerts,Status}` (verified, #135). |

`CallPDVAPI`/`PDVAPIResult` now set `empty_body: true` and `raw: "(empty body)"`
when the server returns an empty body, so a silent empty 200 is no longer
dropped by `omitempty` (#134).

Items marked "needs live HAR capture" above cannot be finalized without live
TitlePro247 credentials and a browser network capture of the real request.

## Mutating / order / billing endpoints (BLOCKED by the client)

These place paid orders, modify cart/state, or upload/delete — the MCP tool
refuses them so an agent can't spend money or corrupt state:

`Areas/PDV/api/PDVAPI/OrderDupCheck`, `SubmitBatch`, `SubmitComp`,
`SubmitOfflineDocumentRequest`, `UpdateCartItemCount`, `UpdateCoverSheet`,
`UpdateDupOrder`, `UpdateFilter`, `UpdateMapType`, `DeleteImage`,
`CompleteComparablesReport`, `FavoriteReports`, `Areas/PDV/api/PDVData/UploadImage`.

## Data tiers (important)

- **Free, immediate:** parcel + owner (`PDVData`), sales/MLS comps
  (`CompsData`), liens, neighborhood stats.
- **Paid (ordered report):** the full subject **Property Detail Report**
  (beds/baths/sqft, year built, full sale/mortgage history as a PDF) is a
  premium product ordered via the cart/`SubmitBatch` flow — NOT auto-fetched.
  Use MLS (themls) for free characteristics/history when available.
