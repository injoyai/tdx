package httpclient

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFetchCodesRejectsInvalidSnapshot(t *testing.T) {
	for _, payload := range []string{
		`[null]`,
		`[{"exchange":"SZ","code":"000001"}]`,
		`[{"exchange":"sz","code":"sz000001"}]`,
		`[{"exchange":"bad","code":"?"}]`,
		`[{"exchange":"hk","code":"00700"}]`,
		`[{"exchange":"sz","code":"000001"},{"exchange":"sz","code":"000001"}]`,
		`[{"exchange":"sh","code":"600000"},{"exchange":"sz","code":""}]`,
		`[{"exchange":"sh","code":"600000","name":"` + strings.Repeat("名", 129) + `"}]`,
	} {
		t.Run(payload, func(t *testing.T) {
			closed := false
			c := New("http://offline.invalid", &http.Client{Transport: dataRoundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: trackedBody{Reader: strings.NewReader(payload), closed: &closed}}, nil
			})})
			if got, err := c.FetchCodes(context.Background()); err == nil || got != nil {
				t.Fatal("published invalid or partial snapshot", got, err)
			}
			if !closed {
				t.Fatal("body not closed")
			}
		})
	}
}

func TestFetchCodesKeepsDistinctExchangesAndAllCategories(t *testing.T) {
	payload := `[{"exchange":"sh","code":"000001"},{"exchange":"sz","code":"000001"},{"exchange":"sh","code":"113001"},{"exchange":"sh","code":"019001"}]`
	c := New("http://offline.invalid", &http.Client{Transport: dataRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload))}, nil
	})})
	got, err := c.FetchCodes(context.Background())
	if err != nil || len(got.Snapshot()) != 4 {
		t.Fatal("valid snapshot rejected", got, err)
	}
	for _, code := range []string{"sh000001", "sz000001", "sh113001", "sh019001"} {
		if got.Get(code) == nil {
			t.Fatal("missing security", code)
		}
	}
}
