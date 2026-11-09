package pull

import (
	"errors"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/storage"
	"github.com/injoyai/tdx/v2/protocol"
)

type scheduledTestUnit struct {
	updateTestUnit
	market protocol.Market
}

func (u *scheduledTestUnit) Name() string { return u.market.String() }

func TestMarketUpdateTime(t *testing.T) {
	for _, tc := range []struct {
		market    protocol.Market
		now, want string
	}{
		{protocol.MarketAStock, "2026-01-15T00:00:00Z", "2026-01-15T07:35:00Z"},
		{protocol.MarketFuture, "2026-01-15T00:00:00Z", "2026-01-15T07:35:00Z"},
		{protocol.MarketHK, "2026-01-15T00:00:00Z", "2026-01-15T08:15:00Z"},
		{protocol.MarketHKIndex, "2026-01-15T00:00:00Z", "2026-01-15T08:15:00Z"},
		{protocol.MarketUS, "2026-01-16T00:00:00Z", "2026-01-16T01:05:00Z"},
		{protocol.MarketUS, "2026-07-16T00:00:00Z", "2026-07-16T00:05:00Z"},
		{protocol.MarketUS, "2026-03-08T12:00:00Z", "2026-03-09T00:05:00Z"},
		{protocol.MarketUS, "2026-11-01T12:00:00Z", "2026-11-02T01:05:00Z"},
	} {
		now, _ := time.Parse(time.RFC3339, tc.now)
		cutoff, err := marketUpdateTime(tc.market, now)
		if err != nil || cutoff.UTC().Format(time.RFC3339) != tc.want {
			t.Fatalf("%s at %s: cutoff=%s want=%s err=%v", tc.market, tc.now, cutoff, tc.want, err)
		}
	}
}

func TestUpdateMarketsAfterClose(t *testing.T) {
	s := testService(t)
	s.goro = 1
	cn := &scheduledTestUnit{market: protocol.MarketAStock}
	hk := &scheduledTestUnit{market: protocol.MarketHK}
	us := &scheduledTestUnit{market: protocol.MarketUS}
	s.units = []Unit{cn, hk, us}
	// 旧整体标记不能挡住晚收盘市场。
	if err := s.store.SaveUpdate(s.updateKey("pull"), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		at   string
		want [3]int
	}{
		{"2026-01-15T15:34:59+08:00", [3]int{0, 0, 0}},
		{"2026-01-15T15:35:00+08:00", [3]int{1, 0, 0}},
		{"2026-01-15T16:14:59+08:00", [3]int{1, 0, 0}},
		{"2026-01-15T16:15:00+08:00", [3]int{1, 1, 0}},
		{"2026-01-15T20:04:59-05:00", [3]int{1, 1, 0}},
		{"2026-01-15T20:05:00-05:00", [3]int{1, 1, 1}},
		{"2026-01-15T20:10:00-05:00", [3]int{1, 1, 1}},
		{"2026-01-16T15:35:00+08:00", [3]int{2, 1, 1}},
		{"2026-01-17T15:35:00+08:00", [3]int{3, 1, 1}}, // 周六也允许一次请求。
	} {
		now, _ := time.Parse(time.RFC3339, tc.at)
		if err := s.update(now, false); err != nil {
			t.Fatal(err)
		}
		got := [3]int{cn.dayCalls, hk.dayCalls, us.dayCalls}
		if got != tc.want {
			t.Fatalf("%s: calls=%v want=%v", tc.at, got, tc.want)
		}
		for _, u := range []*scheduledTestUnit{cn, hk, us} {
			if u.dayCalls != u.minCalls {
				t.Fatal("period calls differ", u.Name())
			}
		}
	}
}

func TestUpdateCompletionBoundary(t *testing.T) {
	now := time.Date(2026, 1, 15, 15, 35, 0, 0, protocol.Location())
	for _, tc := range []struct {
		name   string
		has    bool
		offset int64
		calls  int
	}{
		{"missing", false, 0, 1},
		{"before", true, -1, 1},
		{"exact", true, 0, 0},
		{"after", true, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testService(t)
			u := &scheduledTestUnit{market: protocol.MarketAStock}
			s.units, s.goro = []Unit{u}, 1
			key := s.updateKey("pull:" + u.Name())
			if tc.has {
				if err := s.store.SaveUpdate(key, now.Unix()+tc.offset); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.update(now, false); err != nil {
				t.Fatal(err)
			}
			if u.dayCalls != tc.calls || u.minCalls != tc.calls {
				t.Fatal("completion boundary changed", u.dayCalls, u.minCalls)
			}
		})
	}
}

type markerErrorStore struct {
	storage.BarStore
	loadErr, saveErr error
}

func (s *markerErrorStore) LoadUpdate(key string) (int64, bool, error) {
	if s.loadErr != nil {
		return 0, false, s.loadErr
	}
	return s.BarStore.LoadUpdate(key)
}

func (s *markerErrorStore) SaveUpdate(key string, unix int64) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	return s.BarStore.SaveUpdate(key, unix)
}

func TestUpdateMarkerErrorsRemainRetryable(t *testing.T) {
	for _, stage := range []string{"load", "save"} {
		t.Run(stage, func(t *testing.T) {
			s := testService(t)
			u := &scheduledTestUnit{market: protocol.MarketAStock}
			s.units, s.goro = []Unit{u}, 1
			want := errors.New("marker " + stage + " failed")
			store := &markerErrorStore{BarStore: s.store}
			s.store = store
			calls := 0
			if stage == "load" {
				store.loadErr = want
			} else {
				store.saveErr = want
				calls = 1
			}
			now := time.Date(2026, 1, 15, 15, 35, 0, 0, protocol.Location())
			if err := s.update(now, false); !errors.Is(err, want) {
				t.Fatal("marker error swallowed", err)
			}
			if u.dayCalls != calls || u.minCalls != calls {
				t.Fatal("unexpected fetch on marker error", u.dayCalls, u.minCalls)
			}
			key := s.updateKey("pull:" + u.Name())
			if _, has, err := store.BarStore.LoadUpdate(key); err != nil || has {
				t.Fatal("failed marker operation published completion", has, err)
			}
			store.loadErr, store.saveErr = nil, nil
			if err := s.update(now, false); err != nil {
				t.Fatal(err)
			}
			if err := s.update(now, false); err != nil {
				t.Fatal(err)
			}
			if u.dayCalls != calls+1 || u.minCalls != calls+1 {
				t.Fatal("retry or deduplication failed", u.dayCalls, u.minCalls)
			}
		})
	}
}

func TestUpdateFailedMarketRetriesIndependently(t *testing.T) {
	s := testService(t)
	s.goro, s.retry = 1, 1
	want := errors.New("fetch failed")
	cn := &scheduledTestUnit{market: protocol.MarketAStock, updateTestUnit: updateTestUnit{failDay: true, err: want}}
	hk := &scheduledTestUnit{market: protocol.MarketHK}
	s.units = []Unit{cn, hk}
	now, _ := time.Parse(time.RFC3339, "2026-01-15T16:15:00+08:00")
	if err := s.update(now, false); !errors.Is(err, want) {
		t.Fatal(err)
	}
	if cn.dayCalls != 1 || cn.minCalls != 0 || hk.dayCalls != 1 || hk.minCalls != 1 {
		t.Fatal("failure blocked another market")
	}
	cn.failDay = false
	if err := s.update(now, false); err != nil {
		t.Fatal(err)
	}
	if cn.dayCalls != 2 || cn.minCalls != 1 || hk.dayCalls != 1 || hk.minCalls != 1 {
		t.Fatal("retry did not preserve completed market")
	}
}

func TestEarlyUpdateDoesNotSuppressAfterClose(t *testing.T) {
	s := testService(t)
	s.goro = 1
	u := &scheduledTestUnit{market: protocol.MarketAStock}
	s.units = []Unit{u}
	for i, at := range []string{"2026-01-15T14:00:00+08:00", "2026-01-15T15:35:00+08:00"} {
		now, _ := time.Parse(time.RFC3339, at)
		if err := s.update(now, i == 0); err != nil {
			t.Fatal(err)
		}
		if u.dayCalls != i+1 {
			t.Fatal("early mark suppressed after-close update", u.dayCalls)
		}
		if err := s.update(now, true); err != nil {
			t.Fatal(err)
		}
		if u.dayCalls != i+1 {
			t.Fatal("forced call ignored market deduplication")
		}
	}
}

func TestUpdateMarksOnlyItsSelectedWindow(t *testing.T) {
	for _, tc := range []struct{ name, at, want string }{
		{"before-close", "2026-01-15T15:34:59+08:00", "2026-01-14T15:35:00+08:00"},
		{"after-close", "2026-01-15T20:00:00+08:00", "2026-01-15T15:35:00+08:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := testService(t)
			u := &scheduledTestUnit{market: protocol.MarketAStock}
			s.units = []Unit{u}
			now, _ := time.Parse(time.RFC3339, tc.at)
			want, _ := time.Parse(time.RFC3339, tc.want)
			if err := s.update(now, true); err != nil {
				t.Fatal(err)
			}
			unix, has, err := s.store.LoadUpdate(s.updateKey("pull:" + u.Name()))
			if err != nil || !has || unix != want.Unix() {
				t.Fatal("completion certified the wrong window", unix, want.Unix(), has, err)
			}
		})
	}
}
