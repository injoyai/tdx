package tdx

import (
	"context"
	"encoding/binary"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestHongKongSnapshotRouting(t *testing.T) {
	items := []protocol.ExInstrument{
		{Market: 31, Code: "00700"},
		{Market: 49, Code: "00823"},
		{Market: 31, Code: "01234"},
		{Market: 49, Code: "01234"},
		// This symbol is in an ETF code range but belongs to 31 in this fixture.
		{Market: 31, Code: "02800"},
	}
	var scans, quotes atomic.Int32
	native := respondingTestClient(t, true, func(header, body []byte) []byte {
		switch typ := binary.LittleEndian.Uint16(header[10:12]); typ {
		case protocol.TypeExCount:
			scans.Add(1)
			data := make([]byte, 23)
			binary.LittleEndian.PutUint32(data[19:23], uint32(len(items)))
			return data
		case protocol.TypeExInstrument:
			data := make([]byte, 6+64*len(items))
			binary.LittleEndian.PutUint16(data[4:6], uint16(len(items)))
			for j, item := range items {
				offset := 6 + 64*j
				data[offset+1] = item.Market
				copy(data[offset+5:offset+14], item.Code)
			}
			return data
		case protocol.TypeExQuote:
			quotes.Add(1)
			data := make([]byte, 150)
			copy(data, body)
			return data
		case protocol.TypeExBars:
			if body[0] != 49 || string(body[1:6]) != "00823" {
				t.Errorf("wrong K-line identity: %x", body[:10])
			}
			if binary.LittleEndian.Uint32(body[14:18]) > 0 {
				return make([]byte, 20)
			}
			return testExtendedBar(20261118)
		default:
			t.Errorf("unexpected message type %x", typ)
			return nil
		}
	})
	unified := newMarketClient()
	t.Cleanup(func() { unified.Close() })
	unified.dial[1] = func(context.Context) (protocolClient, error) {
		return native.protocolClient, nil
	}
	// Request views share one lazy snapshot even when the connection is cold.
	var workers sync.WaitGroup
	for n, input := range []string{"hk.00823", "HK00823", "00823.Hk", "00823", "hk/00823"} {
		workers.Go(func() {
			var client Client = native.protocolClient
			if n%2 == 0 {
				client = unified
			}
			view := WithContext(context.Background(), client)
			got, err := view.GetQuote(input)
			if err != nil || len(got) != 1 || got[0].Instrument.Exchange != protocol.ExchangeHKFund ||
				got[0].Extended.Market != 49 || got[0].Instrument.String() != "hk.00823" {
				t.Errorf("%s: %+v, %v", input, got, err)
			}
		})
	}
	workers.Wait()
	if scans.Load() != 1 || quotes.Load() != 5 {
		t.Fatalf("snapshot scans=%d quotes=%d", scans.Load(), quotes.Load())
	}
	bars, err := native.GetKlineDay("hk.00823", 0, 1)
	if err != nil || len(bars.List) != 1 || bars.Instrument.Exchange != protocol.ExchangeHKFund {
		t.Fatal(bars, err)
	}
	empty, err := native.GetHistoryTrade("20261114", "hk.00823", 0, 1)
	if err != nil || empty.Count != 0 || empty.Instrument.Exchange != protocol.ExchangeHKFund {
		t.Fatal(empty, err)
	}
	got, err := native.GetQuote("hk.02800", "02800.49")
	if err != nil || len(got) != 2 || got[0].Extended.Market != 31 || got[1].Extended.Market != 49 {
		t.Fatal(got, err)
	}
	for _, tc := range []struct{ input, want string }{
		{"hk.01234", "同时出现在通达信市场 31 和 49"},
		{"hk.00042", "未找到（候选市场 31、49）"},
	} {
		got, err := native.GetQuote(tc.input)
		if err == nil || !strings.Contains(err.Error(), tc.want) || got != nil {
			t.Fatalf("%s: %+v %v", tc.input, got, err)
		}
	}
	if scans.Load() != 1 || quotes.Load() != 7 {
		t.Fatalf("ambiguous or unknown code sent a request: scans=%d quotes=%d", scans.Load(), quotes.Load())
	}
}
