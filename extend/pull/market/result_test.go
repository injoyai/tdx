package market

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/extend/storage"
	"github.com/injoyai/tdx/v2/protocol"
)

type failingYearStore struct {
	storage.BarStore
	year int
	err  error
}

func (s *failingYearStore) SaveMinute(c model.Code, year int, from string, rows []*model.KlineMinute, coverage *model.MinuteCoverage) error {
	if year == s.year {
		return s.err
	}
	return s.BarStore.SaveMinute(c, year, from, rows, coverage)
}

func TestFetchResultFilteringAndPartialSave(t *testing.T) {
	c := model.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	failure := errors.New("write failed")
	store := &failingYearStore{BarStore: testStore(t.Name()), year: 2025, err: failure}
	s, err := pull.New(&pull.Config{Store: store, Codes: []string{"sz000001"}, StartAt: "20240101"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	day, err := saveDaysResult(s, c, "2024-01-02", []*model.KlineDay{{Time: "2024-01-01"}, {Time: "2024-01-03"}, {Time: "2024-01-02"}})
	if err != nil || !day.Known || day.Fetched != 3 || day.Saved != 2 || day.LatestTime != "2024-01-03" {
		t.Fatal(day, err)
	}
	data := []*model.KlineMinute{{Time: "2024-01-05 09:31"}, {Time: "2025-01-06 09:31"}}
	now := time.Date(2026, 9, 4, 16, 0, 0, 0, protocol.Location())
	result, err := pullMinutesResult(s, c, now, func(string) ([]*model.KlineMinute, error) { return data, nil }, nil)
	if !errors.Is(err, failure) || !result.Known || result.Fetched != 2 || result.Saved != 1 || result.LatestTime != data[1].Time {
		t.Fatal(result, err)
	}
	rows, err := s.QueryMinute(c, "", "")
	if err != nil || len(rows) != 1 || rows[0].Time != data[0].Time {
		t.Fatal(rows, err)
	}
}

func TestMinuteResultEmptyAndFuture(t *testing.T) {
	s, err := pull.New(&pull.Config{Store: testStore(t.Name()), Codes: []string{"sz000001"}, StartAt: "20260101"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	c := model.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	calls := 0
	fetch := func(string) ([]*model.KlineMinute, error) { calls++; return nil, nil }
	result, err := pullMinutesResult(s, c, time.Date(2026, 9, 4, 16, 0, 0, 0, protocol.Location()), fetch, nil)
	if err != nil || calls != 1 || result != (pull.FetchResult{Known: true}) {
		t.Fatal(result, err)
	}
	result, err = pullMinutesResult(s, c, time.Date(2025, 9, 4, 16, 0, 0, 0, protocol.Location()), fetch, nil)
	if err != nil || calls != 1 || result.SkipReason != pull.SkipFuture {
		t.Fatal(result, err)
	}
}

func TestBuiltinResultsAndRecheckKeepHistoricalCoverage(t *testing.T) {
	for _, input := range []string{"sz000001", "sh880001", "hk00700", "hk.HSI", "usAAPL", "cffIF2609"} {
		t.Run(input, func(t *testing.T) {
			year := time.Now().In(protocol.Location()).Year()
			dayTime := fmt.Sprintf("%04d-01-05", year)
			client := &unifiedClient{day: dayTime}
			pool, err := tdx.NewPool(func() (tdx.Client, error) { return client, nil }, 1)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { pool.Close() })
			var events []pull.Event
			s, err := pull.New(&pull.Config{Store: testStore(t.Name()), Codes: []string{input}, StartAt: fmt.Sprintf("%04d0101", year-2), Pool: pool, ExPool: pool,
				OnEvent: func(e pull.Event) { events = append(events, e) }})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { s.Close() })
			code, err := pull.ParseCode(input)
			if err != nil {
				t.Fatal(err)
			}
			u, _ := pull.Get(code.Market.String())
			unit, ok := u.(pull.ResultUnit)
			if !ok {
				t.Fatal("built-in Unit has no results", u.Name())
			}
			day, err := unit.FetchDayResult(s, code)
			if err != nil || day != (pull.FetchResult{Known: true, Fetched: 1, Saved: 1, LatestTime: dayTime}) {
				t.Fatal(day, err)
			}
			for range 2 {
				events = nil
				if err := s.UpdateWithOptions(context.Background(), pull.UpdateOptions{AllowEarly: true, Recheck: true}); err != nil {
					t.Fatal(err)
				}
				if len(events) != 3 || events[1].Stage != pull.StageMinute || events[1].Result.Fetched != 1 || events[1].Result.Saved != 1 || events[1].Result.LatestTime != dayTime+" 09:31" || events[2].Status != pull.StatusSuccess {
					t.Fatal(events)
				}
			}
			// Rechecking the current window does not rewrite historical state.
			for _, year := range []int{year - 2, year - 1} {
				scan, err := s.MinuteYearRecentlyEmpty(code, year, time.Date(year, 1, 1, 0, 0, 0, 0, protocol.Location()).Format(protocol.TimeLayout), time.Now())
				if err != nil || !scan {
					t.Fatal("historical empty scan lost", year, scan, err)
				}
			}
		})
	}
}
