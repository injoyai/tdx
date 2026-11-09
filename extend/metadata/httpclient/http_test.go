package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type dataRoundTrip func(*http.Request) (*http.Response, error)

func (f dataRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNewDoesNotFetchOrSchedule(t *testing.T) {
	var calls atomic.Int32
	c := New("http://offline.invalid/", &http.Client{Transport: dataRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		body := `[]`
		if r.URL.Path == "/gbbqs" {
			body = `{}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})})
	if calls.Load() != 0 {
		t.Fatal("construction accessed network")
	}
	if _, err := c.FetchCodes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.FetchGbbq(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("fetches were not explicit", calls.Load())
	}
	if New("http://offline.invalid", nil).http.Timeout != 30*time.Second {
		t.Fatal("missing default timeout")
	}
}

func TestDataRequestsHaveTimeout(t *testing.T) {
	c := New("http://offline.invalid", &http.Client{Timeout: 20 * time.Millisecond, Transport: dataRoundTrip(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})})
	for _, fetch := range []func() error{
		func() error { _, err := c.FetchCodes(context.Background()); return err },
		func() error { _, err := c.FetchGbbq(context.Background()); return err },
	} {
		if err := fetch(); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("request ignored timeout", err)
		}
	}
}

func TestFetchCancellation(t *testing.T) {
	for _, equity := range []bool{false, true} {
		entered := make(chan struct{})
		c := New("http://offline.invalid", &http.Client{Timeout: time.Second, Transport: dataRoundTrip(func(r *http.Request) (*http.Response, error) {
			close(entered)
			<-r.Context().Done()
			return nil, r.Context().Err()
		})})
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			if equity {
				_, err := c.FetchGbbq(ctx)
				done <- err
			} else {
				_, err := c.FetchCodes(ctx)
				done <- err
			}
		}()
		select {
		case <-entered:
		case <-time.After(time.Second):
			cancel()
			t.Fatal("request did not start")
		}
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("active request ignored cancellation")
		}
	}
	c := New("http://offline.invalid", nil)
	if _, err := c.FetchCodes(nil); err == nil {
		t.Fatal("accepted nil context")
	}
}
