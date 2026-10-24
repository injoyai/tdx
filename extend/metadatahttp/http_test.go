package metadatahttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type dataRoundTrip func(*http.Request) (*http.Response, error)

func (f dataRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDataRequestsHaveTimeout(t *testing.T) {
	previous := dataHTTPClient
	if previous.Timeout <= 0 {
		t.Fatal("missing default timeout")
	}
	t.Cleanup(func() { dataHTTPClient = previous })
	dataHTTPClient = &http.Client{Timeout: 20 * time.Millisecond, Transport: dataRoundTrip(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	for _, call := range []func() error{
		func() error { _, err := DialCodesHTTP("http://offline.invalid"); return err },
		func() error { _, err := DialGbbqHTTP("http://offline.invalid"); return err },
	} {
		if err := call(); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("request ignored timeout", err)
		}
	}
}

func TestHTTPCacheCloseCancelsRequests(t *testing.T) {
	previous := dataHTTPClient
	t.Cleanup(func() { dataHTTPClient = previous })
	dataHTTPClient = &http.Client{Timeout: time.Second, Transport: dataRoundTrip(func(r *http.Request) (*http.Response, error) {
		if err := r.Context().Err(); err != nil {
			return nil, err
		}
		body := `[]`
		if strings.HasSuffix(r.URL.Path, "/gbbqs") {
			body = `{}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	for _, equity := range []bool{false, true} {
		t.Run(fmt.Sprint(equity), func(t *testing.T) {
			var c interface {
				Update() error
				Close() error
			}
			var err error
			if equity {
				c, err = DialGbbqHTTP("http://offline.invalid", "@every 87600h")
			} else {
				c, err = DialCodesHTTP("http://offline.invalid", "@every 87600h")
			}
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
			if err := c.Update(); !errors.Is(err, context.Canceled) {
				t.Fatal("closed cache still fetched data", err)
			}
		})
	}
}

func TestHTTPCacheCloseInterruptsActiveFetch(t *testing.T) {
	previous := dataHTTPClient
	t.Cleanup(func() { dataHTTPClient = previous })
	dataHTTPClient = &http.Client{Timeout: time.Second, Transport: dataRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`[]`)), Header: make(http.Header)}, nil
	})}
	c, err := DialCodesHTTP("http://offline.invalid", "@every 87600h")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	entered := make(chan struct{})
	dataHTTPClient = &http.Client{Timeout: time.Second, Transport: dataRoundTrip(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	fetched := make(chan error, 1)
	go func() { fetched <- c.Update() }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("fetch did not start")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-fetched:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("active request was not canceled")
	}
}
