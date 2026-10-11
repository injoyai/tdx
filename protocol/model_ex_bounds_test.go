package protocol

import (
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func TestExDecodeBounds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		header int
		size   int
		count  int
		decode func([]byte) error
	}{
		{"markets", 2, 64, 0, func(b []byte) error { _, e := MEx.DecodeMarkets(b); return e }},
		{"instruments", 6, 64, 4, func(b []byte) error { _, e := MEx.DecodeInstrument(b); return e }},
		{"bars", 20, 32, 18, func(b []byte) error { _, e := MEx.DecodeBars(b, ExBarsCache{}); return e }},
		{"minute", 12, 18, 10, func(b []byte) error { _, e := MEx.DecodeMinute(b); return e }},
		{"history minute", 20, 18, 18, func(b []byte) error { _, e := MEx.DecodeHistMinute(b); return e }},
		{"trade", 16, 16, 14, func(b []byte) error { _, e := MEx.DecodeTrade(b, ExTradeCache{}); return e }},
		{"history trade", 16, 16, 14, func(b []byte) error { _, e := MEx.DecodeHistTrade(b, ExTradeCache{}); return e }},
		{"bars range", 14, 32, 12, func(b []byte) error { _, e := MEx.DecodeBarsRange(b); return e }},
		{"quote list", 2, 300, 0, func(b []byte) error { _, e := MEx.DecodeQuoteList(b, ExQuoteListCache{Category: 2}); return e }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := make([]byte, tc.header+2*tc.size)
			if err := tc.decode(b[:tc.header]); err != nil {
				t.Fatal("valid zero count rejected", err)
			}
			binary.LittleEndian.PutUint16(b[tc.count:], 2)
			if err := tc.decode(b); err != nil {
				t.Fatal("complete records rejected", err)
			}
			for _, n := range []int{0, tc.header - 1, tc.header, tc.header + tc.size, len(b) - 1} {
				if err := tc.decode(b[:n]); !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("length %d: expected unexpected EOF, got %v", n, err)
				}
			}
		})
	}
	for _, tc := range []struct {
		name   string
		size   int
		decode func([]byte) error
	}{
		{"count", 23, func(b []byte) error { _, e := MEx.DecodeCount(b); return e }},
		{"quote", 150, func(b []byte) error { _, e := MEx.DecodeQuote(b); return e }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := make([]byte, tc.size)
			if err := tc.decode(b); err != nil {
				t.Fatal(err)
			}
			if err := tc.decode(b[:tc.size-1]); !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatal("truncated payload accepted", err)
			}
		})
	}
}
