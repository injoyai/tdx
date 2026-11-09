package pull

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestUpdateOptionsIndependentControls(t *testing.T) {
	for _, early := range []bool{false, true} {
		for _, recheck := range []bool{false, true} {
			for _, marked := range []bool{false, true} {
				for _, before := range []bool{false, true} {
					t.Run(fmt.Sprintf("early=%t/recheck=%t/marked=%t/before=%t", early, recheck, marked, before), func(t *testing.T) {
						s := testService(t)
						u := &scheduledTestUnit{market: protocol.MarketAStock}
						s.units, s.goro = []Unit{u}, 1
						now := time.Date(2026, 1, 15, 16, 0, 0, 0, protocol.Location())
						if before {
							now = now.Add(-time.Hour)
						}
						cutoff, err := marketUpdateTime(u.market, now)
						if err != nil {
							t.Fatal(err)
						}
						if before {
							cutoff = cutoff.AddDate(0, 0, -1)
						}
						key := s.updateKey("pull:" + u.Name())
						if marked {
							if err := s.store.SaveUpdate(key, cutoff.Unix()); err != nil {
								t.Fatal(err)
							}
						}
						if err := s.updateWithOptions(now, UpdateOptions{AllowEarly: early, Recheck: recheck}); err != nil {
							t.Fatal(err)
						}
						want := 0
						if (!before || early) && (!marked || recheck) {
							want = 1
						}
						if u.dayCalls != want || u.minCalls != want {
							t.Fatalf("calls day=%d minute=%d, want %d", u.dayCalls, u.minCalls, want)
						}
						if want == 1 {
							unix, has, err := s.store.LoadUpdate(key)
							if err != nil || !has || unix != cutoff.Unix() {
								t.Fatalf("incorrect window: %d %t %v", unix, has, err)
							}
						}
					})
				}
			}
		}
	}
}

func TestRecheckPreservesErrorsAndCompletion(t *testing.T) {
	s := testService(t)
	u := &scheduledTestUnit{market: protocol.MarketAStock}
	s.units, s.goro = []Unit{u}, 1
	now := time.Date(2026, 1, 15, 16, 0, 0, 0, protocol.Location())
	if err := s.update(now, false); err != nil {
		t.Fatal(err)
	}
	key := s.updateKey("pull:" + u.Name())
	prior, _, _ := s.store.LoadUpdate(key)
	want := errors.New("marker unavailable")
	store := &markerErrorStore{BarStore: s.store, loadErr: want}
	s.store = store
	if err := s.updateWithOptions(now, UpdateOptions{Recheck: true}); !errors.Is(err, want) || u.dayCalls != 1 {
		t.Fatal("recheck hid read error", err)
	}
	store.loadErr = nil
	u.failDay, u.err = true, errors.New("fetch failed")
	if err := s.updateWithOptions(now, UpdateOptions{Recheck: true}); !errors.Is(err, u.err) {
		t.Fatal(err)
	}
	got, has, err := s.store.LoadUpdate(key)
	if err != nil || !has || got != prior {
		t.Fatal("failed recheck changed old completion", got, err)
	}
}

func TestUpdateWithOptionsCancellation(t *testing.T) {
	s := testService(t)
	if err := s.UpdateWithOptions(nil, UpdateOptions{}); err == nil {
		t.Fatal("nil context accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.UpdateWithOptions(ctx, UpdateOptions{Recheck: true}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	s.updateGate <- struct{}{}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	queued := &queuedUpdateContext{Context: ctx, waiting: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- s.UpdateWithOptions(queued, UpdateOptions{AllowEarly: true, Recheck: true}) }()
	select {
	case <-queued.waiting:
	case <-time.After(time.Second):
		t.Fatal("did not queue")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not release queued update")
	}
	<-s.updateGate
}
