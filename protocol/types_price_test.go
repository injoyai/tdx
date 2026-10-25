package protocol

import (
	"bytes"
	"testing"
)

func TestIntegerReaderConsistency(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input, tail []byte
		want        int64
	}{
		{"nil", nil, nil, 0}, {"empty", []byte{}, []byte{}, 0},
		{"zero", []byte{0, 9}, []byte{9}, 0}, {"positive", []byte{63, 9}, []byte{9}, 63},
		{"negative", []byte{65, 9}, []byte{9}, -1}, {"multi", []byte{0x81, 2, 9}, []byte{9}, 129},
		{"negative multi", []byte{0xc1, 2, 9}, []byte{9}, -129},
		{"unfinished", []byte{0x81, 0x82}, []byte{0x81, 0x82}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := append([]byte(nil), tc.input...)
			rest, price := GetPrice(tc.input)
			if price != float64(tc.want)/1000 || !bytes.Equal(rest, tc.tail) || (rest == nil) != (tc.tail == nil) {
				t.Fatal(rest, price)
			}
			for _, read := range []func([]byte) ([]byte, int64){
				CutInt64,
				func(b []byte) ([]byte, int64) { rest, v := CutInt(b); return rest, int64(v) },
			} {
				rest, v := read(tc.input)
				if v != tc.want || !bytes.Equal(rest, tc.tail) || (rest == nil) != (tc.tail == nil) {
					t.Fatal(rest, v, tc.tail, tc.want)
				}
				if !bytes.Equal(tc.input, original) {
					t.Fatal("input mutated")
				}
			}
		})
	}
}
