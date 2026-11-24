package local

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestReadersRejectCorruptRecordsWithoutPartialData(t *testing.T) {
	for _, kind := range []string{"day", "lc1", "lc5"} {
		for _, bad := range []string{"truncated", "date", "clock", "odd shares", "nan", "infinity"} {
			if kind == "day" && bad == "clock" {
				continue
			}
			t.Run(kind+"/"+bad, func(t *testing.T) {
				good := make([]byte, 32)
				if kind == "day" {
					binary.LittleEndian.PutUint32(good, 20260202)
				} else {
					binary.LittleEndian.PutUint16(good, (2026-2004)*2048+202)
					binary.LittleEndian.PutUint16(good[2:], 571)
				}
				binary.LittleEndian.PutUint32(good[24:], 100)
				broken := append([]byte(nil), good...)
				switch bad {
				case "truncated":
					broken = broken[:31]
				case "date":
					if kind == "day" {
						binary.LittleEndian.PutUint32(broken, 20260231)
					} else {
						binary.LittleEndian.PutUint16(broken, (2026-2004)*2048+231)
					}
				case "clock":
					binary.LittleEndian.PutUint16(broken[2:], 24*60)
				case "odd shares":
					binary.LittleEndian.PutUint32(broken[24:], 101)
				case "nan":
					binary.LittleEndian.PutUint32(broken[20:], math.Float32bits(float32(math.NaN())))
				case "infinity":
					binary.LittleEndian.PutUint32(broken[20:], math.Float32bits(float32(math.Inf(1))))
				}
				sub, ext, read := "lday", ".day", ReadDay
				if kind == "lc1" {
					sub, ext, read = "minline", ".lc1", ReadMinute1
				} else if kind == "lc5" {
					sub, ext, read = "fzline", ".lc5", ReadMinute5
				}
				dir := t.TempDir()
				writeLocalFixture(t, dir, "sz000001", sub, ext, append(good, broken...))
				got, err := read(dir, "sz000001")
				if err == nil || got != nil {
					t.Fatal("accepted corrupt data or returned partial records", got, err)
				}
				if bad == "truncated" && !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatal("truncated file lost its error cause", err)
				}
			})
		}
	}
}

func TestReadDayKeepsIndexLotsWithoutDivisibilityCheck(t *testing.T) {
	dir := t.TempDir()
	data := make([]byte, 32)
	binary.LittleEndian.PutUint32(data, 20260202)
	binary.LittleEndian.PutUint32(data[24:], 101)
	writeLocalFixture(t, dir, "sh000001", "lday", ".day", data)
	got, err := ReadDay(dir, "sh000001")
	if err != nil || len(got) != 1 || got[0].Volume != 101 {
		t.Fatal("index volume was treated as shares", got, err)
	}
}

func TestWritersRejectUnrepresentableRecords(t *testing.T) {
	for _, writer := range []struct {
		name string
		fn   func(string, protocol.Klines) ([]byte, error)
	}{
		{"day", WriteDay}, {"lc1", WriteMinute1}, {"lc5", WriteMinute5},
	} {
		t.Run(writer.name, func(t *testing.T) {
			for _, tc := range []struct {
				code   string
				volume int64
			}{
				{"sz000001", -1}, {"sh000001", -1},
				{"sz000001", math.MaxUint32/protocol.SharesPerLot + 1},
				{"sh000001", math.MaxUint32 + 1},
			} {
				row := &protocol.Kline{Time: time.Date(2026, 1, 5, 9, 31, 0, 0, protocol.Location()), Volume: tc.volume}
				if got, err := writer.fn(tc.code, protocol.Klines{row}); err == nil || got != nil {
					t.Errorf("%s volume %d: wrote %d bytes, err=%v", tc.code, tc.volume, len(got), err)
				}
			}
			years := []int{2003, 2036}
			if writer.name == "day" {
				years = []int{0, 10000}
			}
			for _, year := range years {
				row := &protocol.Kline{Time: time.Date(year, 1, 5, 9, 31, 0, 0, protocol.Location())}
				if got, err := writer.fn("sz000001", protocol.Klines{row}); err == nil || got != nil {
					t.Errorf("year %d: wrote %d bytes, err=%v", year, len(got), err)
				}
			}
		})
	}
}

func TestMinute1PreservesZeroVolumeClosingBar(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 9, 2, 15, 0, 0, 0, protocol.Location())
	k := &protocol.Kline{Time: at, Open: 10, High: 10, Low: 10, Close: 10}
	data, err := WriteMinute1("sz000001", protocol.Klines{k})
	if err != nil {
		t.Fatal(err)
	}
	writeLocalFixture(t, dir, "sz000001", "minline", ".lc1", data)
	got, err := ReadMinute1(dir, "sz000001")
	if len(data) != 64 || err != nil || len(got) != 1 || !got[0].Time.Equal(at) || got[0].Close != k.Close {
		t.Fatalf("closing bar round trip: %d bytes, %+v, %v", len(data), got, err)
	}
}
