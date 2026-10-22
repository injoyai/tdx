package tdx

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestHistoryTradeRequiresDailyKline(t *testing.T) {
	for _, extended := range []bool{false, true} {
		for _, wholeDay := range []bool{false, true} {
			for _, state := range []string{"missing", "older-date", "newer-date", "present", "error"} {
				name := map[bool]string{false: "standard", true: "extended"}[extended] + "/" + map[bool]string{false: "page", true: "day"}[wholeDay] + "/" + state
				t.Run(name, func(t *testing.T) {
					var daily, trades atomic.Int32
					c := respondingTestClient(t, extended, func(h, b []byte) []byte {
						typ := binary.LittleEndian.Uint16(h[10:])
						if typ == protocol.TypeKline || typ == protocol.TypeExBars {
							daily.Add(1)
							if state == "error" {
								return nil
							}
							day := uint32(20261009)
							if state == "older-date" {
								day = 20261008
							}
							if state == "newer-date" {
								day = 20261010
							}
							if extended {
								if binary.LittleEndian.Uint16(b[10:]) != 4 {
									t.Error("incorrect extended daily category")
								}
								if state == "missing" {
									return make([]byte, 20)
								}
								return testExtendedBar(day)
							}
							if b[8] != protocol.TypeKlineDay {
								t.Error("incorrect standard daily category")
							}
							if state == "missing" {
								return []byte{0, 0}
							}
							return completionStandardBars([]uint32{day}, nil, false)
						}
						trades.Add(1)
						if extended {
							r := make([]byte, 32)
							binary.LittleEndian.PutUint16(r[14:], 1)
							binary.LittleEndian.PutUint16(r[16:], 570)
							return r
						}
						return []byte{1, 0, 0, 0, 0, 0, 0x3a, 2, 0xa8, 0x0f, 1, 0, 0}
					})
					code := "sz000001"
					if extended {
						code = "hk00700"
					}
					var r *TradeResp
					var err error
					if wholeDay {
						r, err = c.GetHistoryTradeDay("20261009", code)
					} else {
						r, err = c.GetHistoryTrade("20261009", code, 100, 1)
					}
					if state == "error" {
						if r != nil || err == nil || extended && !errors.Is(err, io.ErrUnexpectedEOF) || daily.Load() != 1 || trades.Load() != 0 {
							t.Fatal("daily error must prevent trade request", r, err, trades.Load())
						}
						return
					}
					want := 0
					if state == "present" {
						want = 1
					}
					instrument, _ := protocol.ParseInstrument(code)
					if err != nil || r == nil || r.Count != want || len(r.List) != want || r.Date != "20261009" || r.Instrument != instrument || r.VolumeUnit != VolumeUnknown || daily.Load() != 1 || trades.Load() != int32(want) {
						t.Fatal("daily guard failed", r, err, daily.Load(), trades.Load())
					}
				})
			}
		}
	}
}

func TestHistoryTradeDailyPagination(t *testing.T) {
	var daily, trades atomic.Int32
	c := respondingTestClient(t, false, func(h, b []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeKline {
			daily.Add(1)
			if binary.LittleEndian.Uint16(b[12:]) == 0 {
				days := make([]uint32, 800)
				for j := range days {
					days[j] = 20261010
				}
				return completionStandardBars(days, nil, false)
			}
			return completionStandardBars([]uint32{20261008, 20261009}, nil, false)
		}
		trades.Add(1)
		return []byte{1, 0, 0, 0, 0, 0, 0x3a, 2, 0xa8, 0x0f, 1, 0, 0}
	})
	r, err := c.GetHistoryTradeDay("20261009", "sz000001")
	if err != nil || r.Count != 1 || daily.Load() != 2 || trades.Load() != 1 {
		t.Fatal("daily pagination missed requested date", r, err, daily.Load(), trades.Load())
	}
}

func TestTradePaginationOverflow(t *testing.T) {
	for _, historical := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "historical"}[historical], func(t *testing.T) {
			var previous uint16
			c := respondingTestClient(t, false, func(h []byte, b []byte) []byte {
				if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeKline {
					return completionStandardBars([]uint32{20250102}, nil, false)
				}
				pos := 8
				if historical {
					pos = 12
				}
				start := binary.LittleEndian.Uint16(b[pos : pos+2])
				count := binary.LittleEndian.Uint16(b[pos+2 : pos+4])
				if start < previous {
					t.Error("pagination wrapped", previous, start)
				}
				previous = start
				out := protocol.Bytes(count)
				if historical {
					out = append(out, 0, 0, 0, 0)
				}
				for i := uint16(0); i < count; i++ {
					out = append(out, 0x3a, 2, 0xa8, 0x0f, 1, 0, 0)
					if !historical {
						out = append(out, 0)
					}
				}
				return out
			})
			var r *TradeResp
			var err error
			if historical {
				r, err = c.GetHistoryTradeDay("20250102", "sz000001")
			} else {
				r, err = c.GetTradeAll("sz000001")
			}
			if err == nil || !strings.Contains(err.Error(), "uint16") || r != nil {
				t.Fatal("expected pagination limit error", r, err)
			}
		})
	}
}

func TestOtherPaginationLimits(t *testing.T) {
	for _, mode := range []string{"code", "stock", "index", "until", "extended"} {
		t.Run(mode, func(t *testing.T) {
			c := respondingTestClient(t, mode == "extended", func(h, _ []byte) []byte {
				if mode == "extended" {
					sample := testExtendedBar(20250102)
					out := append([]byte(nil), sample[:20]...)
					binary.LittleEndian.PutUint16(out[18:], 800)
					for j := 0; j < 800; j++ {
						out = append(out, sample[20:]...)
					}
					return out
				}
				if binary.LittleEndian.Uint16(h[10:12]) == protocol.TypeCode {
					return append(protocol.Bytes(uint16(1000)), make([]byte, 1000*29)...)
				}
				record := make([]byte, 16)
				binary.LittleEndian.PutUint32(record, 20250102)
				if mode == "index" {
					record = append(record, 0, 0, 0, 0)
				}
				out := protocol.Bytes(uint16(800))
				for i := 0; i < 800; i++ {
					out = append(out, record...)
				}
				return out
			})
			var err error
			switch mode {
			case "extended":
				_, err = c.GetKlineAll(protocol.TypeKlineDay, "hk00700")
			case "code":
				_, err = c.GetCodeAll(protocol.ExchangeSZ)
			case "index":
				_, err = c.GetKlineAll(protocol.TypeKlineDay, "sh000001")
			case "stock":
				_, err = c.GetKlineAll(protocol.TypeKlineDay, "sz000001")
			case "until":
				seen := 0
				r, e := c.GetKlineUntil(protocol.TypeKlineDay, "sz000001", func(Kline) bool {
					seen++
					return seen == 65535
				})
				if e != nil || r.Count != 65535 || len(r.List) != 65535 {
					t.Fatal("valid boundary stop failed", e)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "uint16") {
				t.Fatal("expected pagination limit error", err)
			}
		})
	}
}

func completionStandardBars(days []uint32, minutes []uint16, index bool) []byte {
	b := protocol.Bytes(uint16(len(days)))
	for j, day := range days {
		if minutes == nil {
			b = append(b, protocol.Bytes(day)...)
		} else {
			year, month, date := int(day)/10000, int(day)/100%100, int(day)%100
			b = append(b, protocol.Bytes(uint16((year-2004)*2048+month*100+date))...)
			b = append(b, protocol.Bytes(minutes[j])...)
		}
		if j == 0 {
			b = append(b, 0x90, 0x9c, 1)
		} else {
			b = append(b, 0)
		}
		b = append(b, 0, 0, 0)
		b = append(b, protocol.Bytes(math.Float32bits(100))...)
		b = append(b, protocol.Bytes(math.Float32bits(1000))...)
		if index {
			b = append(b, 3, 0, 2, 0)
		}
	}
	return b
}

func completionRange() []byte {
	b := make([]byte, 14+32)
	binary.LittleEndian.PutUint16(b[12:], 1)
	binary.LittleEndian.PutUint16(b[14:], uint16((2026-2004)*2048+1000+9))
	binary.LittleEndian.PutUint16(b[16:], 571)
	for p := 18; p < 34; p += 4 {
		binary.LittleEndian.PutUint32(b[p:], math.Float32bits(10))
	}
	binary.LittleEndian.PutUint32(b[34:], 123)
	binary.LittleEndian.PutUint32(b[38:], 456)
	binary.LittleEndian.PutUint32(b[42:], math.Float32bits(9))
	return b
}

func TestUnifiedConvenienceAndRange(t *testing.T) {
	for _, extended := range []bool{false, true} {
		t.Run(map[bool]string{false: "standard", true: "extended"}[extended], func(t *testing.T) {
			var sent atomic.Int32
			code := "sh000001"
			if extended {
				code = "hk00700"
			}
			c := respondingTestClient(t, extended, func(h, b []byte) []byte {
				sent.Add(1)
				typ := binary.LittleEndian.Uint16(h[10:])
				if typ == protocol.TypeExBarsRange {
					if b[10] != 7 || binary.LittleEndian.Uint32(b[12:]) != 20261009 || binary.LittleEndian.Uint32(b[16:]) != 20261009 {
						t.Error("range protocol changed", b)
					}
					return completionRange()
				}
				if typ == protocol.TypeExBars {
					return testExtendedBar(20261009)
				}
				if typ != protocol.TypeKline {
					t.Error("unexpected query", h)
					return nil
				}
				index := b[0] == 1
				if b[8] == protocol.TypeKlineMinute {
					return completionStandardBars([]uint32{20261008, 20261009, 20261009, 20261010}, []uint16{900, 571, 900, 571}, index)
				}
				if b[8] <= protocol.TypeKline60Minute {
					return completionStandardBars([]uint32{20261009}, []uint16{571}, index)
				}
				return testStandardBar(index)
			})
			for _, fn := range []func(string, uint16, uint16) (*KlineResp, error){c.GetKline5Minute, c.GetKline15Minute, c.GetKline30Minute, c.GetKline60Minute, c.GetKlineDay, c.GetKlineWeek, c.GetKlineMonth, c.GetKlineQuarter, c.GetKlineYear} {
				r, err := fn(code, 0, 1)
				if err != nil || r.Count != 1 {
					t.Fatal(r, err)
				}
				if !extended && (r.List[0].Standard.UpCount != 3 || r.List[0].Volume != r.List[0].Standard.Volume) {
					t.Fatal("index decoder was not selected", r)
				}
			}
			for _, fn := range []func(string) (*KlineResp, error){c.GetKline5MinuteAll, c.GetKline15MinuteAll, c.GetKline30MinuteAll, c.GetKline60MinuteAll, c.GetKlineDayAll, c.GetKlineWeekAll, c.GetKlineMonthAll, c.GetKlineQuarterAll, c.GetKlineYearAll} {
				r, err := fn(code)
				if err != nil || r.Count != 1 {
					t.Fatal(r, err)
				}
			}
			r, err := c.GetKlineRange(code, "20261009", "20261009")
			want := 2
			if extended {
				want = 1
			}
			if err != nil || r.Count != want || r.List[0].Time != "2026-10-09 09:31" {
				t.Fatal(r, err)
			}
			if extended {
				if r.List[0].RangeExtended.Position != 123 || r.List[0].RangeExtended.SettlementPrice != 9 || r.List[0].Amount != nil {
					t.Fatal("range fields lost", r)
				}
				if _, err = r.StandardResponse(); err == nil {
					t.Fatal("extended result converted to standard")
				}
			} else {
				raw, err := r.StandardResponse()
				if err != nil || raw.List[0].UpCount != 3 {
					t.Fatal(raw, err)
				}
			}
			before := sent.Load()
			for _, dates := range [][2]string{{"20261010", "20261009"}, {"20260230", "20261009"}} {
				if _, err = c.GetKlineRange(code, dates[0], dates[1]); err == nil {
					t.Fatal("invalid range accepted")
				}
			}
			if _, err = c.GetKlineUntil(9, code, nil); err == nil {
				t.Fatal("nil stop accepted")
			}
			wrong := "hk00700"
			if extended {
				wrong = "sz000001"
			}
			if _, err = c.GetKlineRange(wrong, "20261009", "20261009"); err == nil {
				t.Fatal("wrong protocol accepted")
			}
			if sent.Load() != before {
				t.Fatal("invalid query sent frame")
			}
		})
	}
	// The bound common methods must have identical callable contracts.
	c, m := new(StConn), new(MarketClient)
	ct, mt := reflect.TypeOf(c), reflect.TypeOf(m)
	for n := 0; n < ct.NumMethod(); n++ {
		cm := ct.Method(n)
		mm, ok := mt.MethodByName(cm.Name)
		if !ok || len(cm.Name) < 8 || cm.Name[:8] != "GetKline" {
			continue
		}
		a, b := cm.Type, mm.Type
		if a.NumIn() != b.NumIn() || a.NumOut() != b.NumOut() {
			t.Fatal(cm.Name)
		}
		for j := 1; j < a.NumIn(); j++ {
			if a.In(j) != b.In(j) {
				t.Fatal("input differs", cm.Name)
			}
		}
		for j := 0; j < a.NumOut(); j++ {
			if a.Out(j) != b.Out(j) {
				t.Fatal("output differs", cm.Name)
			}
		}
	}
}

func TestUnifiedKlinePaginationLast(t *testing.T) {
	c := respondingTestClient(t, false, func(_, b []byte) []byte {
		start := binary.LittleEndian.Uint16(b[12:])
		n := 800
		if start != 0 {
			n = 1
		}
		days := make([]uint32, n)
		for j := range days {
			days[j] = 20261009
		}
		return completionStandardBars(days, nil, false)
	})
	r, err := c.GetKlineDayAll("sz000001")
	if err != nil || r.Count != 801 || r.List[1].Standard.Last != 10000 {
		t.Fatal("cross-page previous close lost", err)
	}
}

func TestUnifiedTradePaging(t *testing.T) {
	for _, extended := range []bool{false, true} {
		for _, historical := range []bool{false, true} {
			t.Run(map[bool]string{false: "standard", true: "extended"}[extended]+map[bool]string{false: "today", true: "history"}[historical], func(t *testing.T) {
				var calls atomic.Int32
				var daily atomic.Int32
				c := respondingTestClient(t, extended, func(h, b []byte) []byte {
					if typ := binary.LittleEndian.Uint16(h[10:]); typ == protocol.TypeKline || typ == protocol.TypeExBars {
						daily.Add(1)
						if extended {
							return testExtendedBar(20261009)
						}
						return testStandardBar(false)
					}
					calls.Add(1)
					offset := 8
					if historical {
						offset = 12
					}
					if extended {
						offset = 10
						if historical {
							offset = 14
						}
					}
					start := binary.LittleEndian.Uint16(b[offset:])
					count := binary.LittleEndian.Uint16(b[offset+2:])
					if extended {
						count = binary.LittleEndian.Uint16(b[offset+4:])
					}
					n := int(count)
					if start != 0 {
						n = 1
					}
					minute := uint16(600)
					if start != 0 {
						minute = 570
					}
					if extended {
						r := make([]byte, 16+n*16)
						binary.LittleEndian.PutUint16(r[14:], uint16(n))
						for j := 0; j < n; j++ {
							p := 16 + 16*j
							binary.LittleEndian.PutUint16(r[p:], minute)
							binary.LittleEndian.PutUint32(r[p+2:], 500000)
							binary.LittleEndian.PutUint32(r[p+6:], 123)
						}
						return r
					}
					r := protocol.Bytes(uint16(n))
					if historical {
						r = append(r, 0, 0, 0, 0)
					}
					for j := 0; j < n; j++ {
						r = append(r, protocol.Bytes(minute)...)
						if j == 0 {
							r = append(r, 0xa8, 0x0f)
						} else {
							r = append(r, 0)
						}
						r = append(r, 1)
						if !historical {
							r = append(r, 3)
						}
						r = append(r, 0, 0)
					}
					return r
				})
				code := "sz000001"
				if extended {
					code = "hk00700"
				}
				var r *TradeResp
				var err error
				if historical {
					r, err = c.GetHistoryTradeDay("20261009", code)
				} else {
					r, err = c.GetTradeAll(code)
				}
				n := 1801
				if historical {
					n = 2001
				}
				if err != nil || r.Count != n || calls.Load() != 2 || r.List[0].Time != "09:30:00" || r.List[n-1].Time != "10:00:00" {
					t.Fatal("paging order changed", r, err)
				}
				if historical && (r.List[0].Date != "20261009" || r.List[n-1].Date != "20261009") {
					t.Fatal("history date lost")
				}
				wantDaily := int32(0)
				if historical {
					wantDaily = 1
				}
				if daily.Load() != wantDaily {
					t.Fatal("daily guard repeated for each trade page", daily.Load())
				}
				if extended && (r.List[0].Price != nil || r.List[0].Extended.Price != 500000) {
					t.Fatal("extended scale changed")
				}
			})
		}
	}
	c := newMarketClient()
	defer c.Close()
	c.dial[0] = func() (protocolClient, error) { return nil, io.ErrClosedPipe }
	if r, err := c.GetTradeAll("sz000001"); r != nil || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("partial result returned", r, err)
	}
}

type completionCalendar struct{ days []time.Time }

func (c completionCalendar) Range(start, end time.Time, visit func(time.Time) bool) {
	for _, d := range c.days {
		if !d.Before(start) && d.Before(end) && !visit(d) {
			return
		}
	}
}

func TestUnifiedHistoryCalendar(t *testing.T) {
	var daily atomic.Int32
	c := respondingTestClient(t, true, func(h, b []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeExBars {
			if binary.LittleEndian.Uint16(b[10:]) == 4 {
				daily.Add(1)
				r := testExtendedBar(20261008)
				r = append(r, testExtendedBar(20261009)[20:]...)
				binary.LittleEndian.PutUint16(r[18:], 2)
				return r
			}
			return testExtendedBar(20261001)
		}
		r := make([]byte, 32)
		binary.LittleEndian.PutUint16(r[14:], 1)
		binary.LittleEndian.PutUint16(r[16:], 570)
		binary.LittleEndian.PutUint32(r[18:], 500000)
		if d := binary.LittleEndian.Uint32(b[:4]); d != 20261008 && d != 20261009 {
			t.Error("wrong calendar day", d)
		}
		return r
	})
	days := []time.Time{time.Date(2026, 10, 8, 0, 0, 0, 0, time.FixedZone("caller", 8*3600)), time.Date(2026, 10, 9, 0, 0, 0, 0, time.FixedZone("caller", 8*3600))}
	calendar := completionCalendar{append([]time.Time{days[0].AddDate(0, 0, -1)}, days...)}
	r, err := c.GetHistoryTradeBefore("hk00700", calendar, days[1].AddDate(0, 0, 1))
	if err != nil || r.Count != 2 || r.List[0].Date != "20261008" || r.List[1].Date != "20261009" {
		t.Fatal(r, err)
	}
	if daily.Load() != 1 {
		t.Fatal("repeated daily guard", daily.Load())
	}
	if _, err = c.GetHistoryTradeFull("hk00700", nil); err == nil {
		t.Fatal("nil calendar accepted")
	}
	failed := respondingTestClient(t, true, func(h, b []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeExBars {
			if binary.LittleEndian.Uint16(b[10:]) == 4 {
				r := testExtendedBar(20261008)
				r = append(r, testExtendedBar(20261009)[20:]...)
				binary.LittleEndian.PutUint16(r[18:], 2)
				return r
			}
			return testExtendedBar(20261001)
		}
		if binary.LittleEndian.Uint32(b) == 20261009 {
			return nil
		}
		r := make([]byte, 32)
		binary.LittleEndian.PutUint16(r[14:], 1)
		return r
	})
	if r, err = failed.GetHistoryTradeBefore("hk00700", calendar, days[1].AddDate(0, 0, 1)); r != nil || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("failed day returned partial history", r, err)
	}
}

func TestUnifiedAuctionMinute(t *testing.T) {
	c := respondingTestClient(t, false, func(h, b []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeKline {
			if b[8] == protocol.TypeKlineDay {
				return testStandardBar(false)
			}
			return completionStandardBars([]uint32{20261009}, []uint16{571}, false)
		}
		return []byte{1, 0, 0, 0, 0, 0, 0x35, 2, 0xa8, 0x0f, 1, 0, 0} // 历史9:25成交1手、10元。
	})
	r, err := c.GetKlineMinute241Until("sz000001", func(Kline) bool { return false })
	if err != nil || r.Count != 2 || r.List[0].Time != "2026-10-09 09:30" || r.List[0].Volume != 1 || r.List[1].Volume != 0 || r.List[1].Standard.Last != 10000 {
		t.Fatal("auction split changed", r, err)
	}
	if r.List[0].Standard.Amount != 1000000 || *r.List[0].Amount != 1000 {
		t.Fatal("auction amount units changed", r)
	}
	before := c.Info.WriteCount
	for _, code := range []string{"hk00700", "sh000001", "sz123126"} {
		if _, err = c.GetKlineMinute241Until(code, func(Kline) bool { return false }); err == nil || c.Info.WriteCount != before {
			t.Fatal("unsupported auction sent request", code, err)
		}
	}
}
