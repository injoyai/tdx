package pull

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

type discoveryClient struct{ tdx.Client }

func (*discoveryClient) Close() error { return nil }
func (*discoveryClient) GetInstruments() ([]protocol.ExInstrument, error) {
	return []protocol.ExInstrument{{Market: protocol.ExchangeUS.Uint8(), Code: "AAPL", Name: "Apple"}}, nil
}

type canceledDiscoveryUnit struct {
	mockUnit
	cancel context.CancelFunc
}

func (u *canceledDiscoveryUnit) Codes(*Service) ([]model.Code, error) {
	u.cancel()
	return nil, nil
}

func TestCanceledEmptyDiscoveryDoesNotMarkComplete(t *testing.T) {
	s := testService(t)
	s.overrides = nil
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u := &canceledDiscoveryUnit{mockUnit: mockUnit{name: "test"}, cancel: cancel}
	s.units = []Unit{u}
	if err := s.UpdateContext(ctx, true); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled empty discovery reported success", err)
	}
	if _, has, err := s.store.LoadUpdate(s.updateKey("pull:" + u.Name())); err != nil || has {
		t.Fatal("canceled empty discovery marked complete", has, err)
	}
}

func TestInstrumentDiscoveryUsesClientInterface(t *testing.T) {
	p, err := tdx.NewPool(func() (tdx.Client, error) { return &discoveryClient{}, nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	s := testService(t)
	s.cfg.ExPool = p
	rows, err := s.GetInstruments()
	if err != nil || len(rows) != 1 {
		t.Fatal("custom Client cannot supply instrument discovery", rows, err)
	}
}

type blockingDiscoveryClient struct {
	discoveryClient
	entered, release chan struct{}
	once             sync.Once
	calls            atomic.Int32
}

func (c *blockingDiscoveryClient) GetInstruments() ([]protocol.ExInstrument, error) {
	c.calls.Add(1)
	c.once.Do(func() { close(c.entered); <-c.release })
	return c.discoveryClient.GetInstruments()
}

type concurrentDiscoveryUnit struct{ mockUnit }

func (u *concurrentDiscoveryUnit) Codes(s *Service) ([]model.Code, error) {
	var tasks sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		tasks.Add(1)
		go func() {
			defer tasks.Done()
			rows, err := s.GetInstruments()
			if err == nil {
				if len(rows) != 1 || rows[0].Code != "AAPL" {
					err = errors.New("discovery snapshot was mutated")
				} else {
					rows[0].Code = "changed"
				}
			}
			errs <- err
		}()
	}
	tasks.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return []model.Code{{Market: "test", Code: "one"}}, nil
}

func TestConcurrentInstrumentDiscoverySharesOneSnapshot(t *testing.T) {
	c := &blockingDiscoveryClient{entered: make(chan struct{}), release: make(chan struct{})}
	pool, err := tdx.NewPool(func() (tdx.Client, error) { return c, nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := testService(t)
	s.cfg.ExPool = pool
	s.units = []Unit{&concurrentDiscoveryUnit{mockUnit: mockUnit{name: "test"}}}
	s.overrides = nil
	done := make(chan error, 1)
	go func() { done <- s.Update(true) }()
	var release sync.Once
	defer release.Do(func() { close(c.release) })
	select {
	case <-c.entered:
	case <-time.After(time.Second):
		t.Fatal("discovery did not start")
	}
	release.Do(func() { close(c.release) })
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("concurrent discovery did not finish")
	}
	if c.calls.Load() != 1 {
		t.Fatal("concurrent callers did not share discovery", c.calls.Load())
	}
	rows, err := s.GetInstruments()
	if err != nil || len(rows) != 1 || rows[0].Code != "AAPL" || c.calls.Load() != 2 {
		t.Fatal("standalone discovery reused or mutated round snapshot", rows, err, c.calls.Load())
	}
}

type instrumentDiscoveryUnit struct {
	mockUnit
	entered chan struct{}
}

func (u *instrumentDiscoveryUnit) Codes(s *Service) ([]model.Code, error) {
	close(u.entered)
	_, err := s.GetInstruments()
	return nil, err
}

func TestUpdateContextCancelsDuringStandaloneDiscovery(t *testing.T) {
	c := &blockingDiscoveryClient{entered: make(chan struct{}), release: make(chan struct{})}
	pool, err := tdx.NewPool(func() (tdx.Client, error) { return c, nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	s := testService(t)
	s.cfg.ExPool = pool
	s.overrides = nil
	u := &instrumentDiscoveryUnit{mockUnit: mockUnit{name: "test"}, entered: make(chan struct{})}
	s.units = []Unit{u}
	standalone := make(chan error, 1)
	go func() {
		rows, err := s.GetInstruments()
		if err == nil && len(rows) != 1 {
			err = errors.New("incomplete standalone discovery")
		}
		standalone <- err
	}()
	var once sync.Once
	t.Cleanup(func() {
		once.Do(func() { close(c.release) })
		select {
		case err := <-standalone:
			if err != nil {
				t.Error("standalone discovery was affected", err)
			}
		case <-time.After(time.Second):
			t.Error("standalone discovery did not finish")
		}
	})
	select {
	case <-c.entered:
	case <-time.After(time.Second):
		t.Fatal("standalone discovery did not start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.UpdateContext(ctx, true) }()
	select {
	case <-u.entered:
	case <-time.After(100 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("canceled update waits for standalone discovery")
		once.Do(func() { close(c.release) })
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("update did not finish")
		}
	}
}
