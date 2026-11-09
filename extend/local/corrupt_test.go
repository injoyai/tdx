package local

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"
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
