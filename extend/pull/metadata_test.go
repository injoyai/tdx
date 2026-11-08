package pull

import (
	"context"
	"errors"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/metadata"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/storage/sqlite"
	"github.com/injoyai/tdx/v2/protocol"
)

type metadataClient struct {
	tdx.Client
	calls, closes int
	failExchange  protocol.Exchange
	fail          bool
	badDate       string
	badEvents     bool
	emptyCalendar bool
}

func (c *metadataClient) Close() error { c.closes++; return nil }
func (c *metadataClient) GetCodeAll(ex protocol.Exchange) (*tdx.CodeResp, error) {
	c.calls++
	if c.fail && ex == c.failExchange {
		return nil, errors.New("metadata source failed")
	}
	code := "600000"
	if ex == protocol.ExchangeSZ {
		code = "000001"
	}
	if ex == protocol.ExchangeBJ {
		return &tdx.CodeResp{}, nil
	}
	return &tdx.CodeResp{Count: 1, List: []tdx.Code{{Standard: &protocol.Code{Code: code, Name: "fixture", Decimal: 2}}}}, nil
}
func (c *metadataClient) GetGbbq(code string) (*protocol.GbbqResp, error) {
	c.calls++
	at := time.Date(2026, 1, 5, 15, 0, 0, 0, protocol.Location())
	if c.badEvents {
		code = "sz000001"
	}
	return &protocol.GbbqResp{Count: 2, List: []*protocol.Gbbq{{Code: code, Time: at, Category: 5, C3: 100, C4: 200}, {Code: code, Time: at.AddDate(0, 0, 1), Category: 5, C3: 300, C4: 400}}}, nil
}
func (c *metadataClient) GetKlineDayAll(string) (*tdx.KlineResp, error) {
	c.calls++
	if c.emptyCalendar {
		return &tdx.KlineResp{}, nil
	}
	rows := []tdx.Kline{{Time: "2026-01-05"}, {Time: "2026-01-06"}}
	if c.badDate != "" {
		rows = append(rows, tdx.Kline{Time: c.badDate})
	}
	return &tdx.KlineResp{Count: len(rows), List: rows}, nil
}

type metadataPool struct {
	tdx.ClientPool
	client *metadataClient
}

func (p *metadataPool) Do(fn func(tdx.Client) error) error { return fn(p.client) }

func TestMetadataSynchronizationAndPureQueries(t *testing.T) {
	backend, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	client := &metadataClient{}
	s, err := New(&Config{Pool: &metadataPool{client: client}, CodesStore: backend, GbbqStore: backend, WorkdayStore: backend, Retry: 1})
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 0 {
		t.Fatal("constructor performed network I/O")
	}
	ctx := context.Background()
	snapshot, err := s.SyncCodes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.NewCodes(snapshot.Rows).GetName("sh600000") != "fixture" {
		t.Fatal(snapshot)
	}
	before, err := backend.LoadCodes(ctx, snapshot.Exchanges)
	if err != nil {
		t.Fatal(err)
	}
	client.fail, client.failExchange = true, protocol.ExchangeSZ
	if _, err := s.SyncCodes(ctx); err == nil {
		t.Fatal("partial source failure accepted")
	}
	after, err := backend.LoadCodes(ctx, snapshot.Exchanges)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("partial response changed snapshot/state", after, err)
	}
	client.fail = false
	events, err := s.SyncGbbq(ctx, []string{"SH.600000", "sh600000"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events.Codes) != 1 || len(events.Rows) != 2 {
		t.Fatal(events)
	}
	query, err := metadata.NewGbbq(events.Rows)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 5, 0, 0, 0, 0, protocol.Location())
	if equity := query.GetEquity("sh600000", at); equity == nil || equity.Float != 100 {
		t.Fatal(equity)
	}
	if equity := query.GetEquity("sh600000", at.AddDate(0, 0, -1)); equity != nil {
		t.Fatal("future equity leaked into history")
	}
	client.badEvents = true
	if _, err := s.SyncGbbq(ctx, []string{"sh600000"}); err == nil {
		t.Fatal("wrong-security events accepted")
	}
	stored, err := backend.LoadGbbq(ctx, events.Codes)
	if err != nil || !reflect.DeepEqual(events, stored) {
		t.Fatal("bad events overwrote snapshot", stored, err)
	}
	calendar, err := s.SyncWorkdays(ctx)
	if err != nil {
		t.Fatal(err)
	}
	w, err := metadata.NewWorkday(calendar)
	if err != nil {
		t.Fatal(err)
	}
	if !w.Is(at.UTC()) {
		t.Fatal("source calendar changed timezone")
	}
	for _, bad := range []string{"20260106", "2026-02-30", "2026-01-06 15:00", "2026-1-06"} {
		client.badDate = bad
		if _, err := s.SyncWorkdays(ctx); err == nil {
			t.Fatal("bad calendar accepted", bad)
		}
		got, err := backend.LoadWorkdays(ctx, calendar.Market)
		if err != nil || !reflect.DeepEqual(got, calendar) {
			t.Fatal("calendar changed after invalid response", got, err)
		}
	}
	client.emptyCalendar = true
	if _, err := s.SyncWorkdays(ctx); err == nil {
		t.Fatal("empty response published an uncommitted calendar")
	}
	if got, err := backend.LoadWorkdays(ctx, calendar.Market); err != nil || !reflect.DeepEqual(got, calendar) {
		t.Fatal("empty response damaged calendar", got, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	calls := client.calls
	if _, err := s.SyncCodes(cancelled); !errors.Is(err, context.Canceled) || client.calls != calls {
		t.Fatal("cancelled task reached source", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SyncCodes(ctx); !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
	if client.closes != 0 {
		t.Fatal("closed borrowed client")
	}
	if _, err := backend.LoadCodes(ctx, snapshot.Exchanges); err != nil {
		t.Fatal("closed borrowed backend", err)
	}
}

func TestNewMetadataDependenciesDoNotCreateLocalDefaults(t *testing.T) {
	if _, err := New(&Config{}); err == nil {
		t.Fatal("accepted no storage")
	}
	for _, cfg := range []*Config{{CodesStore: (*sqlite.Metadata)(nil)}, {GbbqStore: (*sqlite.Metadata)(nil)}, {WorkdayStore: (*sqlite.Metadata)(nil)}} {
		if _, err := New(cfg); err == nil {
			t.Fatal("accepted typed nil metadata store")
		}
	}
}

type universeUnit struct{ name string }

func (u *universeUnit) Name() string                           { return u.name }
func (u *universeUnit) Codes(s *Service) ([]model.Code, error) { return s.DiscoverCodes() }
func (*universeUnit) FetchDay(*Service, model.Code) error      { return nil }
func (*universeUnit) FetchMinute(*Service, model.Code) error   { return nil }

func TestUniverseFixedWithinOneRun(t *testing.T) {
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	calls := 0
	list := []model.Code{{Market: protocol.MarketAStock, Code: "sh600000"}}
	backend := testStore(t.Name())
	s, err := New(&Config{Store: backend, Codes: []string{"test.one"}, Day: true, ListCodes: func(context.Context) ([]model.Code, error) { calls++; return list, nil }})
	if err != nil {
		t.Fatal(err)
	}
	s.units = []Unit{&universeUnit{name: "test"}, &universeUnit{name: "universe"}}
	s.overrides = nil
	if err := s.UpdateContext(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("universe changed during run", calls)
	}
	backend.Mu.Lock()
	backend.Marks.Marks = nil
	backend.Mu.Unlock()
	list = []model.Code{{Market: protocol.MarketAStock, Code: "sz000001"}}
	if err := s.UpdateContext(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("next run retained stale universe", calls)
	}
}

func TestMetadataCancellationWhileWaitingForPool(t *testing.T) {
	backend, err := sqlite.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	client := &metadataClient{}
	pool, err := tdx.NewPool(func() (tdx.Client, error) { return client, nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	borrowed, err := pool.Get()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Put(borrowed)
	s, err := New(&Config{Pool: pool, CodesStore: backend, Retry: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := s.SyncCodes(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("pool wait ignored cancellation", err)
	}
	if client.calls != 0 {
		t.Fatal("cancelled borrower reached source")
	}
}
