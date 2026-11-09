package protocol

import (
	"bytes"
	"testing"
)

func TestCheckedIntegerReader(t *testing.T) {
	for _, tc := range []struct {
		name        string
		input, tail []byte
		want        int
		wantErr     bool
	}{
		{name: "nil", wantErr: true},
		{name: "empty", input: []byte{}, wantErr: true},
		{name: "zero", input: []byte{0, 9}, tail: []byte{9}},
		{name: "positive", input: []byte{63, 9}, tail: []byte{9}, want: 63},
		{name: "negative", input: []byte{65, 9}, tail: []byte{9}, want: -1},
		{name: "multi", input: []byte{0x81, 2, 9}, tail: []byte{9}, want: 129},
		{name: "negative multi", input: []byte{0xc1, 2, 9}, tail: []byte{9}, want: -129},
		{name: "max magnitude", input: []byte{0xbf, 0xff, 0xff, 0xff, 0x0f, 9}, tail: []byte{9}, want: 2147483647},
		{name: "negative max magnitude", input: []byte{0xff, 0xff, 0xff, 0xff, 0x0f, 9}, tail: []byte{9}, want: -2147483647},
		{name: "unfinished", input: []byte{0x81, 0x82}, wantErr: true},
		{name: "overflow", input: []byte{0x80, 0x80, 0x80, 0x80, 0x10}, wantErr: true},
		{name: "overlong", input: []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := bytes.Clone(tc.input)
			rest, got, err := cutIntChecked(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v, wantErr=%v", err, tc.wantErr)
			}
			if got != tc.want || !bytes.Equal(rest, tc.tail) || (rest == nil) != (tc.tail == nil) {
				t.Fatalf("value=%d tail=%x, want value=%d tail=%x", got, rest, tc.want, tc.tail)
			}
			if !bytes.Equal(tc.input, original) {
				t.Fatal("input mutated")
			}
		})
	}
}
