package httpserver

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2"
)

func TestServerLive(t *testing.T) {
	if os.Getenv("TDX_HTTP_LIVE") != "1" {
		t.Skip("set TDX_HTTP_LIVE=1 to run rate-limited HTTP test with real upstream")
	}
	host := os.Getenv("TDX_STD_HOST")
	if host == "" {
		host = tdx.Hosts[0]
	}
	var mu sync.Mutex
	var next time.Time
	closed := make(chan struct{})
	s, err := New(WithHosts(host), WithPoolSize(1), WithClientOptions(
		tdx.WithRedial(false), tdx.WithDebug(false),
		client.WithWriteWith(func(bs []byte) ([]byte, error) {
			mu.Lock()
			defer mu.Unlock()
			if wait := time.Until(next); wait > 0 {
				time.Sleep(wait)
			}
			next = time.Now().Add(time.Second)
			return bs, nil
		}),
		client.WithDisconnect(func(*client.Client, error) { close(closed) }),
	))
	if err != nil {
		t.Fatal("HTTP real upstream initialization", err)
	}
	t.Cleanup(func() { s.Close() })
	w := httptest.NewRecorder()
	s.server.Handler.ServeHTTP(w, httptest.NewRequest("GET", "/count?exchange=sz", nil))
	var result struct {
		Code int
		Msg  string
		Data struct{ Count uint16 }
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Code != 0 || result.Data.Count == 0 {
		t.Fatal("HTTP real data response", w.Body.String(), err)
	}
	t.Logf("server=%s concurrency=1 write_interval=1s SZ codes=%d", host, result.Data.Count)
	if err := s.Close(); err != nil {
		t.Fatal("HTTP Close", err)
	}
	select {
	case <-closed:
	default:
		t.Fatal("HTTP Close did not release real upstream connection")
	}
	t.Log("HTTP Close released real upstream connection")
}
