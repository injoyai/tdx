package spider

import (
	"context"
	"errors"
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
		func() error { _, err := GetTHSDayKline("sz000001", THSBFQ); return err },
		func() error { _, err := GetSinaFactor("sz000001", SinaQFQ); return err },
	} {
		if err := call(); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("request ignored timeout", err)
		}
	}
}

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
			got, err := GetTHSDayKline("sz000001", THSBFQ)
			if (err == nil) != tc.ok {
				t.Fatal(got, err)
			}
			if tc.ok && (len(got) != 1 || got[0].Open != 10.01 || got[0].High != 10.02 || got[0].Low != 10 || got[0].Close != 10.03 || got[0].Volume != 1 || got[0].Time.Format("20060102") != "20260105") {
				t.Fatal("valid payload changed", got)
			}
		})
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
			got, err := GetSinaFactor("sz000001", SinaQFQ)
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
	got, err := GetSinaFactor("sz000001", SinaHFQ)
	if err != nil || len(got) != 1 || got[0].HFactor != 2.5 || got[0].QFactor != 0 {
		t.Fatal(got, err)
	}
}
