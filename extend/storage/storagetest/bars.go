// Package storagetest provides the public BarStore conformance suite.
// Factories must supply a fresh, empty, isolated dataset for each subtest and
// register resource cleanup with t.Cleanup. No database driver is required here.
package storagetest

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/storage"
	"github.com/injoyai/tdx/v2/protocol"
)

type FailurePoint string

const (
	// DayInsert fails insertion inside SaveDay, after its tail deletion.
	DayInsert FailurePoint = "day_insert"
	// MinuteInsert fails insertion inside SaveMinute, after its tail deletion.
	MinuteInsert FailurePoint = "minute_insert"
	// CoverageWrite fails coverage persistence after minute rows were written.
	CoverageWrite FailurePoint = "coverage_write"
)

type Failure struct {
	Point FailurePoint
	Code  model.Code
	Year  int // zero for daily bars
}

type Fixture struct {
	Store storage.BarStore
	// Fail arms a real persistence failure for the next save to the given target.
	// It must leave reads working and register cleanup for the injected fault.
	// Nil explicitly skips transaction failure subtests, not the common suite.
	Fail func(*testing.T, Failure)
}

type Factory func(*testing.T) Fixture

// Run checks backend-independent behavior. It does not test a particular schema,
// ORM, file layout, or network source. Skipped fault tests are not verified passes.
func Run(t *testing.T, factory Factory) {
	t.Helper()
	c := model.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	run := func(name string, test func(*testing.T, Fixture)) {
		t.Run(name, func(t *testing.T) {
			f := factory(t)
			if f.Store == nil {
				t.Fatal("factory returned nil Store")
			}
			test(t, f)
		})
	}
	run("empty", func(t *testing.T, f Fixture) {
		s := f.Store
		if id := s.Identity(); id == "" || id != s.Identity() {
			t.Fatal("Identity must be nonempty and stable")
		}
		rows, err := s.QueryDay(c, "", "")
		check(t, err)
		if len(rows) != 0 {
			t.Fatal("nonempty new dataset")
		}
		mins, err := s.QueryMinute(c, "", "")
		check(t, err)
		if len(mins) != 0 {
			t.Fatal("nonempty new minute dataset")
		}
		last, err := s.LastDayTime(c)
		check(t, err)
		equal(t, last, "")
		last, err = s.LastMinuteTime(c, 2025)
		check(t, err)
		equal(t, last, "")
		cov, err := s.QueryMinuteCoverage(c, 2025)
		check(t, err)
		equal(t, cov, (*model.MinuteCoverage)(nil))
		scan, err := s.LoadMinuteEmptyScan(c, 2025)
		check(t, err)
		equal(t, scan, (*model.MinuteEmptyScan)(nil))
	})
	run("day_range_and_tail", func(t *testing.T, f Fixture) {
		s := f.Store
		rows := days()
		// Unsorted input must still be queried in source-time order.
		check(t, s.SaveDay(c, "", []*model.KlineDay{rows[2], rows[0], rows[1]}))
		for _, tc := range []struct {
			start, end string
			want       []*model.KlineDay
		}{
			{"", "", rows}, {rows[1].Time, rows[1].Time, rows[1:2]},
			{"", rows[1].Time, rows[:2]}, {rows[1].Time, "", rows[1:]},
			{"2025-02-01", "", nil},
		} {
			got, err := s.QueryDay(c, tc.start, tc.end)
			check(t, err)
			if len(tc.want) == 0 {
				equal(t, len(got), 0)
			} else {
				equal(t, got, tc.want)
			}
		}
		last, err := s.LastDayTime(c)
		check(t, err)
		equal(t, last, rows[2].Time)
		replacement := &model.KlineDay{Time: rows[1].Time, Close: 9, Amount: amount(0)}
		for range 2 {
			check(t, s.SaveDay(c, rows[1].Time, []*model.KlineDay{replacement}))
			got, err := s.QueryDay(c, "", "")
			check(t, err)
			equal(t, got, []*model.KlineDay{rows[0], replacement})
		}
		check(t, s.SaveDay(c, "", rows[2:]))
		got, err := s.QueryDay(c, "", "")
		check(t, err)
		equal(t, got, rows[2:])
	})
	run("minute_range_year_and_tail", func(t *testing.T, f Fixture) {
		s := f.Store
		rows := minutes()
		check(t, s.SaveMinute(c, 2025, "", []*model.KlineMinute{rows[2], rows[0], rows[1]}, nil))
		next := []*model.KlineMinute{{Time: "2026-01-05 09:31", Close: 4, Source: model.MinuteSourceTrade}}
		check(t, s.SaveMinute(c, 2026, "", next, nil))
		for _, tc := range []struct {
			start, end string
			want       []*model.KlineMinute
		}{
			{"", "", append(append([]*model.KlineMinute{}, rows...), next...)},
			{rows[1].Time, rows[1].Time, rows[1:2]}, {"", rows[1].Time, rows[:2]},
			{rows[2].Time, "", []*model.KlineMinute{rows[2], next[0]}},
			{"2027-01-01 00:00", "", nil},
		} {
			got, err := s.QueryMinute(c, tc.start, tc.end)
			check(t, err)
			if len(tc.want) == 0 {
				equal(t, len(got), 0)
			} else {
				equal(t, got, tc.want)
			}
		}
		last, err := s.LastMinuteTime(c, 2025)
		check(t, err)
		equal(t, last, rows[2].Time)
		replacement := &model.KlineMinute{Time: rows[1].Time, Close: 9, Source: model.MinuteSourceTrade}
		for range 2 {
			check(t, s.SaveMinute(c, 2025, rows[1].Time, []*model.KlineMinute{replacement}, nil))
		}
		got, err := s.QueryMinute(c, "", "")
		check(t, err)
		equal(t, got, []*model.KlineMinute{rows[0], replacement, next[0]})
		check(t, s.SaveMinute(c, 2025, "", rows[2:], nil))
		got, err = s.QueryMinute(c, "", "")
		check(t, err)
		equal(t, got, []*model.KlineMinute{rows[2], next[0]})
	})
	run("empty_batch_preserves_data_and_state", func(t *testing.T, f Fixture) {
		s := f.Store
		cov := &model.MinuteCoverage{From: "2025-01-02 00:00"}
		check(t, s.SaveDay(c, "", days()))
		check(t, s.SaveMinute(c, 2025, "", minutes(), cov))
		check(t, s.SaveUpdate("test", 123))
		for _, rows := range [][]*model.KlineDay{nil, {}} {
			check(t, s.SaveDay(c, "", rows))
		}
		for _, rows := range [][]*model.KlineMinute{nil, {}} {
			check(t, s.SaveMinute(c, 2025, "", rows, &model.MinuteCoverage{From: "2025-01-01 00:00"}))
			check(t, s.SaveMinute(c, 2024, "", rows, &model.MinuteCoverage{From: "2024-01-01 00:00"}))
		}
		got, err := s.QueryDay(c, "", "")
		check(t, err)
		equal(t, got, days())
		mins, err := s.QueryMinute(c, "", "")
		check(t, err)
		equal(t, mins, minutes())
		gotCov, err := s.QueryMinuteCoverage(c, 2025)
		check(t, err)
		equal(t, gotCov, cov)
		gotCov, err = s.QueryMinuteCoverage(c, 2024)
		check(t, err)
		equal(t, gotCov, (*model.MinuteCoverage)(nil))
		unix, has, err := s.LoadUpdate("test")
		check(t, err)
		equal(t, unix, int64(123))
		equal(t, has, true)
	})
	run("coverage", func(t *testing.T, f Fixture) {
		s := f.Store
		write := func(cov *model.MinuteCoverage) {
			check(t, s.SaveMinute(c, 2025, minutes()[0].Time, minutes(), cov))
		}
		write(&model.MinuteCoverage{From: "2025-01-03 00:00"})
		write(&model.MinuteCoverage{From: "2025-01-01 00:00"})
		write(&model.MinuteCoverage{From: "2025-01-04 00:00"})
		got, err := s.QueryMinuteCoverage(c, 2025)
		check(t, err)
		equal(t, got, &model.MinuteCoverage{From: "2025-01-01 00:00"})
		for _, cov := range []*model.MinuteCoverage{
			{From: "2025-01-01 00:00", Through: "2025-01-03 00:00", TradeFallback: true},
			{From: "2025-01-03 00:00", Through: "2025-01-06 00:00", TradeFallback: true},
		} {
			write(cov)
		}
		got, err = s.QueryMinuteCoverage(c, 2025)
		check(t, err)
		equal(t, got, &model.MinuteCoverage{From: "2025-01-01 00:00", Through: "2025-01-06 00:00", TradeFallback: true})
		separate := &model.MinuteCoverage{From: "2025-02-01 00:00", Through: "2025-02-02 00:00", TradeFallback: true}
		write(separate)
		got, err = s.QueryMinuteCoverage(c, 2025)
		check(t, err)
		equal(t, got, separate)
	})
	run("isolation", func(t *testing.T, f Fixture) {
		s := f.Store
		codes := []model.Code{c, {Market: protocol.MarketIndex, Code: c.Code}, {Market: c.Market, Code: "sz000002"},
			{Market: protocol.MarketFuture, Code: "cff/IF2609"}, {Market: protocol.MarketFuture, Code: "cff/if2609"},
			{Market: "custom", Code: "A"}, {Market: "custom", Code: "a"}, {Market: "Custom", Code: "a"},
			{Market: "custom", Code: "~41"}, {Market: "custom", Code: "é"}, {Market: "custom", Code: "e\u0301"}}
		for i, code := range codes {
			check(t, s.SaveDay(code, "", []*model.KlineDay{{Time: "2025-01-06", Close: float64(i)}}))
			check(t, s.SaveMinute(code, 2025, "", []*model.KlineMinute{{Time: "2025-01-06 09:31", Close: float64(i)}}, &model.MinuteCoverage{From: fmt.Sprintf("2025-01-01 00:%02d", i)}))
			check(t, s.SaveMinuteEmptyScan(code, 2025, model.MinuteEmptyScan{From: "2025-01-01 00:00", CheckedAt: int64(i + 1)}))
		}
		for i, code := range codes {
			rows, err := s.QueryDay(code, "", "")
			check(t, err)
			equal(t, rows, []*model.KlineDay{{Time: "2025-01-06", Close: float64(i)}})
			mins, err := s.QueryMinute(code, "", "")
			check(t, err)
			equal(t, mins, []*model.KlineMinute{{Time: "2025-01-06 09:31", Close: float64(i)}})
			cov, err := s.QueryMinuteCoverage(code, 2025)
			check(t, err)
			equal(t, cov, &model.MinuteCoverage{From: fmt.Sprintf("2025-01-01 00:%02d", i)})
			scan, err := s.LoadMinuteEmptyScan(code, 2025)
			check(t, err)
			equal(t, scan, &model.MinuteEmptyScan{From: "2025-01-01 00:00", CheckedAt: int64(i + 1)})
		}
	})
	run("marks_and_empty_scan", func(t *testing.T, f Fixture) {
		s := f.Store
		for range 2 {
			_, has, err := s.LoadUpdate("missing")
			check(t, err)
			equal(t, has, false)
		}
		for _, unix := range []int64{123, 456} {
			check(t, s.SaveUpdate("a", unix))
			got, has, err := s.LoadUpdate("a")
			check(t, err)
			equal(t, has, true)
			equal(t, got, unix)
		}
		_, has, err := s.LoadUpdate("b")
		check(t, err)
		equal(t, has, false)
		for _, scan := range []model.MinuteEmptyScan{{From: "2025-01-01 00:00", CheckedAt: 123}, {From: "2025-02-01 00:00", CheckedAt: 456}} {
			check(t, s.SaveMinuteEmptyScan(c, 2025, scan))
			got, err := s.LoadMinuteEmptyScan(c, 2025)
			check(t, err)
			equal(t, got, &scan)
		}
		got, err := s.LoadMinuteEmptyScan(c, 2024)
		check(t, err)
		equal(t, got, (*model.MinuteEmptyScan)(nil))
	})
	run("invalid_batch_preserves_data", func(t *testing.T, f Fixture) {
		s := f.Store
		check(t, s.SaveDay(c, "", days()))
		check(t, s.SaveMinute(c, 2025, "", minutes(), nil))
		if err := s.SaveDay(c, "", []*model.KlineDay{{Time: "bad"}}); err == nil {
			t.Fatal("invalid date accepted")
		}
		if err := s.SaveMinute(c, 2025, "", []*model.KlineMinute{{Time: "2026-01-05 09:31"}}, nil); err == nil {
			t.Fatal("wrong source year accepted")
		}
		rows, err := s.QueryDay(c, "", "")
		check(t, err)
		equal(t, rows, days())
		mins, err := s.QueryMinute(c, "", "")
		check(t, err)
		equal(t, mins, minutes())
	})
	run("concurrent", func(t *testing.T, f Fixture) {
		var wg sync.WaitGroup
		for i := range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				code := model.Code{Market: c.Market, Code: fmt.Sprintf("sz%06d", i+1)}
				for range 3 {
					if err := f.Store.SaveDay(code, "", days()); err != nil {
						t.Error(err)
						return
					}
					rows, err := f.Store.QueryDay(code, "", "")
					if err != nil || !reflect.DeepEqual(rows, days()) {
						t.Errorf("concurrent read/write: %v, %+v", err, rows)
						return
					}
					if err := f.Store.SaveMinute(code, 2025, "", minutes(), nil); err != nil {
						t.Error(err)
						return
					}
					if err := f.Store.SaveUpdate(code.Code, 123); err != nil {
						t.Error(err)
						return
					}
				}
			}()
		}
		wg.Wait()
	})
	for _, point := range []FailurePoint{DayInsert, MinuteInsert, CoverageWrite} {
		run(string(point)+"_rollback", func(t *testing.T, f Fixture) {
			if f.Fail == nil {
				t.Skip("backend did not provide persistence fault injection")
			}
			s := f.Store
			cov := &model.MinuteCoverage{From: "2025-01-02 00:00"}
			check(t, s.SaveDay(c, "", days()))
			check(t, s.SaveMinute(c, 2025, "", minutes(), cov))
			year := 2025
			if point == DayInsert {
				year = 0
			}
			f.Fail(t, Failure{Point: point, Code: c, Year: year})
			var err error
			if point == DayInsert {
				err = s.SaveDay(c, days()[1].Time, []*model.KlineDay{{Time: days()[1].Time, Close: 99}})
			} else {
				err = s.SaveMinute(c, 2025, minutes()[1].Time, []*model.KlineMinute{{Time: minutes()[1].Time, Close: 99}}, &model.MinuteCoverage{From: "2025-01-01 00:00"})
			}
			if err == nil {
				t.Fatal("persistence failure was hidden")
			}
			rows, err := s.QueryDay(c, "", "")
			check(t, err)
			equal(t, rows, days())
			mins, err := s.QueryMinute(c, "", "")
			check(t, err)
			equal(t, mins, minutes())
			got, err := s.QueryMinuteCoverage(c, 2025)
			check(t, err)
			equal(t, got, cov)
		})
	}
}

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func equal(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(want)
		t.Fatalf("got %s, want %s", a, b)
	}
}
func amount(v float64) *float64 { return &v }
func days() []*model.KlineDay {
	return []*model.KlineDay{
		{Time: "2025-01-06", Open: 1, High: 3, Low: 1, Close: 2, Volume: 100, Amount: nil},
		{Time: "2025-01-07", Close: 3, Amount: amount(0)},
		{Time: "2025-01-08", Close: 4, Amount: amount(123.45)},
	}
}
func minutes() []*model.KlineMinute {
	return []*model.KlineMinute{
		{Time: "2025-01-06 09:31", Open: 1, High: 3, Low: 1, Close: 2, Volume: 100, Amount: nil},
		{Time: "2025-01-06 09:32", Close: 3, Amount: amount(0), Source: model.MinuteSourceTrade},
		{Time: "2025-01-06 09:33", Close: 4, Amount: amount(123.45)},
	}
}
