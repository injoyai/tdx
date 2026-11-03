package pull

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

// probeStore records the service boundary without a database or filesystem.
type probeStore struct {
	identity         string
	marks            map[string]int64
	code             Code
	year             int
	from, start, end string
	days             []*KlineDay
	minutes          []*KlineMinute
	coverage         *MinuteCoverage
	scan             *MinuteEmptyScan
	err              error
	closed           bool
	writes           int
}

func (p *probeStore) Identity() string { return p.identity }
func (p *probeStore) Close() error     { p.closed = true; return nil }
func (p *probeStore) LoadUpdate(key string) (int64, bool, error) {
	v, ok := p.marks[key]
	return v, ok, p.err
}
func (p *probeStore) SaveUpdate(key string, unix int64) error {
	if p.err != nil {
		return p.err
	}
	if p.marks == nil {
		p.marks = map[string]int64{}
	}
	p.marks[key] = unix
	return nil
}
func (p *probeStore) LastDayTime(code Code) (string, error) {
	p.code = code
	if len(p.days) == 0 {
		return "", p.err
	}
	return p.days[len(p.days)-1].Time, p.err
}
func (p *probeStore) LastMinuteTime(code Code, year int) (string, error) {
	p.code, p.year = code, year
	if len(p.minutes) == 0 {
		return "", p.err
	}
	return p.minutes[len(p.minutes)-1].Time, p.err
}
func (p *probeStore) QueryDay(code Code, start, end string) ([]*KlineDay, error) {
	p.code, p.start, p.end = code, start, end
	return p.days, p.err
}
func (p *probeStore) QueryMinute(code Code, start, end string) ([]*KlineMinute, error) {
	p.code, p.start, p.end = code, start, end
	return p.minutes, p.err
}
func (p *probeStore) SaveDay(code Code, from string, rows []*KlineDay) error {
	if p.err != nil {
		return p.err
	}
	p.writes++
	p.code, p.from, p.days = code, from, rows
	return nil
}
func (p *probeStore) SaveMinute(code Code, year int, from string, rows []*KlineMinute, coverage *MinuteCoverage) error {
	if p.err != nil {
		return p.err
	}
	p.writes++
	p.code, p.year, p.from, p.minutes, p.coverage = code, year, from, rows, coverage
	return nil
}
func (p *probeStore) QueryMinuteCoverage(code Code, year int) (*MinuteCoverage, error) {
	p.code, p.year = code, year
	return p.coverage, p.err
}
func (p *probeStore) LoadMinuteEmptyScan(code Code, year int) (*MinuteEmptyScan, error) {
	p.code, p.year = code, year
	return p.scan, p.err
}
func (p *probeStore) SaveMinuteEmptyScan(code Code, year int, scan MinuteEmptyScan) error {
	if p.err != nil {
		return p.err
	}
	p.code, p.year, p.scan = code, year, &scan
	return nil
}

func customService(t *testing.T, p Store, dir string) *Service {
	t.Helper()
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	s, err := New(&Config{Store: p, Dir: dir, Codes: []string{"test.one"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCustomStoreNoLocalFilesAndOwnership(t *testing.T) {
	p := &probeStore{identity: "dataset-a"}
	dir := filepath.Join(t.TempDir(), "must-not-be-created")
	s := customService(t, p, dir)
	c := Code{Market: protocol.MarketAStock, Code: "sz000001"}
	if err := s.SaveDay(c, "2025-01-01", []*KlineDay{{Time: "2025-01-02", Close: 3}}); err != nil {
		t.Fatal(err)
	}
	if p.code != c || p.from != "2025-01-01" {
		t.Fatal("lost security or overwrite boundary")
	}
	if last, err := s.LastDayTime(c); err != nil || last != "2025-01-02" {
		t.Fatal(last, err)
	}
	if rows, err := s.QueryDay(c, "2025-01-01", "2025-01-03"); err != nil || len(rows) != 1 || p.start != "2025-01-01" || p.end != "2025-01-03" {
		t.Fatal(rows, err)
	}
	rows := []*KlineMinute{{Time: "2025-01-02 09:32"}, {Time: "2025-01-02 09:31"}}
	if err := s.SaveMinuteComplete(c, 2025, "2025-01-01 00:00", rows); err != nil {
		t.Fatal(err)
	}
	if p.code != c || p.year != 2025 || p.from != "2025-01-02 09:31" || p.coverage == nil || p.coverage.From != "2025-01-01 00:00" {
		t.Fatal("data and coverage were not forwarded in one write", p)
	}
	if done, err := s.MinuteYearComplete(c, 2025, "2025-01-01 00:00"); err != nil || !done {
		t.Fatal(done, err)
	}
	if _, err := s.LastMinuteTime(c, 2025); err != nil {
		t.Fatal(err)
	}
	if got, err := s.QueryMinute(c, "", ""); err != nil || len(got) != 2 {
		t.Fatal(got, err)
	}
	now := time.Now()
	if err := s.SaveMinuteEmptyScan(c, 2024, "2024-01-01 00:00", now); err != nil {
		t.Fatal(err)
	}
	if empty, err := s.MinuteYearRecentlyEmpty(c, 2024, "2024-01-01 00:00", now); err != nil || !empty {
		t.Fatal(empty, err)
	}
	if err := s.updated.Mark(s.updateKey("pull:test")); err != nil {
		t.Fatal(err)
	}
	if len(p.marks) != 1 {
		t.Fatal("update marker bypassed custom store")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("custom store created local files", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if p.closed {
		t.Fatal("closed injected store")
	}
	if err := s.SaveDay(c, "", []*KlineDay{{Time: "2025-01-01"}}); !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
}

func TestCustomStoreValidationFailureAndNamespace(t *testing.T) {
	p := &probeStore{identity: "dataset-a"}
	s := customService(t, p, "") // Dir is optional for custom storage.
	c := Code{Market: protocol.MarketAStock, Code: "sz000001"}
	if err := s.SaveDay(c, "", []*KlineDay{{Time: "bad"}}); err == nil {
		t.Fatal("invalid source date reached store")
	}
	if err := s.SaveMinute(c, 2024, "", []*KlineMinute{{Time: "2025-01-01 09:31"}}); err == nil {
		t.Fatal("wrong source year reached store")
	}
	if err := s.SaveMinuteComplete(c, 2024, "", nil); err != nil || p.writes != 0 {
		t.Fatal("empty or invalid write reached store", err)
	}
	other := customService(t, &probeStore{identity: "dataset-b"}, "")
	if s.updateKey("pull:test") == other.updateKey("pull:test") {
		t.Fatal("different stores share update namespace")
	}
	want := errors.New("write failed")
	p.err = want
	if err := s.SaveMinuteFallbackComplete(c, 2025, "2025-01-01 00:00", "2025-01-03 00:00", []*KlineMinute{{Time: "2025-01-02 09:31"}}); !errors.Is(err, want) {
		t.Fatal(err)
	}
	if p.coverage != nil || p.writes != 0 {
		t.Fatal("failure published coverage")
	}
	if _, err := s.QueryMinuteCoverage(c, 2025); !errors.Is(err, want) {
		t.Fatal(err)
	}
	if err := s.Update(true); !errors.Is(err, want) {
		t.Fatal("marker error swallowed", err)
	}
	if len(p.marks) != 0 {
		t.Fatal("failed store marked complete")
	}
	if _, err := New(&Config{Store: &probeStore{}, Codes: []string{"test.one"}}); err == nil {
		t.Fatal("empty storage identity accepted")
	}
}
