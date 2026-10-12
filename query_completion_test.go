package tdx

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

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
			for _, fn := range []func(string, uint16, uint16) (*MarketKlineResp, error){c.GetKline5Minute, c.GetKline15Minute, c.GetKline30Minute, c.GetKline60Minute, c.GetKlineDay, c.GetKlineWeek, c.GetKlineMonth, c.GetKlineQuarter, c.GetKlineYear} {
				r, err := fn(code, 0, 1)
				if err != nil || r.Count != 1 {
					t.Fatal(r, err)
				}
				if !extended && (r.List[0].Standard.UpCount != 3 || r.List[0].Volume != r.List[0].Standard.Volume) {
					t.Fatal("index decoder was not selected", r)
				}
			}
			for _, fn := range []func(string) (*MarketKlineResp, error){c.GetKline5MinuteAll, c.GetKline15MinuteAll, c.GetKline30MinuteAll, c.GetKline60MinuteAll, c.GetKlineDayAll, c.GetKlineWeekAll, c.GetKlineMonthAll, c.GetKlineQuarterAll, c.GetKlineYearAll} {
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
	c, m := new(Client), new(MarketClient)
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
				c := respondingTestClient(t, extended, func(_, b []byte) []byte {
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
				var r *MarketTradeResp
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
				if extended && (r.List[0].Price != nil || r.List[0].Extended.Price != 500000) {
					t.Fatal("extended scale changed")
				}
			})
		}
	}
	c := NewClient()
	defer c.Close()
	c.dial[0] = func() (*Client, error) { return nil, io.ErrClosedPipe }
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
	c := respondingTestClient(t, true, func(h, b []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeExBars {
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
	r, err := c.GetHistoryTradeBefore("hk00700", completionCalendar{days}, days[1].AddDate(0, 0, 1))
	if err != nil || r.Count != 2 || r.List[0].Date != "20261008" || r.List[1].Date != "20261009" {
		t.Fatal(r, err)
	}
	if _, err = c.GetHistoryTradeFull("hk00700", nil); err == nil {
		t.Fatal("nil calendar accepted")
	}
	failed := respondingTestClient(t, true, func(h, b []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeExBars {
			return testExtendedBar(20261001)
		}
		if binary.LittleEndian.Uint32(b) == 20261009 {
			return nil
		}
		r := make([]byte, 32)
		binary.LittleEndian.PutUint16(r[14:], 1)
		return r
	})
	if r, err = failed.GetHistoryTradeBefore("hk00700", completionCalendar{days}, days[1].AddDate(0, 0, 1)); r != nil || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("failed day returned partial history", r, err)
	}
}

func TestUnifiedAuctionMinute(t *testing.T) {
	c := respondingTestClient(t, false, func(h, _ []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeKline {
			return completionStandardBars([]uint32{20261009}, []uint16{571}, false)
		}
		return []byte{1, 0, 0, 0, 0, 0, 0x35, 2, 0xa8, 0x0f, 1, 0, 0} // 历史9:25成交1手、10元。
	})
	r, err := c.GetKlineMinute241Until("sz000001", func(MarketKline) bool { return false })
	if err != nil || r.Count != 2 || r.List[0].Time != "2026-10-09 09:30" || r.List[0].Volume != 1 || r.List[1].Volume != 0 || r.List[1].Standard.Last != 10000 {
		t.Fatal("auction split changed", r, err)
	}
	if r.List[0].Standard.Amount != 1000000 || *r.List[0].Amount != 1000 {
		t.Fatal("auction amount units changed", r)
	}
	before := c.Info.WriteCount
	for _, code := range []string{"hk00700", "sh000001", "sz123126"} {
		if _, err = c.GetKlineMinute241Until(code, func(MarketKline) bool { return false }); err == nil || c.Info.WriteCount != before {
			t.Fatal("unsupported auction sent request", code, err)
		}
	}
}
