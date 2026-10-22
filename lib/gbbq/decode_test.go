package gbbq

import (
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func TestDecodeTruncatedFile(t *testing.T) {
	for n := 0; n < 33; n++ {
		bs := make([]byte, n)
		if n >= 4 {
			binary.LittleEndian.PutUint32(bs, 1)
		}
		if _, err := Decode(bs); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("length %d: %v", n, err)
		}
	}
	if got, err := Decode(make([]byte, 4)); err != nil || len(got) != 0 {
		t.Fatal("empty valid file", got, err)
	}
}
