package pull

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/storage/sqlite"
	"github.com/injoyai/tdx/v2/protocol"
)

type customClient struct{ unifiedClient }

func (*customClient) GetInstruments() ([]protocol.ExInstrument, error) {
	return []protocol.ExInstrument{{Market: 252, Code: "Aa12"}, {Market: 251, Code: "Bb34"}}, nil
}

func (*customClient) GetBlockDataWithIndex(string) ([]*protocol.Block, error) {
	return nil, nil
}

func TestRegisteredMarketPull(t *testing.T) {
	const id protocol.MarketID = "testex"
	if _, known := id.Exchange(); !known {
		m, err := protocol.NewMarket(protocol.MarketDef{
			Key: id, Exchange: 252, CodePattern: "[A-Z][a-z][0-9]{2}",
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := protocol.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	client := &customClient{}
	pool := &adapterPool{c: client}
	for _, mode := range []string{"disabled", "discover", "explicit", "missing schedule"} {
		t.Run(mode, func(t *testing.T) {
			store, err := sqlite.NewPull(t.TempDir(), 3)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			cfg := &Config{
				Store: store, Pool: pool, ExPool: pool, StartAt: "20260101",
				ListCodes: func(context.Context) ([]model.Code, error) { return nil, nil },
			}
			if mode == "discover" || mode == "explicit" {
				cfg.Schedules = map[protocol.MarketID]Schedule{id: {TimeZone: "UTC", CloseTime: "15:00"}}
			}
			if mode == "explicit" || mode == "missing schedule" {
				cfg.Codes = []string{"Aa12.TESTEX"}
			}
			s, err := New(cfg)
			if mode == "missing schedule" {
				if err == nil {
					t.Fatal("missing schedule accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if mode == "explicit" {
				u, _ := Get(id.String())
				value, err := s.schedule(u)
				at, boundaryErr := value.boundary(time.Date(2026, 1, 5, 12, 0, 0, 0, time.UTC))
				if err != nil || boundaryErr != nil || !at.Equal(time.Date(2026, 1, 5, 15, 0, 0, 0, time.UTC)) {
					t.Fatal("schedule override not applied", value, at, err, boundaryErr)
				}
			}
			client.code = ""
			if err := s.Update(true); err != nil {
				t.Fatal(err)
			}
			if mode == "disabled" {
				if client.code != "" {
					t.Fatal("unconfigured market ran")
				}
				return
			}
			if client.code != "testex.Aa12" {
				t.Fatal("wrong query code", client.code)
			}
			code := model.Code{Market: id, Code: "Aa12"}
			days, err := s.QueryDay(code, "", "")
			if err != nil || len(days) != 1 || days[0].Volume != 12345 {
				t.Fatal(days, err)
			}
			minutes, err := s.QueryMinute(code, "", "")
			if err != nil || len(minutes) != 1 || minutes[0].Time != "2026-01-05 09:31" || minutes[0].Volume != 12345 {
				t.Fatal(minutes, err)
			}
		})
	}
}

func TestProtocolUnitNameConflict(t *testing.T) {
	const id protocol.MarketID = "occupiedunit"
	if _, known := id.Exchange(); !known {
		custom := &mockUnit{name: "OccupiedUnit"}
		Register(custom)
		t.Cleanup(func() {
			registry.Lock()
			delete(registry.units, custom.Name())
			registry.Unlock()
		})
		if u, ok := Get(id.String()); !ok || u != custom {
			t.Fatal("custom unit not available")
		}
		m, err := protocol.NewMarket(protocol.MarketDef{Key: id, Exchange: 245, CodePattern: "X[0-9]"})
		if err != nil {
			t.Fatal(err)
		}
		if err := protocol.Register(m); err != nil {
			t.Fatal(err)
		}
	}
	if u, ok := Get("OccupiedUnit"); !ok || u.Name() != id.String() {
		t.Fatal("protocol unit did not take precedence", u)
	}
	count := 0
	for _, u := range Units() {
		if strings.EqualFold(u.Name(), id.String()) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("visible units with name %s: %d", id, count)
	}
	if c, err := ParseCode("OccupiedUnit.X1"); err != nil || c.Market != id || c.Code != "X1" {
		t.Fatal(c, err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("custom unit registered over protocol route")
		}
	}()
	Register(&mockUnit{name: "OccupiedUnit"})
}
