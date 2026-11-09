package pull

import (
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

type resultTestUnit struct {
	mockUnit
	dayCalls    int
	dayError    error
	day, minute FetchResult
}

func (u *resultTestUnit) FetchDayResult(*Service, model.Code) (FetchResult, error) {
	u.dayCalls++
	if u.dayCalls == 1 && u.dayError != nil {
		return FetchResult{Known: true, Fetched: 99}, u.dayError
	}
	return u.day, nil
}
func (u *resultTestUnit) FetchMinuteResult(*Service, model.Code) (FetchResult, error) {
	return u.minute, nil
}

func TestNoEventKeepsLegacyUnitEntryPoints(t *testing.T) {
	s := testService(t)
	u := &resultTestUnit{mockUnit: mockUnit{name: "test"}}
	if err := s.updateUnit(u); err != nil || u.dayCalls != 0 {
		t.Fatal("result methods replaced the legacy entry point without an event subscriber", u.dayCalls, err)
	}
}

func TestStageEventsKeepLegacyProgressAndFinalAttempt(t *testing.T) {
	s := testService(t)
	s.goro = 1
	u := &resultTestUnit{mockUnit: mockUnit{name: "test"}, dayError: temporaryTestError{errors.New("try again")},
		day: FetchResult{Known: true, Fetched: 3, Saved: 2, LatestTime: "2026-01-05"}, minute: FetchResult{Known: true}}
	var events []Event
	progress := 0
	s.cfg.OnEvent = func(e Event) { events = append(events, e) }
	s.cfg.OnProgress = func(p Progress) {
		progress++
		if len(events) != 2 || p.Completed != 1 || p.Total != 1 || p.Err != nil {
			t.Error("legacy callback semantics changed", p)
		}
	}
	if err := s.updateUnit(u); err != nil {
		t.Fatal(err)
	}
	if progress != 1 || len(events) != 2 {
		t.Fatal(progress, events)
	}
	day, minute := events[0], events[1]
	if day.Stage != StageDay || day.Status != StatusSuccess || day.Attempts != 2 || day.Result != u.day || day.Completed != 0 || day.Total != 1 {
		t.Fatalf("day: %+v", day)
	}
	if minute.Stage != StageMinute || minute.Status != StatusNoData || minute.Attempts != 1 {
		t.Fatalf("minute: %+v", minute)
	}
}

func TestEventFailureSkipAndUnknownMetrics(t *testing.T) {
	for _, fail := range []bool{false, true} {
		s := testService(t)
		u := &updateTestUnit{failDay: fail, err: errors.New("day failed")}
		var events []Event
		s.cfg.OnEvent = func(e Event) { events = append(events, e) }
		err := s.updateUnit(u)
		if len(events) != 2 {
			t.Fatal(events)
		}
		if fail {
			if !errors.Is(err, u.err) || events[0].Status != StatusFailed || !errors.Is(events[0].Err, u.err) || events[1].Status != StatusSkipped || events[1].Reason != SkipDayFailed || events[1].Attempts != 0 {
				t.Fatal(events, err)
			}
		} else if err != nil || events[0].Status != StatusSuccess || events[0].Result.Known {
			t.Fatal("legacy Unit invented no-data metrics", events, err)
		}
	}
	s := testService(t)
	u := &resultTestUnit{mockUnit: mockUnit{name: "test"}, day: FetchResult{Known: true, SkipReason: SkipFuture}, minute: FetchResult{Known: true, SkipReason: SkipCovered}}
	var events []Event
	s.cfg.OnEvent = func(e Event) { events = append(events, e) }
	if err := s.updateUnit(u); err != nil {
		t.Fatal(err)
	}
	if events[0].Status != StatusSkipped || events[0].Reason != SkipFuture || events[1].Reason != SkipCovered {
		t.Fatal(events)
	}
}

func TestMarketEvents(t *testing.T) {
	s := testService(t)
	u := &mockUnit{name: "test"}
	s.units = []Unit{u}
	var events []Event
	s.cfg.OnEvent = func(e Event) { events = append(events, e) }
	now := time.Date(2026, 1, 15, 15, 0, 0, 0, protocol.Location())
	if err := s.updateWithOptions(now, UpdateOptions{Recheck: true}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Stage != "" || events[0].Reason != SkipBeforeClose {
		t.Fatal(events)
	}
	events = nil
	now = now.Add(time.Hour)
	if err := s.update(now, false); err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[2].Stage != "" || events[2].Status != StatusSuccess {
		t.Fatal(events)
	}
	events = nil
	if err := s.update(now, false); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Reason != SkipCompleted {
		t.Fatal(events)
	}
	events = nil
	markerErr := errors.New("commit failed")
	s.store = &markerErrorStore{BarStore: s.store, saveErr: markerErr}
	if err := s.updateWithOptions(now, UpdateOptions{Recheck: true}); !errors.Is(err, markerErr) {
		t.Fatal(err)
	}
	if len(events) != 3 || events[2].Status != StatusFailed || !errors.Is(events[2].Err, markerErr) {
		t.Fatal(events)
	}
	events = nil
	s.overrides = nil
	if err := s.updateWithOptions(now, UpdateOptions{Recheck: true}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Reason != SkipEmpty {
		t.Fatal(events)
	}
}

func TestEventsAndProgressAreSerializedTogether(t *testing.T) {
	s := testService(t)
	s.goro = 4
	s.overrides = map[protocol.Market][]model.Code{"test": {{Code: "one"}, {Code: "two"}, {Code: "three"}}}
	var active atomic.Int32
	enter := func() {
		if active.Add(1) != 1 {
			t.Error("concurrent callbacks")
		}
		runtime.Gosched()
		active.Add(-1)
	}
	events, completed := 0, 0
	s.cfg.OnEvent = func(e Event) {
		enter()
		events++
		if e.Total != 3 || e.Completed != completed {
			t.Error("stage changed completion count", e)
		}
	}
	s.cfg.OnProgress = func(p Progress) {
		enter()
		completed++
		if p.Completed != completed {
			t.Error(p)
		}
	}
	if err := s.updateUnit(&mockUnit{name: "test"}); err != nil {
		t.Fatal(err)
	}
	if events != 6 || completed != 3 {
		t.Fatal(events, completed)
	}
}
