package pull

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestCustomScheduleAndSnapshotIsolation(t *testing.T) {
	s := testService(t)
	u := &scopeTestUnit{mockUnit: mockUnit{name: "test"}}
	s.units = []Unit{u}
	s.cfg.Schedules = map[protocol.Market]Schedule{"test": {TimeZone: "America/New_York", CloseTime: "20:00", Delay: 5 * time.Minute}}
	before, _ := time.Parse(time.RFC3339, "2026-07-16T00:04:59Z")
	if err := s.update(before, false); err != nil || len(u.days) != 0 {
		t.Fatal("ran before custom boundary", u.days, err)
	}
	if err := s.update(before.Add(time.Second), false); err != nil || len(u.days) != 1 {
		t.Fatal("missed custom boundary", u.days, err)
	}
	copy := s.Config()
	copy.Schedules["test"] = Schedule{TimeZone: "UTC", CloseTime: "00:00"}
	if s.cfg.Schedules["test"].TimeZone != "America/New_York" {
		t.Fatal("configuration leaked map")
	}
}

type unscheduledUnit struct{ Unit }

func (*unscheduledUnit) Name() string { return "unscheduled-review" }

func TestCustomMarketRequiresSchedule(t *testing.T) {
	u := &unscheduledUnit{}
	s := testService(t)
	if _, err := s.schedule(u); err == nil {
		t.Fatal("silently assumed mainland schedule")
	}
	for _, schedule := range []Schedule{{TimeZone: "", CloseTime: "15:00"}, {TimeZone: "UTC", CloseTime: "3:00"}, {TimeZone: "UTC", CloseTime: "24:00"}, {TimeZone: "UTC", CloseTime: "15:00", Delay: -time.Second}} {
		if _, err := schedule.boundary(time.Now()); err == nil {
			t.Fatal("accepted invalid schedule", schedule)
		}
	}
}

type mutableUniverseUnit struct {
	mockUnit
	calls atomic.Int32
	rows  []model.Code
}

func (u *mutableUniverseUnit) Codes(*Service) ([]model.Code, error) { return u.rows, nil }
func (u *mutableUniverseUnit) FetchDay(*Service, model.Code) error  { u.calls.Add(1); return nil }

func TestEmptyUniverseDoesNotPreventLaterDiscovery(t *testing.T) {
	s := testService(t)
	u := &mutableUniverseUnit{mockUnit: mockUnit{name: "test"}}
	s.units = []Unit{u}
	s.overrides = nil
	s.day = true
	s.minute = false
	if err := s.Update(true); err != nil {
		t.Fatal(err)
	}
	if _, has, err := s.store.LoadUpdate(s.updateKey("pull:test")); has || err != nil {
		t.Fatal("empty discovery certified completion", has, err)
	}
	u.rows = []model.Code{{Market: "test", Code: "one"}}
	if err := s.Update(true); err != nil || u.calls.Load() != 1 {
		t.Fatal("new universe skipped", u.calls.Load(), err)
	}
	if err := s.Update(true); err != nil || u.calls.Load() != 1 {
		t.Fatal("identical universe not deduplicated", u.calls.Load(), err)
	}
	s.cfg.UniverseVersion = "v2"
	u.rows = append(u.rows, model.Code{Market: "test", Code: "two"})
	if err := s.Update(true); err != nil || u.calls.Load() != 3 {
		t.Fatal("changed universe skipped", u.calls.Load(), err)
	}
}

func TestPermanentFailureDoesNotRetry(t *testing.T) {
	s := testService(t)
	u := &updateTestUnit{failDay: true, err: errors.New("invalid fixture")}
	if err := s.updateUnit(u); !errors.Is(err, u.err) || u.dayCalls != 1 {
		t.Fatal("retried permanent error", u.dayCalls, err)
	}
}

func TestRetryBackoffIsCancelable(t *testing.T) {
	s := testService(t)
	s.cfg.RetryDelay = time.Hour
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.retryWait(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestProgressIsSerializedAndCountsAllCodes(t *testing.T) {
	s := testService(t)
	s.goro = 4
	s.day = true
	s.minute = false
	u := &mockUnit{name: "test"}
	s.overrides = map[protocol.Market][]model.Code{"test": {{Code: "one"}, {Code: "two"}, {Code: "three"}}}
	var active atomic.Int32
	count := 0
	s.cfg.OnProgress = func(p Progress) {
		if active.Add(1) != 1 {
			t.Error("concurrent progress callbacks")
		}
		defer active.Add(-1)
		count++
		if p.Completed != count || p.Total != 3 || p.Err != nil {
			t.Error("invalid progress", p)
		}
	}
	if err := s.updateUnit(u); err != nil || count != 3 {
		t.Fatal("missing progress", count, err)
	}
}
