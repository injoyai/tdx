package httpclient

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/metadata"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

type trackedBody struct {
	io.Reader
	closed *bool
}

func (b trackedBody) Close() error { *b.closed = true; return nil }

type failedBody struct{}

func (failedBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestHTTPCacheJSONValidation(t *testing.T) {
	for _, kind := range []string{"codes", "gbbq"} {
		for _, mode := range []string{"valid", "empty", "null", "malformed", "trailing", "status", "read error"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				payload := `[{"exchange":"sz","code":"000001","name":"new"}]`
				if kind == "gbbq" {
					payload = `{"sz000001":[{"Code":"sz000001","Time":"2026-01-06T15:00:00+08:00","Category":5,"C3":300}]}`
				}
				ok := mode == "valid" || mode == "empty" || mode == "null"
				if mode == "empty" {
					payload = `[]`
					if kind == "gbbq" {
						payload = `{}`
					}
				}
				if mode == "null" {
					payload = `null`
				}
				if mode == "malformed" {
					payload = `!`
				}
				if mode == "trailing" {
					payload += ` {}`
				}
				closed := false
				transport := &http.Client{Transport: dataRoundTrip(func(*http.Request) (*http.Response, error) {
					status := 200
					if mode == "status" {
						status = 503
					}
					var reader io.Reader = strings.NewReader(payload)
					if mode == "read error" {
						reader = failedBody{}
					}
					return &http.Response{StatusCode: status, Body: trackedBody{reader, &closed}}, nil
				})}
				if kind == "codes" {
					client := New("http://offline.invalid", transport)
					c, err := metadata.NewCodes(model.CodeModels{{Exchange: "sz", Code: "000001", Name: "old"}})
					if err != nil {
						t.Fatal(err)
					}
					fresh, err := client.FetchCodes(context.Background())
					if err == nil {
						c = fresh
					}
					if (err == nil) != ok {
						t.Fatal(err)
					}
					want := "old"
					if ok {
						want = "new"
						if mode != "valid" {
							want = ""
						}
					}
					if c.GetName("sz000001") != want {
						t.Fatal("cache replacement contract changed", c.GetName("sz000001"), want)
					}
				} else {
					at := time.Date(2026, 1, 6, 15, 0, 0, 0, protocol.Location())
					snapshot, err := metadata.NewGbbq([]*protocol.Gbbq{{Code: "sz000001", Time: at, Category: 5, C3: 100}})
					if err != nil {
						t.Fatal(err)
					}
					client := New("http://offline.invalid", transport)
					c := snapshot
					fresh, fetchErr := client.FetchGbbq(context.Background())
					err = fetchErr
					if err == nil {
						c = fresh
					}
					if (err == nil) != ok {
						t.Fatal(err)
					}
					want := int64(100)
					if ok {
						want = 0
						if mode == "valid" {
							want = 300
						}
					}
					eq := c.GetEquity("sz000001", at)
					if (want == 0 && eq != nil) || (want != 0 && (eq == nil || eq.Float != want)) {
						t.Fatal("cache replacement contract changed", eq, want)
					}
				}
				if !closed {
					t.Fatal("response body leaked")
				}
			})
		}
	}
}
