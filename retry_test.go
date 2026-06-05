package titlepro247

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestTransientTimeoutRetriesThenSucceeds verifies an idempotent GET that hits
// a transport timeout on its first attempt is retried and succeeds.
func TestTransientTimeoutRetriesThenSucceeds(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&n, 1) == 1 {
			time.Sleep(200 * time.Millisecond) // outlast the client timeout once
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c, err := New(Auth{AuthCookie: "x"},
		withBaseURL(srv.URL),
		WithTimeout(40*time.Millisecond),
		WithRetry(3, time.Millisecond),
		WithMinRequestGap(0),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	raw, _, err := c.getBytes(context.Background(), "/Account", nil)
	if err != nil {
		t.Fatalf("getBytes: %v", err)
	}
	if string(raw) != "ok" {
		t.Fatalf("body = %q, want %q", raw, "ok")
	}
	if got := atomic.LoadInt32(&n); got < 2 {
		t.Fatalf("attempts = %d, want >= 2 (a retry occurred)", got)
	}
}

// TestAuthErrorDoesNotRetry verifies a 4xx/auth response is not retried: the
// upstream is hit exactly once.
func TestAuthErrorDoesNotRetry(t *testing.T) {
	var n int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&n, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c, err := New(Auth{AuthCookie: "x"},
		withBaseURL(srv.URL),
		WithRetry(3, time.Millisecond),
		WithMinRequestGap(0),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, _, err = c.getBytes(context.Background(), "/Account", nil)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if got := atomic.LoadInt32(&n); got != 1 {
		t.Fatalf("attempts = %d, want exactly 1 (no retry on auth)", got)
	}
}

// TestAggregateCapReturnsPromptly verifies WithMaxTotalWait bounds the total
// elapsed time when the upstream always times out, rather than letting the
// per-attempt timeouts stack across the GetMe → relogin → Login retries.
func TestAggregateCapReturnsPromptly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // never answers within the cap
	}))
	defer srv.Close()

	c, err := New(Auth{Username: "u", Password: "p"},
		withBaseURL(srv.URL),
		WithMaxTotalWait(150*time.Millisecond),
		WithRetry(3, time.Millisecond),
		WithMinRequestGap(0),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	start := time.Now()
	_, err = c.GetMe(context.Background())
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("GetMe: want error (upstream always times out), got nil")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("GetMe took %v; aggregate cap (150ms) did not bound the layered retries", elapsed)
	}
}

// TestBuildErrorNotTransient verifies a request-build failure is classified as
// a permanent ErrInvalidParams, not the transient ErrRequestFailed that would
// burn the retry budget.
func TestBuildErrorNotTransient(t *testing.T) {
	c, err := New(Auth{AuthCookie: "x"}, WithMinRequestGap(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// An invalid method makes http.NewRequestWithContext fail.
	_, _, err = c.doRequest(context.Background(), "BAD METHOD", c.base()+"/Account", nil, "")
	if !errors.Is(err, ErrInvalidParams) {
		t.Fatalf("err = %v, want ErrInvalidParams", err)
	}
	if errors.Is(err, ErrRequestFailed) {
		t.Fatalf("build error wrongly classified as transient ErrRequestFailed: %v", err)
	}
}
