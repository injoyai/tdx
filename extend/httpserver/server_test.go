package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2"
)

func TestNewFailureClosesPool(t *testing.T) {
	previous := tdx.DefaultCodes
	t.Cleanup(func() { tdx.DefaultCodes = previous })
	for _, stage := range []string{"codes", "extended"} {
		t.Run(stage, func(t *testing.T) {
			tdx.DefaultCodes = nil
			if stage == "extended" {
				tdx.DefaultCodes = tdx.NewCodesBase()
			}
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { listener.Close() })
			closed := make(chan struct{})
			go func() {
				defer close(closed)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				io.Copy(io.Discard, conn)
			}()
			failure := errors.New("initialization failure")
			var created atomic.Int32
			op := []Option{
				WithHosts(listener.Addr().String()),
				WithOptions(tdx.WithDebug(false), func(c *client.Client) {
					if created.Add(1) == 2 {
						c.SetDial(func(context.Context) (ios.ReadWriteCloser, string, error) {
							return nil, "", failure
						})
					}
				}),
			}
			if stage == "extended" {
				op = append(op, WithExHqHosts(listener.Addr().String()))
			}
			if s, err := New(op...); s != nil || !errors.Is(err, failure) {
				t.Fatal("expected initialization failure", s, err)
			}
			select {
			case <-closed:
			case <-time.After(2 * time.Second):
				t.Fatal("initialization failure leaked pool connection")
			}
		})
	}
}

func TestCloseReleasesPools(t *testing.T) {
	var closed atomic.Int32
	newPool := func() *tdx.Pool {
		p, err := tdx.NewPool(func() (*tdx.Conn, error) {
			c := client.New(nil)
			c.Closer.Reset()
			c.SetCloseFunc(func(error) error { closed.Add(1); return nil })
			return &tdx.Conn{Client: c}, nil
		}, 2)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	s := &Server{pool: newPool(), exPool: newPool(), server: &http.Server{}}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if closed.Load() != 4 {
		t.Fatalf("closed %d connections, want 4", closed.Load())
	}
	if err := s.Close(); err != nil || closed.Load() != 4 {
		t.Fatal("repeated close changed result", closed.Load(), err)
	}
}

func TestRespondOK(t *testing.T) {
	w := httptest.NewRecorder()
	respondOK(w, map[string]any{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 0 {
		t.Errorf("expected code 0, got %d", resp.Code)
	}
	if resp.Msg != "ok" {
		t.Errorf("expected msg 'ok', got '%s'", resp.Msg)
	}
}

func TestRespondErr(t *testing.T) {
	w := httptest.NewRecorder()
	respondErr(w, http.StatusBadRequest, "bad request")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 1 {
		t.Errorf("expected code 1, got %d", resp.Code)
	}
	if resp.Msg != "bad request" {
		t.Errorf("expected msg 'bad request', got '%s'", resp.Msg)
	}
}

func TestParseExchange(t *testing.T) {
	cases := []struct {
		in   string
		want uint8
		err  bool
	}{
		{"sh", 1, false},
		{"sz", 0, false},
		{"bj", 2, false},
		{"SH", 1, false},
		{"xx", 0, true},
	}
	for _, c := range cases {
		ex, err := parseExchange(c.in)
		if c.err {
			if err == nil {
				t.Errorf("parseExchange(%q) expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseExchange(%q) unexpected error: %v", c.in, err)
		}
		if ex.Uint8() != c.want {
			t.Errorf("parseExchange(%q) = %d, want %d", c.in, ex.Uint8(), c.want)
		}
	}
}

func TestQueryUint16Default(t *testing.T) {
	req := httptest.NewRequest("GET", "/?count=100", nil)
	if got := queryUint16Default(req, "count", 50); got != 100 {
		t.Errorf("expected 100, got %d", got)
	}
	req2 := httptest.NewRequest("GET", "/", nil)
	if got := queryUint16Default(req2, "count", 50); got != 50 {
		t.Errorf("expected default 50, got %d", got)
	}
}
