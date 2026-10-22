package tdx

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestCodeListsAndReports(t *testing.T) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, name := range []string{protocol.FileTdxBjMore, protocol.FileTdxZs, protocol.FileTdxBk, protocol.FileTdxStat, protocol.FileTdxStat2, protocol.FileXgsg, protocol.FileSpBlock} {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if name == protocol.FileTdxBjMore {
			if _, err = w.Write([]byte("44|920001|2|BJ|1|\n")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	exchanges := make(chan uint16, 10)
	var archive atomic.Value
	archive.Store(append([]byte(nil), buf.Bytes()...))
	c := respondingTestClient(t, false, func(h, b []byte) []byte {
		switch binary.LittleEndian.Uint16(h[10:]) {
		case protocol.TypeBlockInfo:
			raw := archive.Load().([]byte)
			return append(protocol.Bytes(uint32(len(raw))), raw...)
		case protocol.TypeCode:
			ex := binary.LittleEndian.Uint16(b)
			exchanges <- ex
			codes := []string{"600000", "510300", "000001", "880001"}
			if ex == 0 {
				codes = []string{"000001", "159001", "399001"}
			}
			data := make([]byte, 2+29*len(codes))
			binary.LittleEndian.PutUint16(data, uint16(len(codes)))
			for i, code := range codes {
				copy(data[2+29*i:], code)
			}
			return data
		default:
			t.Errorf("unexpected request %x", h)
			return nil
		}
	})
	for _, tc := range []struct {
		name string
		get  func() ([]string, error)
		want []string
	}{
		{"stocks", c.GetStockCodeAll, []string{"sh600000", "sz000001", "bj920001"}},
		{"etfs", c.GetETFCodeAll, []string{"sh510300", "sz159001"}},
		{"indexes", c.GetIndexCodeAll, []string{"bj899050", "sh000001", "sh880001", "sz399001"}},
	} {
		got, err := tc.get()
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Fatal(tc.name, got, err)
		}
		if got := []uint16{<-exchanges, <-exchanges}; !reflect.DeepEqual(got, []uint16{1, 0}) {
			t.Fatal("changed standard exchange range/order", got)
		}
		t.Logf("fixture %s: %v", tc.name, got)
	}
	for _, get := range []func() error{
		func() error { _, err := c.GetTdxZs(); return err }, func() error { _, err := c.GetTdxBk(); return err },
		func() error { _, err := c.GetTdxStat(); return err }, func() error { _, err := c.GetTdxStat2(); return err },
		func() error { _, err := c.GetXgsg(); return err }, func() error { _, err := c.GetSpBlock(); return err },
	} {
		if err := get(); err != nil {
			t.Fatal(err)
		}
	}
	buf.Reset()
	z = zip.NewWriter(&buf)
	if _, err := z.Create("unrelated"); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	archive.Store(append([]byte(nil), buf.Bytes()...))
	for _, tc := range []struct {
		name string
		get  func() error
	}{
		{protocol.FileTdxZs, func() error { _, e := c.GetTdxZs(); return e }},
		{protocol.FileTdxBk, func() error { _, e := c.GetTdxBk(); return e }},
		{protocol.FileTdxStat, func() error { _, e := c.GetTdxStat(); return e }},
		{protocol.FileTdxStat2, func() error { _, e := c.GetTdxStat2(); return e }},
		{protocol.FileXgsg, func() error { _, e := c.GetXgsg(); return e }},
		{protocol.FileSpBlock, func() error { _, e := c.GetSpBlock(); return e }},
	} {
		want := fmt.Sprintf("%s 中缺少 %s", protocol.ReportZHB, tc.name)
		if err := tc.get(); err == nil || err.Error() != want {
			t.Fatal(tc.name, err, want)
		}
	}
}
