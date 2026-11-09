package pull

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
)

type waitingUpdateUnit struct {
	mockUnit
	entered, release chan struct{}
}

type queuedUpdateContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *queuedUpdateContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func (u *waitingUpdateUnit) FetchDay(*Service, model.Code) error {
	close(u.entered)
	<-u.release
	return nil
}

func TestUpdateContextCancelsWhileQueued(t *testing.T) {
	s := testService(t)
	u := &waitingUpdateUnit{mockUnit: mockUnit{name: "test"}, entered: make(chan struct{}), release: make(chan struct{})}
	s.units, s.goro = []Unit{u}, 1
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() { firstDone <- s.UpdateContext(firstCtx, true) }()
	var release sync.Once
	t.Cleanup(func() {
		defer cancelFirst()
		release.Do(func() { close(u.release) })
		select {
		case err := <-firstDone:
			if err != nil {
				t.Error("first update changed", err)
			}
		case <-time.After(time.Second):
			t.Error("first update did not finish")
		}
	})
	select {
	case <-u.entered:
	case <-time.After(time.Second):
		t.Fatal("first update did not start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queued := &queuedUpdateContext{Context: ctx, waiting: make(chan struct{})}
	secondDone := make(chan error, 1)
	go func() { secondDone <- s.UpdateContext(queued, true) }()
	select {
	case <-queued.waiting:
	case <-time.After(time.Second):
		t.Fatal("queued update did not enter a cancellable wait")
	}
	cancel()
	select {
	case err := <-secondDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled update still waits for another update")
	}
	if s.Context() != firstCtx {
		t.Fatal("queued cancellation replaced the active context")
	}
}
