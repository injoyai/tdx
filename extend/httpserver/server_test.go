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

type closeTestClient struct {
	tdx.Client
	closeFn func() error
}

func (c *closeTestClient) Close() error { return c.closeFn() }

func TestNewFailureClosesPool(t *testing.T) {
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
	failure := errors.New("extended initialization failure")
	var created atomic.Int32
	s, err := New(WithHosts(listener.Addr().String()), WithExHosts(listener.Addr().String()),
		WithClientOptions(tdx.WithRedial(false), tdx.WithDebug(false), func(c *client.Client) {
			if created.Add(1) == 2 {
				c.SetDial(func(context.Context) (ios.ReadWriteCloser, string, error) {
					return nil, "", failure
				})
			}
		}))
	if s != nil || !errors.Is(err, failure) {
		t.Fatal("expected initialization failure", s, err)
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("initialization failure leaked pool connection")
	}
}

func TestNewOnlyCreatesConfiguredConnections(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(io.Discard, conn)
	}()
	var created atomic.Int32
	s, err := New(WithHosts(listener.Addr().String()), WithClientOptions(tdx.WithRedial(false), tdx.WithDebug(false),
		func(*client.Client) { created.Add(1) }))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if created.Load() != 1 {
		t.Fatal("created an unsolicited metadata client", created.Load())
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pool connection leaked")
	}
}

func TestCloseReleasesPools(t *testing.T) {
	var closed atomic.Int32
	newPool := func() *tdx.Pool {
		p, err := tdx.NewPool(func() (tdx.Client, error) {
			c := client.New(nil)
			c.Closer.Reset()
			c.SetCloseFunc(func(error) error { closed.Add(1); return nil })
			return &closeTestClient{closeFn: c.Close}, nil
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
