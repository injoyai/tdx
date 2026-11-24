package protocol

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestGetTimeIndependentOfHostTimezone(t *testing.T) {
	previous := time.Local
	defer func() { time.Local = previous }()
	var day, minute [4]byte
	binary.LittleEndian.PutUint32(day[:], 20261020)
	binary.LittleEndian.PutUint16(minute[:], uint16((2026-2004)*2048+10*100+20))
	binary.LittleEndian.PutUint16(minute[2:], 9*60+31)
	for _, host := range []*time.Location{time.UTC, time.FixedZone("west", -5*3600), Location()} {
		time.Local = host
		for _, tc := range []struct {
			raw       [4]byte
			kind      uint8
			hour, min int
		}{{day, TypeKlineDay, 15, 0}, {minute, TypeKlineMinute, 9, 31}} {
			got := GetTime(tc.raw, tc.kind)
			want := time.Date(2026, 10, 20, tc.hour, tc.min, 0, 0, Location())
			if !got.Equal(want) {
				t.Fatalf("host %s: %s != %s", host, got, want)
			}
		}
	}
}

func TestKlineDecodersRejectInvalidWireTimes(t *testing.T) {
	var badDay, badMinuteDate, badClock [4]byte
	binary.LittleEndian.PutUint32(badDay[:], 20260230)
	binary.LittleEndian.PutUint16(badMinuteDate[:], (2026-2004)*2048+230)
	binary.LittleEndian.PutUint16(badClock[:], (2026-2004)*2048+228)
	binary.LittleEndian.PutUint16(badClock[2:], 24*60)
	for _, tc := range []struct {
		name     string
		category uint8
		raw      [4]byte
	}{
		{"day", TypeKlineDay, badDay},
		{"minute date", TypeKlineMinute, badMinuteDate},
		{"minute clock", TypeKlineMinute, badClock},
	} {
		t.Run(tc.name, func(t *testing.T) {
			std := make([]byte, 2+16)
			std[0] = 1
			copy(std[2:], tc.raw[:])
			if got, err := MKline.Decode(std, KlineCache{Type: tc.category}); err == nil || got != nil {
				t.Errorf("standard accepted invalid time: %+v, %v", got, err)
			}
			ex := make([]byte, 20+32)
			ex[18] = 1
			copy(ex[20:], tc.raw[:])
			if got, err := MEx.DecodeBars(ex, ExBarsCache{Category: tc.category}); err == nil || got != nil {
				t.Errorf("extended accepted invalid time: %+v, %v", got, err)
			}
			if tc.category == TypeKlineMinute {
				interval := make([]byte, 14+32)
				interval[12] = 1
				copy(interval[14:], tc.raw[:])
				if got, err := MEx.DecodeBarsRange(interval); err == nil || got != nil {
					t.Errorf("extended range accepted invalid time: %+v, %v", got, err)
				}
			}
		})
	}
}
