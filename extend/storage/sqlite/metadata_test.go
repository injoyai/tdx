package sqlite

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
	"xorm.io/xorm/names"
)

func TestCodeScopeIdentityAndRollback(t *testing.T) {
	for _, snake := range []bool{false, true} {
		t.Run(fmt.Sprint(snake), func(t *testing.T) {
			db, err := Open(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if snake {
				db.SetMapper(names.SnakeMapper{})
			}
			s, err := NewMetadata(db)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			at := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)
			seed := model.CodeSnapshot{Exchanges: []protocol.Exchange{protocol.ExchangeSH, protocol.ExchangeSZ}, FetchedAt: at, Rows: []*model.CodeModel{{Exchange: "sh", Code: "600000", Name: "original", Decimal: 2, LastPrice: 10}, {Exchange: "sh", Code: "600001", Name: "delisted"}, {Exchange: "sz", Code: "000001", Name: "untouched"}}}
			if err := s.ReplaceCodes(ctx, seed); err != nil {
				t.Fatal(err)
			}
			var old codeRecord
			if _, err := db.Where("`"+s.column("Code")+"`=? AND `"+s.column("Exchange")+"`=?", "600000", "sh").Get(&old); err != nil {
				t.Fatal(err)
			}
			fresh := model.CodeSnapshot{Exchanges: []protocol.Exchange{protocol.ExchangeSH}, FetchedAt: at.Add(time.Hour), Rows: []*model.CodeModel{{Exchange: "sh", Code: "600000", Name: "renamed"}}}
			if err := s.ReplaceCodes(ctx, fresh); err != nil {
				t.Fatal(err)
			}
			got, err := s.LoadCodes(ctx, seed.Exchanges)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Rows) != 2 || got.Rows[0].Name != "renamed" || got.Rows[0].Decimal != 0 || got.Rows[0].LastPrice != 0 || got.Rows[1].Name != "untouched" || !got.FetchedAt.Equal(at) {
				t.Fatal(got)
			}
			var changed codeRecord
			if _, err := db.ID(old.ID).Get(&changed); err != nil || changed.ID != old.ID || changed.InDate != old.InDate {
				t.Fatal("identity changed", changed, err)
			}
			if seed.Rows[0].Name != "original" {
				t.Fatal("mutated snapshot input")
			}
			before, err := s.LoadCodes(ctx, fresh.Exchanges)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("CREATE TRIGGER reject_late BEFORE INSERT ON codes WHEN NEW." + s.column("Code") + "='600101' BEGIN SELECT RAISE(ABORT,'reject'); END"); err != nil {
				t.Fatal(err)
			}
			bulk := fresh
			bulk.FetchedAt = at.Add(2 * time.Hour)
			bulk.Rows = nil
			for i := 0; i < 125; i++ {
				bulk.Rows = append(bulk.Rows, &model.CodeModel{Exchange: "sh", Code: fmt.Sprintf("600%03d", i), Name: "new"})
			}
			if err := s.ReplaceCodes(ctx, bulk); err == nil {
				t.Fatal("failed second batch committed")
			}
			after, err := s.LoadCodes(ctx, fresh.Exchanges)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("snapshot/state failed to roll back", after, err)
			}
			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			if err := s.ReplaceCodes(cancelled, fresh); err == nil {
				t.Fatal("accepted cancelled write")
			}
		})
	}
}

func TestMetadataStateFailureRollsBackData(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s, err := NewMetadata(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TRIGGER reject_state BEFORE INSERT ON metadataState BEGIN SELECT RAISE(ABORT,'state unavailable'); END"); err != nil {
		t.Fatal(err)
	}
	snapshot := model.CodeSnapshot{Exchanges: []protocol.Exchange{protocol.ExchangeSH}, FetchedAt: time.Now(), Rows: []*model.CodeModel{{Exchange: "sh", Code: "600000"}}}
	if err := s.ReplaceCodes(context.Background(), snapshot); err == nil {
		t.Fatal("state failure hidden")
	}
	got, err := s.LoadCodes(context.Background(), snapshot.Exchanges)
	if err != nil || len(got.Rows) != 0 || !got.FetchedAt.IsZero() {
		t.Fatal("data committed without state", got, err)
	}
}

func TestEventScopeAndBatchRollback(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s, err := NewMetadata(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	at := time.Date(2026, 1, 5, 15, 0, 0, 0, protocol.Location())
	seed := model.GbbqSnapshot{Codes: []string{"sh600000", "sz000001"}, FetchedAt: at, Rows: []*model.Gbbq{{Code: "sh600000", Time: at, Category: 5, C3: 100, C4: 200}, {Code: "sz000001", Time: at, Category: 5, C3: 300, C4: 400}}}
	if err := s.ReplaceGbbq(ctx, seed); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TRIGGER reject_event BEFORE INSERT ON gbbq WHEN NEW.C1=101 BEGIN SELECT RAISE(ABORT,'reject'); END"); err != nil {
		t.Fatal(err)
	}
	fresh := model.GbbqSnapshot{Codes: []string{"sh600000"}, FetchedAt: at.Add(time.Hour)}
	for i := 0; i < 125; i++ {
		fresh.Rows = append(fresh.Rows, &model.Gbbq{Code: "sh600000", Time: at, Category: 5, C1: float64(i)})
	}
	if err := s.ReplaceGbbq(ctx, fresh); err == nil {
		t.Fatal("partial event batch committed")
	}
	got, err := s.LoadGbbq(ctx, seed.Codes)
	if err != nil || len(got.Rows) != 2 || got.Rows[0].C3 != 100 || !got.FetchedAt.Equal(at) || !got.Rows[0].Time.Equal(at) {
		t.Fatal(got, err)
	}
	// Explicitly successful empty history clears only this security.
	fresh.Rows = nil
	if err := s.ReplaceGbbq(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	got, err = s.LoadGbbq(ctx, seed.Codes)
	if err != nil || len(got.Rows) != 1 || got.Rows[0].Code != "sz000001" {
		t.Fatal("unrelated event history lost", got, err)
	}
}

func TestCalendarCoverageCorrectionsAndRollback(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s, err := NewMetadata(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	seed := model.WorkdaySnapshot{Market: protocol.MarketAStock, Source: "fixture", From: "2026-01-02", Through: "2026-01-05", Dates: []string{"2026-01-02", "2026-01-03", "2026-01-05"}, FetchedAt: time.Now().UTC()}
	if err := s.SaveWorkdays(ctx, seed); err != nil {
		t.Fatal(err)
	}
	fresh := seed
	fresh.From = "2026-01-03"
	fresh.Through = "2026-01-06"
	fresh.Dates = []string{"2026-01-05", "2026-01-06"}
	if err := s.SaveWorkdays(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	before, err := s.LoadWorkdays(ctx, seed.Market)
	if err != nil || before.From != "2026-01-02" || before.Through != "2026-01-06" || !reflect.DeepEqual(before.Dates, []string{"2026-01-02", "2026-01-05", "2026-01-06"}) {
		t.Fatal(before, err)
	}
	for _, bad := range []model.WorkdaySnapshot{
		{Market: seed.Market, Source: seed.Source, From: "2026-02-02", Through: "2026-02-03", Dates: []string{"2026-02-02"}, FetchedAt: time.Now()},
		{Market: seed.Market, Source: "other", From: seed.From, Through: seed.Through, FetchedAt: time.Now()},
	} {
		if err := s.SaveWorkdays(ctx, bad); err == nil {
			t.Fatal("invalid coverage merged")
		}
	}
	if _, err := db.Exec("CREATE TRIGGER reject_calendar BEFORE INSERT ON calendarDate WHEN NEW.Date='2026-01-06' BEGIN SELECT RAISE(ABORT,'reject'); END"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveWorkdays(ctx, fresh); err == nil {
		t.Fatal("calendar failure hidden")
	}
	after, err := s.LoadWorkdays(ctx, seed.Market)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("calendar/state failed to roll back", after, err)
	}
}
