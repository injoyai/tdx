package gbbq

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

type downloadTransport func(*http.Request) (*http.Response, error)

func (t downloadTransport) RoundTrip(r *http.Request) (*http.Response, error) { return t(r) }

func TestDownloadTimeout(t *testing.T) {
	previous := downloadClient
	if previous.Timeout <= 0 {
		t.Fatal("missing download timeout")
	}
	t.Cleanup(func() { downloadClient = previous })
	downloadClient = &http.Client{Timeout: 20 * time.Millisecond, Transport: downloadTransport(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })}
	if _, err := Download(t.TempDir()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("download ignored timeout", err)
	}
}
