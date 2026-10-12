package pull

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
)

type instrumentTestUnit struct {
	mockUnit
	calls int
	empty bool
}

func (u *instrumentTestUnit) Codes(s *Service) ([]Code, error) {
	ins, err := s.GetInstruments()
	if err == nil && ((u.empty && len(ins) != 0) || (!u.empty && (len(ins) != 801 || ins[800].Code != "LAST"))) {
		return nil, errors.New("incomplete instrument snapshot")
	}
	// Callers cannot mutate another market's cached snapshot.
	if len(ins) > 0 {
		ins[800].Code = "changed"
	}
	u.calls++
	return nil, err
}

type failOnceInstrumentPool struct {
	tdx.IPool
	fail bool
}

func (p *failOnceInstrumentPool) Do(fn func(tdx.Client) error) error {
	if p.fail {
		p.fail = false
		return errors.New("instrument discovery failed")
	}
	return p.IPool.Do(fn)
}

func TestInstrumentsSharedOnlyWithinUpdate(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var pages atomic.Int32
	var empty atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		b, err := l.Accept()
		if err != nil {
			return
		}
		defer b.Close()
		for {
			header := make([]byte, 12)
			if _, err := io.ReadFull(b, header); err != nil {
				return
			}
			body := make([]byte, int(binary.LittleEndian.Uint16(header[6:8]))-2)
			if _, err := io.ReadFull(b, body); err != nil {
				return
			}
			if header[0] != 0x01 {
				t.Errorf("instrument discovery used wrong protocol prefix %x", header[0])
				return
			}
			if binary.LittleEndian.Uint32(header[1:5]) == 0 {
				continue
			}
			pages.Add(1)
			start := binary.LittleEndian.Uint32(body)
			n := 800
			if start == 800 {
				n = 1
			}
			if empty.Load() {
				n = 0
			}
			data := make([]byte, 6+n*64)
			binary.LittleEndian.PutUint16(data[4:6], uint16(n))
			if start == 800 && n > 0 {
				copy(data[11:], "LAST")
			}
			response := make([]byte, 16+len(data))
			copy(response, []byte{0xb1, 0xcb, 0x74, 0, 0x1c})
			copy(response[5:9], header[1:5])
			copy(response[10:12], header[10:12])
			binary.LittleEndian.PutUint16(response[12:14], uint16(len(data)))
			binary.LittleEndian.PutUint16(response[14:16], uint16(len(data)))
			copy(response[16:], data)
			if _, err := b.Write(response); err != nil {
				return
			}
		}
	}()
	c, err := tdx.DialExHq(l.Addr().String(), tdx.WithRedial(false), tdx.WithLevel(tdx.LevelNone), tdx.WithDebug(false))
	if err != nil {
		l.Close()
		<-done
		t.Fatal(err)
	}
	c.SetTimeout(time.Second)
	p, err := tdx.NewPool(func() (tdx.Client, error) { return c, nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close(); l.Close(); <-done })
	faulty := &failOnceInstrumentPool{IPool: p}
	s := testService(t)
	s.cfg.ExPool = faulty
	s.overrides = nil
	s.units = []Unit{
		&instrumentTestUnit{mockUnit: mockUnit{name: "hk"}},
		&instrumentTestUnit{mockUnit: mockUnit{name: "hk/index"}},
		&instrumentTestUnit{mockUnit: mockUnit{name: "us"}},
		&instrumentTestUnit{mockUnit: mockUnit{name: "future"}},
	}
	if err := s.Update(true); err != nil {
		t.Fatal(err)
	}
	if pages.Load() != 2 {
		t.Fatalf("4 markets scanned %d pages; want 2, old path required 8", pages.Load())
	}
	t.Log("same 801 records: 4 markets use 2 pages instead of 8")
	clearMarks := func() {
		if _, err := s.updatedDB.Exec("DELETE FROM `update`"); err != nil {
			t.Fatal(err)
		}
	}
	clearMarks()
	if err := s.Update(true); err != nil || pages.Load() != 4 {
		t.Fatal("new Update reused stale snapshot", err, pages.Load())
	}
	for i := 0; i < 2; i++ {
		if _, err := s.GetInstruments(); err != nil {
			t.Fatal(err)
		}
	}
	if pages.Load() != 8 {
		t.Fatal("standalone discovery reused update cache", pages.Load())
	}
	clearMarks()
	faulty.fail = true
	if err := s.Update(true); err == nil {
		t.Fatal("discovery failure swallowed")
	}
	if pages.Load() != 10 {
		t.Fatal("failed scan cached instead of retried by next market", pages.Load())
	}
	if err := s.Update(true); err != nil || pages.Load() != 12 {
		t.Fatal("failed market could not recover on next Update", err, pages.Load())
	}
	empty.Store(true)
	clearMarks()
	for _, unit := range s.units {
		unit.(*instrumentTestUnit).empty = true
	}
	if err := s.Update(true); err != nil || pages.Load() != 13 {
		t.Fatal("successful empty snapshot was repeatedly fetched", err, pages.Load())
	}

}
