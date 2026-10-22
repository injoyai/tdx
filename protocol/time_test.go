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
