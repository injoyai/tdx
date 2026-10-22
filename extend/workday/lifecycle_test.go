package workday

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

type metadataClient struct {
	tdx.Client
	closes atomic.Int32
	days   protocol.Klines
}

func (c *metadataClient) Close() error { c.closes.Add(1); return nil }
func (*metadataClient) GetCodeAll(protocol.Exchange) (*tdx.CodeResp, error) {
	return &tdx.CodeResp{}, nil
}
func (*metadataClient) GetGbbqAll() (map[string][]*protocol.Gbbq, error) {
	return map[string][]*protocol.Gbbq{}, nil
}
func (c *metadataClient) GetKlineDayAll(string) (*tdx.KlineResp, error) {
	r := &tdx.KlineResp{Instrument: protocol.Instrument{Exchange: protocol.ExchangeSH, Code: "000001"}}
	for _, k := range c.days {
		r.List = append(r.List, tdx.Kline{Standard: k})
	}
	r.Count = len(r.List)
	return r, nil
}

func TestWorkdayPublishesOnlyPersistedDates(t *testing.T) {
	db, err := xorms.NewSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := &metadataClient{}
	w, err := New(WithDB(db), WithClient(c), WithSpec("@every 87600h"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	day := time.Date(2026, 1, 5, 15, 0, 0, 0, protocol.Location())
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
		if !w.Is(k.Time) || !w.Is(k.Time.UTC()) {
			t.Fatal("committed day absent", k.Time)
		}
	}
	n, err := db.Count(new(WorkdayModel))
	if err != nil || n != 2 {
		t.Fatal("persisted dates", n, err)
	}
}
