package titlepro247

import (
	"testing"
	"time"
)

func TestDefaultTimeout(t *testing.T) {
	c, err := New(Auth{Username: "u", Password: "p"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := c.httpClient.Timeout; got != defaultTimeout {
		t.Fatalf("default timeout = %v, want %v", got, defaultTimeout)
	}
}

func TestWithTimeout(t *testing.T) {
	want := 90 * time.Second
	c, err := New(Auth{Username: "u", Password: "p"}, WithTimeout(want))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := c.httpClient.Timeout; got != want {
		t.Fatalf("timeout = %v, want %v", got, want)
	}
}

func TestWithTimeoutNonPositiveIgnored(t *testing.T) {
	c, err := New(Auth{Username: "u", Password: "p"}, WithTimeout(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := c.httpClient.Timeout; got != defaultTimeout {
		t.Fatalf("timeout = %v, want default %v", got, defaultTimeout)
	}
}
