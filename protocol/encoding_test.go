package protocol

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"math/rand"
	"testing"

	"github.com/injoyai/conv"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// These oracles retain v1.0.0's padding, truncation and invalid-GBK behavior.
func legacyGBK(b []byte) []byte {
	r := transform.NewReader(bytes.NewReader(b), simplifiedchinese.GBK.NewDecoder())
	v, _ := io.ReadAll(r)
	return bytes.ReplaceAll(v, []byte{0}, []byte{})
}

func TestEncodingLegacyCompatibility(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for n := 0; n <= 32; n++ {
		for sample := 0; sample < 32; sample++ {
			b := make([]byte, n)
			rng.Read(b)
			original := bytes.Clone(b)
			if got, want := Uint16(b), conv.Uint16(Reverse(b)); got != want {
				t.Fatalf("Uint16(%x)=%x, want %x", b, got, want)
			}
			if got, want := Uint32(b), conv.Uint32(Reverse(b)); got != want {
				t.Fatalf("Uint32(%x)=%x, want %x", b, got, want)
			}
			var want float32
			_ = binary.Read(bytes.NewReader(b), binary.LittleEndian, &want)
			if math.Float32bits(Float32(b)) != math.Float32bits(want) {
				t.Fatalf("Float32(%x) changed bits", b)
			}
			if !bytes.Equal(b, original) {
				t.Fatal("decoder changed input")
			}
		}
	}
	for _, bits := range []uint32{0, 0x80000000, 0x7f800000, 0xff800000, 0x7fc01234, 0x7f800001} {
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, bits)
		if math.Float32bits(Float32(b)) != bits {
			t.Fatalf("float bits %08x changed", bits)
		}
	}
}

func TestGBKLegacyCompatibility(t *testing.T) {
	check := func(b []byte) {
		t.Helper()
		original := bytes.Clone(b)
		got, want := GBKToUTF8(b), legacyGBK(b)
		if !bytes.Equal(got, want) || (got == nil) != (want == nil) {
			t.Fatalf("GBK %x: got %x, want %x (nil %v/%v)", b, got, want, got == nil, want == nil)
		}
		if !bytes.Equal(b, original) {
			t.Fatal("GBK decoder changed input")
		}
	}
	check(nil)
	check([]byte{})
	check([]byte{'A', 0, 'B', 0})
	check([]byte{0xc6, 0xbd, 0xb0, 0xb2, 0xd2, 0xf8, 0xd0, 0xd0})
	for first := 0; first <= 255; first++ {
		check([]byte{byte(first)})
		for _, second := range []byte{0, 0x30, 0x40, 0x7f, 0x80, 0xfe, 0xff} {
			check([]byte{byte(first), second})
		}
	}
	rng := rand.New(rand.NewSource(2))
	for _, n := range []int{3, 8, 17, 64, 4095, 4096, 4097, 16384} {
		b := make([]byte, n)
		rng.Read(b)
		check(b)
	}
}
