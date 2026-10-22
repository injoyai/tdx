package update

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type updateFunc func() error

func (f updateFunc) Update() error { return f() }

func TestStartTimerStop(t *testing.T) {
	for _, retry := range []bool{false, true} {
		t.Run(map[bool]string{false: "in-flight", true: "retry-wait"}[retry], func(t *testing.T) {
			var calls atomic.Int32
			started, release := make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			stop, err := StartTimer("@every 1s", 3, updateFunc(func() error {
				if calls.Add(1) == 1 {
					return nil
				}
				close(started)
				if retry {
					return errors.New("retry later")
				}
				<-release
				return nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { unblock(); stop() }()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("scheduled update did not start")
			}
			if !retry {
				time.Sleep(1100 * time.Millisecond)
				if calls.Load() != 2 {
					t.Fatal("scheduled updates overlapped", calls.Load())
				}
			}
			done := make(chan struct{})
			go func() { stop(); close(done) }()
			if !retry {
				select {
				case <-done:
					t.Fatal("stop returned while update was running")
				case <-time.After(20 * time.Millisecond):
				}
				unblock()
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				unblock()
				t.Fatal("stop did not cancel retry/wait for update")
			}
			stop()
			if calls.Load() != 2 {
				t.Fatal("unexpected extra update", calls.Load())
			}
		})
	}
}

func TestStartTimerStartupFailure(t *testing.T) {
	want := errors.New("initial update failed")
	if stop, err := StartTimer("@every 1s", 3, updateFunc(func() error { return want })); stop != nil || !errors.Is(err, want) {
		t.Fatal("lost initial error", err)
	}
	var calls int
	if stop, err := StartTimer("invalid cron", 3, updateFunc(func() error { calls++; return nil })); stop != nil || err == nil || calls != 0 {
		t.Fatal("invalid schedule executed update", err, calls)
	}
}
