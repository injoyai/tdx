package tdx

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

type metadataClient struct {
	Client
	closes atomic.Int32
	days   protocol.Klines
}

func (c *metadataClient) Close() error                                    { c.closes.Add(1); return nil }
func (c *metadataClient) GetCodeAll(protocol.Exchange) (*CodeResp, error) { return &CodeResp{}, nil }
func (c *metadataClient) GetGbbqAll() (map[string][]*protocol.Gbbq, error) {
	return map[string][]*protocol.Gbbq{}, nil
}
func (c *metadataClient) GetKlineDayAll(string) (*KlineResp, error) {
	r := &KlineResp{Instrument: protocol.Instrument{Exchange: protocol.ExchangeSH, Code: "000001"}}
	for _, k := range c.days {
		r.List = append(r.List, Kline{Standard: k})
	}
	r.Count = len(r.List)
	return r, nil
}

func TestMetadataCloseOwnership(t *testing.T) {
	for _, kind := range []string{"codes", "gbbq", "workday"} {
		for _, ownClient := range []bool{false, true} {
			for _, ownDB := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/client=%t/db=%t", kind, ownClient, ownDB), func(t *testing.T) {
					db, err := xorms.NewSqlite(":memory:")
					if err != nil {
						t.Fatal(err)
					}
					defer db.Close()
					c := &metadataClient{}
					var object interface {
						Update() error
						Close() error
					}
					switch kind {
					case "codes":
						cop, dop := WithCodesClient(c), WithCodesDB(db)
						if ownClient {
							cop = WithCodesDialClient(func() (Client, error) { return c, nil })
						}
						if ownDB {
							dop = WithCodesDialDB(func() (*xorms.Engine, error) { return db, nil })
						}
						object, err = NewCodes(cop, dop, WithCodesSpec("@every 87600h"))
					case "gbbq":
						cop, dop := WithGbbqClient(c), WithGbbqDB(db)
						if ownClient {
							cop = WithGbbqDialClient(func() (Client, error) { return c, nil })
						}
						if ownDB {
							dop = WithGbbqDialDB(func() (*xorms.Engine, error) { return db, nil })
						}
						object, err = NewGbbq(cop, dop, WithGbbqSpec("@every 87600h"))
					case "workday":
						cop, dop := WithWorkdayClient(c), WithWorkdayDB(db)
						if ownClient {
							cop = WithWorkdayDialClient(func() (Client, error) { return c, nil })
						}
						if ownDB {
							dop = WithWorkdayDialDB(func() (*xorms.Engine, error) { return db, nil })
						}
						object, err = NewWorkday(cop, dop, WithWorkdaySpec("@every 87600h"))
					}
					if err != nil {
						t.Fatal(err)
					}
					defer object.Close()
					var wg sync.WaitGroup
					for i := 0; i < 4; i++ {
						wg.Go(func() {
							if err := object.Close(); err != nil {
								t.Error(err)
							}
						})
					}
					wg.Wait()
					if err := object.Update(); !errors.Is(err, net.ErrClosed) {
						t.Fatal("updated after close", err)
					}
					want := int32(0)
					if ownClient {
						want = 1
					}
					if c.closes.Load() != want {
						t.Fatal("client ownership", c.closes.Load(), want)
					}
					if err := db.Ping(); (err != nil) != ownDB {
						t.Fatal("db ownership", err)
					}
				})
			}
		}
	}
}

func TestWorkdayPublishesOnlyPersistedDates(t *testing.T) {
	db, err := xorms.NewSqlite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := &metadataClient{}
	w, err := NewWorkday(WithWorkdayDB(db), WithWorkdayClient(c), WithWorkdaySpec("@every 87600h"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	day := time.Date(2026, 1, 5, 15, 0, 0, 0, time.Local)
	c.days = protocol.Klines{{Time: day}, {Time: day.AddDate(0, 0, 1)}}
	_, err = db.Exec("CREATE TRIGGER reject_workday BEFORE INSERT ON workday BEGIN SELECT RAISE(ABORT, 'test write failure'); END")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Update(); err == nil {
		t.Fatal("expected write failure")
	}
	for _, k := range c.days {
		if w.Is(k.Time) {
			t.Fatal("uncommitted day visible", k.Time)
		}
	}
	_, err = db.Exec("DROP TRIGGER reject_workday")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Update(); err != nil {
		t.Fatal(err)
	}
	for _, k := range c.days {
		if !w.Is(k.Time) {
			t.Fatal("committed day absent", k.Time)
		}
	}
	n, err := db.Count(new(WorkdayModel))
	if err != nil || n != 2 {
		t.Fatal("persisted dates", n, err)
	}
}

func TestMetadataCloseWaitsForRefresh(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var cleanup atomic.Int32
	u := &updates{cleanup: func() error { cleanup.Add(1); return nil }}
	defer func() { unblock(); u.Close() }()
	refreshed := make(chan error, 1)
	go func() { refreshed <- u.run(func() error { close(entered); <-release; return nil }) }()
	<-entered
	closed := make(chan struct{})
	go func() { u.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("closed resources during refresh")
	case <-time.After(20 * time.Millisecond):
	}
	if cleanup.Load() != 0 {
		t.Fatal("cleanup ran during refresh")
	}
	unblock()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("close did not finish")
	}
	if err := <-refreshed; err != nil {
		t.Fatal(err)
	}
	if cleanup.Load() != 1 {
		t.Fatal("cleanup count", cleanup.Load())
	}
	if err := u.run(func() error { t.Error("started refresh after close"); return nil }); !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
}
