package titlepro247

// Auth carries the credentials needed to talk to v3.titlepro247.com.
//
// On a successful Login the server sets a long-lived first-party
// .SiteXPro_AUTH cookie. The cookie value is opaque (looks like an
// encrypted blob) and we cache it on AuthCookie.
type Auth struct {
	Username string
	Password string

	// AuthCookie is the value of the .SiteXPro_AUTH cookie issued by
	// v3.titlepro247.com on a successful login. Populated automatically
	// after Login(); supply it manually to skip the bootstrap.
	AuthCookie string
}

// User is the bare-minimum profile surfaced by the dashboard. The
// TitlePro247 site has no JSON /me endpoint, so GetMe parses HTML.
type User struct {
	DisplayName string `json:"display_name,omitempty"`
	LoggedIn    bool   `json:"logged_in"`
}

// AccountPage is the parsed envelope from /Account.
type AccountPage struct {
	URL          string `json:"url"`
	StatusCode   int    `json:"status_code"`
	ContentBytes int    `json:"content_bytes"`
	Title        string `json:"title,omitempty"`
}
