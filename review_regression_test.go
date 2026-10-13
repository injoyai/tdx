package tdx

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/base/maps"
	"github.com/injoyai/base/maps/wait"
	"github.com/injoyai/conv"
	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestReviewRegressionMetadataSnapshot(t *testing.T) {
	for _, mode := range []string{"refresh", "unchanged", "zero metadata", "bulk", "rollback"} {
		t.Run(mode, func(t *testing.T) {
			db, err := xorms.NewSqlite(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err = db.Sync2(new(CodeModel)); err != nil {
				t.Fatal(err)
			}
			old := []*CodeModel{{Exchange: "sh", Code: "600000", Name: "old", Decimal: 2, Multiple: 100, LastPrice: 10}, {Exchange: "sh", Code: "600001", Name: "delisted", Decimal: 2}}
			if _, err = db.Insert(old); err != nil {
				t.Fatal(err)
			}
			old = nil
			if err := db.Find(&old); err != nil {
				t.Fatal(err)
			}
			updated, err := NewUpdated(db, 9, 0)
			if err != nil {
				t.Fatal(err)
			}
			remote := []*protocol.Code{{Code: "600000", Name: "new", Decimal: 2}}
			switch mode {
			case "unchanged":
				remote = []*protocol.Code{{Code: "600000", Name: "old", Decimal: 2, Multiple: 100, LastPrice: 10}, {Code: "600001", Name: "delisted", Decimal: 2}}
			case "zero metadata":
				remote = []*protocol.Code{{Code: "600000", Name: "old"}}
			case "bulk":
				for i := 100; i < 205; i++ {
					remote = append(remote, &protocol.Code{Code: fmt.Sprintf("600%03d", i), Name: "added", Decimal: 2})
				}
			case "rollback":
				remote = append(remote, &protocol.Code{Code: "600002", Name: "reject"})
				if _, err := db.Exec("CREATE TRIGGER reject_new BEFORE INSERT ON codes WHEN NEW.code='600002' BEGIN SELECT RAISE(ABORT, 'reject'); END"); err != nil {
					t.Fatal(err)
				}
			}
			var z bytes.Buffer
			zw := zip.NewWriter(&z)
			if _, err = zw.Create(protocol.FileTdxBjMore); err != nil {
				t.Fatal(err)
			}
			if err = zw.Close(); err != nil {
				t.Fatal(err)
			}
			c := &conn{Client: client.New(nil), Wait: wait.New(time.Second), m: maps.NewSafe()}
			c.SetTimeout(time.Second)
			c.Client.MoreWriter = ios.NewMoreWrite(frameTestWriter(func(bs []byte) (int, error) {
				var r any
				if binary.LittleEndian.Uint16(bs[10:12]) == protocol.TypeCode {
					resp := &protocol.CodeResp{}
					if bs[12] == protocol.ExchangeSH.Uint8() {
						resp.Count = uint16(len(remote))
						resp.List = remote
					}
					r = resp
				} else {
					r = &protocol.BlockInfoResp{Data: z.Bytes()}
				}
				c.waitMu.Lock()
				c.Wait.Done(conv.String(binary.LittleEndian.Uint32(bs[1:5])), r)
				c.waitMu.Unlock()
				return len(bs), nil
			}))
			c.wrapWriter(c.Client)
			cs := &Codes{c: newStConn(c), db: db, updated: updated, updateKey: "codes", CodesBase: NewCodesBase()}

			cs.CodesBase.Update(old)
			err = cs.Update()
			if mode == "rollback" {
				if err == nil {
					t.Fatal("insert failure did not roll back")
				}
				var stored []*CodeModel
				if err := db.Find(&stored); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(stored, old) || cs.GetName("sh600000") != "old" || cs.Get("sh600001") == nil {
					t.Fatal("failure changed database or cache", stored)
				}
				if done, err := updated.Updated("codes"); done || err != nil {
					t.Fatal("failure marked refresh complete", done, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var stored []*CodeModel
			if err := db.Find(&stored); err != nil {
				t.Fatal(err)
			}
			if len(stored) != len(remote) || !reflect.DeepEqual(stored, []*CodeModel(cs.GetStocks())) {
				t.Fatal("cache differs from committed database", stored, cs.GetStocks())
			}
			if cs.Get("sh600000").ID != old[0].ID || cs.Get("sh600000").InDate != old[0].InDate {
				t.Fatal("refresh replaced existing identity")
			}
			if mode == "unchanged" && !reflect.DeepEqual(stored, old) {
				t.Fatal("unchanged refresh lost records", stored)
			}
			if mode == "zero metadata" {
				row := cs.Get("sh600000")
				if row.Decimal != 0 || row.Multiple != 0 || row.LastPrice != 0 {
					t.Fatal("zero metadata was not updated", row)
				}
			}
			if mode == "refresh" && (cs.Get("sh600001") != nil || cs.GetName("sh600000") != "new") {
				t.Fatal("delisted or renamed code remained stale")
			}
		})
	}
}

func TestReviewRegressionCodesSnapshotRace(t *testing.T) {
	cs := NewCodesBase()
	a := []*CodeModel{{Exchange: "sh", Code: "600000"}}
	b := []*CodeModel{{Exchange: "sz", Code: "000001"}}
	cs.Update(a)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			cs.Update(a)
			cs.Update(b)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = cs.GetStocks()
			_ = cs.GetETFs()
			_ = cs.GetIndexes()
			for code := range cs.Iter() {
				_ = cs.Get(code)
			}
		}
	}()
	wg.Wait()
}

func TestReviewRegressionManageFailureCleanup(t *testing.T) {
	want := errors.New("offline initialization failed")
	for _, tc := range []struct {
		name    string
		options []Option
	}{
		{"codes", []Option{WithDialCodes(func(Client) (ICodes, error) { return nil, want })}},
		{"workday", []Option{WithCodes(NewCodesBase()), WithDialWorkday(func(Client) (*Workday, error) { return nil, want })}},
		{"equity", []Option{WithCodes(NewCodesBase()), WithWorkday(&Workday{}), WithDialGbbq(func(Client) (IGbbq, error) { return nil, want })}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var closed atomic.Int32
			p, err := NewPool(func() (Client, error) { return poolTestClient(&closed), nil }, 1)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			options := append([]Option{WithDialPool(func() (IPool, error) { return p, nil })}, tc.options...)
			m, err := NewManage(options...)
			if m != nil || !errors.Is(err, want) {
				t.Fatal(m, err)
			}
			if closed.Load() != 1 {
				t.Fatalf("failed initialization closed %d connections; want 1", closed.Load())
			}
		})
	}
}

func TestReviewRegressionCanceledDial(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, test := range []struct {
		name    string
		factory func([]string) ios.DialFunc
	}{{"host", NewHostDial}, {"random", NewRandomDial}, {"range", NewRangeDial}} {
		t.Run(test.name, func(t *testing.T) {
			c, _, err := test.factory([]string{listener.Addr().String()})(ctx)
			if c != nil {
				defer c.Close()
			}
			if !errors.Is(err, context.Canceled) {
				t.Errorf("canceled context still established TCP connection: error=%v", err)
			}
		})
	}
}

func TestRangeDialCancelsRetryWait(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	start := time.Now()
	c, _, err := NewRangeDial([]string{addr, addr})(ctx)
	cancel()
	if c != nil {
		c.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatal("retry wait ignored context", err, time.Since(start))
	}
}

func TestManagePreservesExternalPoolOnFailure(t *testing.T) {
	var closed atomic.Int32
	p, err := NewPool(func() (Client, error) { return poolTestClient(&closed), nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	want := errors.New("offline codes initialization failed")
	if _, err := NewManage(WithPool(p), WithDialCodes(func(Client) (ICodes, error) { return nil, want })); !errors.Is(err, want) {
		t.Fatal(err)
	}
	if closed.Load() != 0 {
		t.Fatal("caller-owned pool closed")
	}
	c, err := p.Get()
	if err != nil {
		t.Fatal("caller-owned pool unusable", err)
	}
	p.Put(c)
}

func TestMalformedResponsesReachCaller(t *testing.T) {
	c := respondingTestClient(t, false, func(h, _ []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:12]) == protocol.TypeQuote {
			return []byte{0, 0, 1, 0}
		}
		return []byte{1, 0}
	})
	for _, f := range []*protocol.Frame{protocol.MCode.Frame(protocol.ExchangeSH, 0), {Control: protocol.Control01, Type: protocol.TypeKline, Data: make([]byte, 28)}, {Control: protocol.Control01, Type: protocol.TypeQuote}} {
		start := time.Now()
		if _, err := c.SendFrame(f, protocol.KlineCache{Type: protocol.TypeKlineDay}); err == nil || time.Since(start) >= 500*time.Millisecond {
			t.Fatal("malformed response was swallowed until timeout", err)
		}
	}
}
