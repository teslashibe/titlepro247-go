package titlepro247

import (
	"errors"
	"fmt"
)

// Sentinel errors.
var (
	ErrInvalidAuth   = errors.New("titlepro247: missing or invalid auth credentials")
	ErrUnauthorized  = errors.New("titlepro247: unauthorized (session expired)")
	ErrForbidden     = errors.New("titlepro247: forbidden")
	ErrNotFound      = errors.New("titlepro247: not found")
	ErrRateLimited   = errors.New("titlepro247: rate limited")
	ErrInvalidParams = errors.New("titlepro247: invalid parameters")
	ErrRequestFailed = errors.New("titlepro247: request failed")
	ErrLoginFailed   = errors.New("titlepro247: login failed (bad username/password)")
)

// HTTPError is returned for unexpected non-2xx HTTP responses.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("titlepro247: HTTP %d: %s", e.StatusCode, e.Body)
}
