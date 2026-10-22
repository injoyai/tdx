package gbbq

import (
	"testing"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/lib/update"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

type gbbqRowsClient struct {
	tdx.Client
	rows []*protocol.Gbbq
}

func (c *gbbqRowsClient) GetGbbqAll() (map[string][]*protocol.Gbbq, error) {
	return map[string][]*protocol.Gbbq{"sz000001": c.rows}, nil
}

func TestGbbqRefreshRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		db, err := xorms.NewSQLite(":memory:")
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Sync2(new(protocol.Gbbq)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Insert(&protocol.Gbbq{Code: "old", C1: -1}); err != nil {
			t.Fatal(err)
		}
		u, err := update.New(db, 9, 0)
		if err != nil {
			t.Fatal(err)
		}
		c := &gbbqRowsClient{rows: make([]*protocol.Gbbq, 125)}
		for j := range c.rows {
			c.rows[j] = &protocol.Gbbq{Code: "sz000001", Category: 5, C1: float64(j), C3: 100, C4: 200}
		}
		if fail {
			if _, err := db.Exec("CREATE TRIGGER reject_refresh BEFORE INSERT ON gbbq WHEN NEW.C1=101 BEGIN SELECT RAISE(ABORT, 'reject'); END"); err != nil {
				t.Fatal(err)
			}
		}
		g := &Gbbq{db: db, c: c, updated: u, updateKey: "batch"}
		_, err = g.update()
		if (err != nil) != fail {
			t.Fatal("unexpected refresh result", err)
		}
		var rows []*protocol.Gbbq
		if err := db.Asc("C1").Find(&rows); err != nil {
			t.Fatal(err)
		}
		if fail {
			if len(rows) != 1 || rows[0].Code != "old" {
				t.Fatal("failed refresh did not roll back", rows)
			}
		} else {
			if len(rows) != 125 || rows[124].C1 != 124 || rows[0].C3 != 100 {
				t.Fatal("refresh lost fields", rows)
			}
		}
		if done, err := u.IsUpdated("batch"); err != nil || done == fail {
			t.Fatal("invalid update mark", done, err)
		}
		db.Close()
	}
}

func BenchmarkGbbqRefresh(b *testing.B) {
	db, err := xorms.NewSQLite(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	if err := db.Sync2(new(protocol.Gbbq)); err != nil {
		b.Fatal(err)
	}
	u, err := update.New(db, 9, 0)
	if err != nil {
		b.Fatal(err)
	}
	u.IsUpdated("bench")
	c := &gbbqRowsClient{rows: make([]*protocol.Gbbq, 10000)}
	for j := range c.rows {
		c.rows[j] = &protocol.Gbbq{Code: "sz000001", Category: 5, C1: float64(j), C3: 100, C4: 200}
	}
	g := &Gbbq{db: db, c: c, updated: u, updateKey: "bench"}
	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		if _, err := g.update(); err != nil {
			b.Fatal(err)
		}
	}
}
