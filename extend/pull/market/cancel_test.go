package market

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

type waitingPool struct {
	*tdx.Pool
	entered chan struct{}
	once    sync.Once
}

type cancelAfterFetchClient struct {
	*unifiedClient
	cancel context.CancelFunc
}

func (c *cancelAfterFetchClient) GetKlineUntil(category uint8, code string, stop func(tdx.Kline) bool) (*tdx.KlineResp, error) {
	rows, err := c.unifiedClient.GetKlineUntil(category, code, stop)
	c.cancel()
	return rows, err
}

func (c *cancelAfterFetchClient) GetKlineMinute241Until(code string, stop func(tdx.Kline) bool) (*tdx.KlineResp, error) {
	return c.GetKlineUntil(protocol.TypeKlineMinute, code, stop)
}

func TestCanceledFetchDoesNotSaveBarsOrCompletion(t *testing.T) {
	for _, minute := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		c := &cancelAfterFetchClient{unifiedClient: &unifiedClient{}, cancel: cancel}
		pool, err := tdx.NewPool(func() (tdx.Client, error) { return c, nil }, 1)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		store := testStore(t.TempDir())
		s, err := pull.New(&pull.Config{Store: store, Codes: []string{"sz000001"}, StartAt: "20260101",
			Day: !minute, Minute: minute, Pool: pool, Retry: 1})
		if err != nil {
			cancel()
			pool.Close()
			t.Fatal(err)
		}
		err = s.UpdateContext(ctx, true)
		cancel()
		s.Close()
		pool.Close()
		if !errors.Is(err, context.Canceled) {
			t.Fatal("canceled fetch reported success", minute, err)
		}
		code := model.Code{Market: protocol.MarketAStock, Code: "sz000001"}
		days, err := store.QueryDay(code, "", "")
		if err != nil || len(days) != 0 {
			t.Fatal("canceled fetch saved days", days, err)
		}
		minutes, err := store.QueryMinute(code, "", "")
		if err != nil || len(minutes) != 0 {
			t.Fatal("canceled fetch saved minutes", minutes, err)
		}
		if len(store.Marks.Marks) != 0 {
			t.Fatal("canceled fetch marked complete")
		}
	}
}

func (p *waitingPool) DoContext(ctx context.Context, fn func(tdx.Client) error) error {
	p.once.Do(func() { close(p.entered) })
	return p.Pool.DoContext(ctx, fn)
}

func TestUpdateCancellationWhileWaitingForPool(t *testing.T) {
	for _, code := range []string{"sz000001", "usAAPL", "sh880001"} {
		t.Run(code, func(t *testing.T) {
			pool, err := tdx.NewPool(func() (tdx.Client, error) { return &unifiedClient{}, nil }, 1)
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			held, err := pool.Get()
			if err != nil {
				t.Fatal(err)
			}
			var release sync.Once
			unblock := func() { release.Do(func() { pool.Put(held) }) }
			defer unblock()
			waiting := &waitingPool{Pool: pool, entered: make(chan struct{})}
			s, err := pull.New(&pull.Config{Store: testStore(t.TempDir()), Codes: []string{code},
				Day: true, Retry: 1, Pool: waiting, ExPool: waiting})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- s.UpdateContext(ctx, true) }()
			select {
			case <-waiting.entered:
			case <-time.After(time.Second):
				t.Fatal("update did not wait for a connection")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatal("cancellation was not returned", err)
				}
			case <-time.After(100 * time.Millisecond):
				unblock()
				<-done
				t.Fatal("canceled update remained blocked waiting for a connection")
			}
		})
	}
}
