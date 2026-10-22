package httpserver

import (
	"context"
	"github.com/injoyai/tdx/v2"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCanceledRequestsDoNotQueueOrFetch(t *testing.T) {
	for _, path := range []string{"/code/stocks", "/quote?codes=sz000001", "/ex/count"} {
		t.Run(path, func(t *testing.T) {
			c := &dedupClient{}
			p := handlerPool(t, c)
			held, err := p.Get()
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if held != nil {
					p.Put(held)
				}
			}()
			mux := http.NewServeMux()
			(&Server{pool: p, exPool: p}).registerRoutes(mux)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			req := httptest.NewRequest("GET", path, nil).WithContext(ctx)
			done := make(chan struct{})
			go func() { defer close(done); mux.ServeHTTP(httptest.NewRecorder(), req) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				p.Close()
				<-done
				t.Fatal("canceled request remained queued")
			}
			if c.called != "" {
				t.Fatal("canceled request fetched", c.called)
			}
			// 等待取消不能使连接丢失。
			p.Put(held)
			held = nil
			if err := p.Do(func(tdx.Client) error { return nil }); err != nil {
				t.Fatal(err)
			}
		})
	}
}
