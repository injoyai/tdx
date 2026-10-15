package extend

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

func TestTHSResponseValidation(t *testing.T) {
	previous := dataHTTPClient
	t.Cleanup(func() { dataHTTPClient = previous })
	valid := `callback({"total":1,"sortYear":[[2026,1]],"priceFactor":100,"price":"1000,1,2,3","dates":"0105","volumn":"100"});`
	for _, tc := range []struct {
		name, body string
		status     int
		ok         bool
	}{
		{"valid", valid, 200, true},
		{"empty", "", 200, false},
		{"status", valid, 503, false},
		{"no wrapper", `{}`, 200, false},
		{"unclosed", `callback({}`, 200, false},
		{"bad json", `callback(!)`, 200, false},
		{"zero divisor", strings.Replace(valid, `"priceFactor":100`, `"priceFactor":0`, 1), 200, false},
		{"bad year shape", strings.Replace(valid, `[[2026,1]]`, `[[2026]]`, 1), 200, false},
		{"year overflow", strings.Replace(valid, `[[2026,1]]`, `[[2025,2],[2026,1]]`, 1), 200, false},
		{"negative year size", strings.Replace(valid, `[[2026,1]]`, `[[2025,-1],[2026,1]]`, 1), 200, false},
		{"missing year", strings.Replace(valid, `[[2026,1]]`, `[]`, 1), 200, false},
		{"bad count", strings.Replace(valid, `"total":1`, `"total":3`, 1), 200, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dataHTTPClient = &http.Client{Timeout: time.Second, Transport: dataRoundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}
			got, err := GetTHSDayKline("sz000001", THS_BFQ)
			if (err == nil) != tc.ok {
				t.Fatal(got, err)
			}
			if tc.ok && (len(got) != 1 || got[0].Open != 10010 || got[0].High != 10020 || got[0].Low != 10000 || got[0].Close != 10030 || got[0].Volume != 1 || got[0].Time.Format("20060102") != "20260105") {
				t.Fatal("valid payload changed", got)
			}
		})
	}
}

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
		func() error { _, err := GetTHSDayKline("sz000001", THS_BFQ); return err },
		func() error { _, err := GetSinaFactor("sz000001", Sina_QFQ); return err },
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

func TestSinaFactorValidation(t *testing.T) {
	previous := dataHTTPClient
	t.Cleanup(func() { dataHTTPClient = previous })
	for _, tc := range []struct {
		name, body string
		ok         bool
	}{
		{"valid", `{"data":[{"d":"2026-01-05","f":"1.23456789"}]}`, true},
		{"wrapped", `var sz000001qfq={"data":[{"d":"2026-01-05","f":"1.23456789"}]}/* comment */`, true},
		{"null", `{"data":[null]}`, false},
		{"broken", `{"data":[{"d":"2026-01-05","f":"broken"}]}`, false},
		{"nan", `{"data":[{"d":"2026-01-05","f":"NaN"}]}`, false},
		{"infinity", `{"data":[{"d":"2026-01-05","f":"+Inf"}]}`, false},
		{"overflow", `{"data":[{"d":"2026-01-05","f":"1e999"}]}`, false},
		{"invalid date", `{"data":[{"d":"2026-02-30","f":"1"}]}`, false},
		{"invalid json", `!`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dataHTTPClient = &http.Client{Transport: dataRoundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			got, err := GetSinaFactor("sz000001", Sina_QFQ)
			if (err == nil) != tc.ok {
				t.Fatal(got, err)
			}
			if tc.ok && (len(got) != 1 || got[0].QFactor != 1.23456789 || got[0].HFactor != 0 || got[0].Date != time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC).Unix()) {
				t.Fatal("valid sample changed", got)
			}
		})
	}
	dataHTTPClient = &http.Client{Transport: dataRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"d":"2026-01-05","f":"2.5"}]}`))}, nil
	})}
	got, err := GetSinaFactor("sz000001", Sina_HFQ)
	if err != nil || len(got) != 1 || got[0].HFactor != 2.5 || got[0].QFactor != 0 {
		t.Fatal(got, err)
	}
}
