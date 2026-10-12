package tdx

import (
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestProtocolClientConstruction(t *testing.T) {
	standard := respondingTestClient(t, false, func(_, _ []byte) []byte { return nil })
	extended := respondingTestClient(t, true, func(_, _ []byte) []byte { return nil })
	if _, err := NewStandardClient(nil); err == nil {
		t.Fatal("nil standard connection accepted")
	}
	if _, err := NewExtendedClient(nil); err == nil {
		t.Fatal("nil extended connection accepted")
	}
	if _, err := NewStandardClient(extended); err == nil {
		t.Fatal("extended connection accepted as standard")
	}
	if _, err := NewExtendedClient(standard); err == nil {
		t.Fatal("standard connection accepted as extended")
	}
	std, err := NewStandardClient(standard)
	if err != nil || std.Conn != standard {
		t.Fatal(std, err)
	}
	ex, err := NewExtendedClient(extended)
	if err != nil || ex.Conn != extended {
		t.Fatal(ex, err)
	}
	var wg sync.WaitGroup
	for j := 0; j < 8; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, _ := NewStandardClient(standard)
			b, _ := NewExtendedClient(extended)
			if a != std || b != ex {
				t.Error("connection created duplicate protocol implementations")
			}
		}()
	}
	wg.Wait()
}

func TestProtocolClientQueries(t *testing.T) {
	for _, extended := range []bool{false, true} {
		name, code, wrong := "standard", "sz000001", "hk00700"
		if extended {
			name, code, wrong = "extended", wrong, code
		}
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			var daily, history atomic.Int32
			native := respondingTestClient(t, extended, func(h, b []byte) []byte {
				requests.Add(1)
				typ := binary.LittleEndian.Uint16(h[10:])
				if h[0] != 0x0c && !extended || h[0] != 0x01 && extended {
					t.Errorf("wrong request prefix %x", h[0])
				}
				switch typ {
				case protocol.TypeQuote:
					return testQuoteResponse(b)
				case protocol.TypeExQuote:
					data := make([]byte, 150)
					copy(data, b)
					return data
				case protocol.TypeKline:
					daily.Add(1)
					return testStandardBar(false)
				case protocol.TypeExBars:
					daily.Add(1)
					return testExtendedBar(20261009)
				case protocol.TypeHistoryTrade:
					history.Add(1)
					return []byte{1, 0, 0, 0, 0, 0, 0x3a, 2, 0xa8, 0x0f, 1, 1, 0}
				case protocol.TypeExHistTrade:
					history.Add(1)
					data := make([]byte, 32)
					binary.LittleEndian.PutUint16(data[14:], 1)
					return data
				default:
					t.Errorf("unexpected request type %x", typ)
					return nil
				}
			})
			var api Client
			if extended {
				api, _ = NewExtendedClient(native)
			} else {
				api, _ = NewStandardClient(native)
			}
			quotes, err := api.GetQuote(code, code)
			if err != nil || len(quotes) != 2 || quotes[0].Instrument != quotes[1].Instrument {
				t.Fatal("quote order/duplicates changed", quotes, err)
			}
			bars, err := api.GetKlineDayAll(code)
			if err != nil || bars.Count != 1 {
				t.Fatal("shared pagination failed", bars, err)
			}
			before := daily.Load()
			trades, err := api.GetHistoryTradeDay("20261009", code)
			if err != nil || trades.Count != 1 || trades.List[0].Date != "20261009" || daily.Load()-before != 1 || history.Load() != 1 {
				t.Fatal("history check/date changed", trades, err, daily.Load(), history.Load())
			}
			beforeRequests := requests.Load()
			for _, call := range []func() error{
				func() error { _, err := api.GetQuote(wrong); return err },
				func() error { _, err := api.GetKlineDayAll(wrong); return err },
				func() error { _, err := api.GetHistoryMinute("20261009", wrong); return err },
				func() error { _, err := api.GetHistoryTrade("20261009", wrong, 0, 1); return err },
				func() error { _, err := api.GetKlineRange(wrong, "20261009", "20261009"); return err },
			} {
				if err := call(); err == nil {
					t.Fatal("other protocol accepted")
				}
			}
			if extended {
				if _, err := api.GetFinanceInfo(protocol.ExchangeSZ, "000001"); err == nil {
					t.Fatal("extended client accepted finance")
				}
				if _, err := api.GetReportFile("a"); err == nil {
					t.Fatal("extended client accepted standard report")
				}
			}
			if requests.Load() != beforeRequests {
				t.Fatal("unsupported query sent a packet")
			}
			if err := api.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := api.GetMarkets(); !errors.Is(err, net.ErrClosed) {
				t.Fatal("closed implementation remained usable", err)
			}
		})
	}
}

func TestMarketClientUsesProtocolImplementations(t *testing.T) {
	m := marketTestClient(t, func(_, _ []byte) []byte { return nil }, func(_, _ []byte) []byte { return nil })
	std, err := m.connection(protocol.ExchangeSH)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := std.(*StandardClient); !ok {
		t.Fatalf("standard route returned %T", std)
	}
	ex, err := m.connection(protocol.ExchangeHK)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ex.(*ExtendedClient); !ok {
		t.Fatalf("extended route returned %T", ex)
	}
	if std != m.links[0].protocolClient() || ex != m.links[1].protocolClient() {
		t.Fatal("facade did not reuse the connection implementation")
	}
}

func TestMarketClientRejectsWrongDialProtocol(t *testing.T) {
	for _, empty := range []bool{false, true} {
		m := newMarketClient()
		t.Cleanup(func() { m.Close() })
		wrong := respondingTestClient(t, true, func(_, _ []byte) []byte { return nil })
		m.dial[0] = func() (*Conn, error) {
			if empty {
				return nil, nil
			}
			return wrong, nil
		}
		if _, err := m.GetCount(protocol.ExchangeSZ); err == nil || m.links[0] != nil {
			t.Fatal("invalid protocol connection was published", err)
		}
		if !empty {
			select {
			case <-wrong.Done():
			default:
				t.Fatal("rejected connection leaked")
			}
		}
	}
}
