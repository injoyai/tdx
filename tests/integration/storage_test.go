package integration_test

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/codes"
	"github.com/injoyai/tdx/v2/extend/gbbq"
	"github.com/injoyai/tdx/v2/extend/workday"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

type memoryMarks struct {
	mu      sync.Mutex
	marks   map[string]int64
	err     error
	saveErr error
	closed  bool
}

func (s *memoryMarks) LoadUpdate(key string) (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.marks[key]
	return v, ok, s.err
}
func (s *memoryMarks) SaveUpdate(key string, unix int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	if s.marks == nil {
		s.marks = map[string]int64{}
	}
	s.marks[key] = unix
	return nil
}
func (s *memoryMarks) Close() error { s.closed = true; return nil }

type memoryCodes struct {
	memoryMarks
	rows []*codes.CodeModel
}

func (s *memoryCodes) Load() ([]*codes.CodeModel, error) { return s.rows, s.err }
func (s *memoryCodes) Save(rows []*codes.CodeModel) ([]*codes.CodeModel, error) {
	if s.saveErr != nil {
		return nil, s.saveErr
	}
	s.rows = rows
	return rows, nil
}

type memoryGbbq struct {
	memoryMarks
	rows []*protocol.Gbbq
}

func (s *memoryGbbq) Load() ([]*protocol.Gbbq, error) { return s.rows, s.err }
func (s *memoryGbbq) Save(rows []*protocol.Gbbq) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.rows = rows
	return nil
}

type memoryWorkdays struct {
	memoryMarks
	rows []*workday.WorkdayModel
}

func (s *memoryWorkdays) Load() ([]*workday.WorkdayModel, error) { return s.rows, s.err }
func (s *memoryWorkdays) Save(rows []*workday.WorkdayModel) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.rows = append(s.rows, rows...)
	return nil
}

var _ codes.Store = (*memoryCodes)(nil)
var _ gbbq.Store = (*memoryGbbq)(nil)
var _ workday.Store = (*memoryWorkdays)(nil)

type storeClient struct {
	metadataClient
	codeCalls, gbbqCalls int
}

func (c *storeClient) GetCodeAll(ex protocol.Exchange) (*tdx.CodeResp, error) {
	c.codeCalls++
	resp := &tdx.CodeResp{}
	if ex == protocol.ExchangeSH {
		resp.List = []tdx.Code{{Standard: &protocol.Code{Code: "600000", Name: "stored", Decimal: 2}}}
	}
	return resp, nil
}
func (c *storeClient) GetGbbqAll() (map[string][]*protocol.Gbbq, error) {
	c.gbbqCalls++
	return map[string][]*protocol.Gbbq{"sz000001": {{Code: "sz000001", Time: time.Date(2025, 1, 1, 0, 0, 0, 0, protocol.Location()), Category: 5, C3: 100, C4: 200}}}, nil
}

func TestMetadataCustomStores(t *testing.T) {
	c := &storeClient{metadataClient: metadataClient{days: protocol.Klines{{Time: time.Date(2025, 1, 6, 0, 0, 0, 0, protocol.Location())}}}}
	cs, gs, ws := &memoryCodes{}, &memoryGbbq{}, &memoryWorkdays{}
	codesCache, err := codes.New(codes.WithClient(c), codes.WithStore(cs), codes.WithSpec("@every 87600h"))
	if err != nil {
		t.Fatal(err)
	}
	defer codesCache.Close()
	gbbqCache, err := gbbq.New(gbbq.WithClient(c), gbbq.WithStore(gs), gbbq.WithSpec("@every 87600h"))
	if err != nil {
		t.Fatal(err)
	}
	defer gbbqCache.Close()
	calendar, err := workday.New(workday.WithClient(c), workday.WithStore(ws), workday.WithSpec("@every 87600h"))
	if err != nil {
		t.Fatal(err)
	}
	defer calendar.Close()
	if codesCache.GetName("sh600000") != "stored" || len(cs.rows) != 1 || len(gs.rows) != 1 || len(gbbqCache.All()["sz000001"]) != 1 || !calendar.Is(c.days[0].Time) || len(ws.rows) != 1 {
		t.Fatal("custom stores did not feed the public caches")
	}
	if err := codesCache.Update(); err != nil {
		t.Fatal(err)
	}
	if err := gbbqCache.Update(); err != nil {
		t.Fatal(err)
	}
	if c.codeCalls != 3 || c.gbbqCalls != 1 {
		t.Fatal("custom update marks not reused", c.codeCalls, c.gbbqCalls)
	}
	if err := calendar.Update(); err != nil || len(ws.rows) != 1 {
		t.Fatal("calendar appended duplicate dates", err)
	}
	for _, object := range []interface {
		Update() error
		Close() error
	}{codesCache, gbbqCache, calendar} {
		if err := object.Close(); err != nil {
			t.Fatal(err)
		}
		if err := object.Update(); !errors.Is(err, net.ErrClosed) {
			t.Fatal(err)
		}
	}
	if cs.closed || gs.closed || ws.closed || c.closes.Load() != 0 {
		t.Fatal("closed injected resource")
	}
}

func TestMetadataStoreFailures(t *testing.T) {
	want := errors.New("store unavailable")
	for _, phase := range []string{"load", "save"} {
		t.Run(phase, func(t *testing.T) {
			c := &storeClient{metadataClient: metadataClient{days: protocol.Klines{{Time: time.Date(2025, 1, 6, 0, 0, 0, 0, protocol.Location())}}}}
			cs, gs, ws := &memoryCodes{}, &memoryGbbq{}, &memoryWorkdays{}
			if phase == "load" {
				cs.err, gs.err, ws.err = want, want, want
			} else {
				cs.saveErr, gs.saveErr, ws.saveErr = want, want, want
			}
			_, err := codes.New(codes.WithClient(c), codes.WithStore(cs))
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
			_, err = gbbq.New(gbbq.WithClient(c), gbbq.WithStore(gs))
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
			_, err = workday.New(workday.WithClient(c), workday.WithStore(ws))
			if !errors.Is(err, want) {
				t.Fatal(err)
			}
			if cs.closed || gs.closed || ws.closed || len(cs.marks) != 0 || len(gs.marks) != 0 || len(cs.rows) != 0 || len(gs.rows) != 0 || len(ws.rows) != 0 {
				t.Fatal("failed initialization changed ownership, data or completion state")
			}
		})
	}
}

func TestStoreAndDBOptionsConflict(t *testing.T) {
	dial := func() (*xorms.Engine, error) { t.Fatal("opened database despite conflicting Store"); return nil, nil }
	if _, err := codes.New(codes.WithStore(&memoryCodes{}), codes.WithDialDB(dial)); err == nil {
		t.Fatal("codes accepted conflicting storage")
	}
	if _, err := gbbq.New(gbbq.WithStore(&memoryGbbq{}), gbbq.WithDialDB(dial)); err == nil {
		t.Fatal("gbbq accepted conflicting storage")
	}
	if _, err := workday.New(workday.WithStore(&memoryWorkdays{}), workday.WithDialDB(dial)); err == nil {
		t.Fatal("workday accepted conflicting storage")
	}
}
