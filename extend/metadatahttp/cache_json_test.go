package metadatahttp

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/injoyai/tdx/v2/extend/codes"
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
	previous := dataHTTPClient
	t.Cleanup(func() { dataHTTPClient = previous })
	for _, kind := range []string{"codes", "gbbq"} {
		for _, mode := range []string{"valid", "empty", "null", "malformed", "trailing", "status", "read error"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				payload := `[{"exchange":"sz","code":"000001","name":"new"}]`
				if kind == "gbbq" {
					payload = `{"sz000001":[{"Category":5,"C3":300}]}`
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
				dataHTTPClient = &http.Client{Transport: dataRoundTrip(func(*http.Request) (*http.Response, error) {
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
					c := &CodesHTTP{Base: codes.NewBase(), address: "http://offline.invalid", httpUpdates: newHTTPUpdates()}
					defer c.Close()
					c.Base.Update(codes.CodeModels{{Exchange: "sz", Code: "000001", Name: "old"}})
					err := c.Update()
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
					c := &GbbqHTTP{address: "http://offline.invalid", httpUpdates: newHTTPUpdates(), m: map[string][]*protocol.Gbbq{"old": {{Category: 5}}}}
					defer c.Close()
					err := c.Update()
					if (err == nil) != ok {
						t.Fatal(err)
					}
					if !ok && len(c.m["old"]) != 1 {
						t.Fatal("failed response replaced cache")
					}
					if ok {
						want := 0
						if mode == "valid" {
							want = 1
						}
						if len(c.m) != want {
							t.Fatal(c.m)
						}
					}
				}
				if !closed {
					t.Fatal("response body leaked")
				}
			})
		}
	}
}
