package workday

import (
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

type sourceDaysClient struct {
	tdx.Client
	rows []tdx.Kline
}

func (c *sourceDaysClient) GetKlineDayAll(string) (*tdx.KlineResp, error) {
	return &tdx.KlineResp{List: c.rows}, nil
}

func TestWorkdayUsesUnifiedSourceDates(t *testing.T) {
	db, err := xorms.NewSQLite(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := &sourceDaysClient{rows: []tdx.Kline{{Time: "2026-01-05"}}}
	w, err := New(WithDB(db), WithClient(c), WithSpec("@every 87600h"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	day := time.Date(2026, 1, 5, 15, 0, 0, 0, protocol.Location())
	if !w.Is(day) || !w.Is(day.UTC()) {
		t.Fatal("unified source date not published")
	}
	var row WorkdayModel
	if ok, err := db.Get(&row); err != nil || !ok || row.Unix != day.Unix() || row.Date != "20260105" {
		t.Fatal("legacy calendar key changed", row, err)
	}
	for _, bad := range []string{"20260106", "2026-02-30", "2026-01-06 15:00", "2026-1-06", ""} {
		c.rows = []tdx.Kline{{Time: "2026-01-06"}, {Time: bad}}
		if err := w.Update(); err == nil {
			t.Fatal("accepted malformed source date", bad)
		}
		if count, err := db.Count(new(WorkdayModel)); err != nil || count != 1 || w.Is(day.AddDate(0, 0, 1)) {
			t.Fatal("invalid batch published dates", count, err)
		}
	}
}
