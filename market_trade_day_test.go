package tdx

import (
	"encoding/binary"
	"errors"
	"io"
	"sync/atomic"
	"testing"

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
					var r *MarketTradeResp
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
