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
| POST | `Areas/PDV/api/CompsData/PostCompsData` | Subject + nearby **sales/MLS comps** (SqFt, SalePrice, SaleDate) | criteria `{fips, apn, BillingTypeID, …filters}` → `{standard, distressed, listings, sales[], mls[], fips, apn, mapurl}` |
| POST | `Areas/PDV/api/HistoryData/PostHistoryData/` | Initiate a transfer/history search | `{SearchType, Keyword, Location, State, FIPS, City, Zip}` → `{success}` |
| GET  | `Areas/PDV/api/HistoryData/GetHistoryData/{key}` | Retrieve search history options | path key |
| GET  | `Areas/PDV/api/PDVAPI/SearchLienAlert` | Lien alert search | query |
| GET  | `Areas/PDV/api/PDVAPI/GetUserInfo` | Current user info | — |
| GET  | `Areas/PDV/api/PDVAPI/GetZoneList` | Zoning list | — |
| GET  | `Areas/PDV/api/PDVAPI/GetFilter` | Saved comps filter | — |
| GET  | `Areas/PDV/api/PDVAPI/GetResultsByShape` | Map-shape parcel results | query |
| GET  | `Areas/PDV/api/PDVAPI/GetCoverSheet/{id}` | Report cover sheet | path id |
| GET  | `Areas/PDV/api/PDVAPI/GetOpenOrderDetails/{id}` | Open order details | path id |
| GET  | `Areas/Lists/api/ListsData/ReadNeighborhoodReport` | Neighborhood report | query |
| GET  | `Areas/Lists/api/ListsData/ReadStats` | Neighborhood stats | query |
| GET  | `Areas/Lists/api/ListsData/SearchLocations` | Location search | query |
| GET  | `Areas/Lists/api/ListsData/GetPins` | Map pins | query |
| GET  | `Areas/Orders/api/OrdersData/GetStatus/{id}` | Order status | path id |
| GET  | `Areas/DocumentRetrieval/api/DocumentData/GetAvailableDocuments` | Available title docs | query |
| GET  | `PDV/Home/StandardizeAddress` | Normalize an address | `address, lastline` |
| GET  | `PDV/Home/GetUserProducts` | Products the account can order | — |

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
