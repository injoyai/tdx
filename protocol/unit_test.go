package protocol

import (
	"encoding/hex"
	"math"
	"testing"
)

func TestUTF8ToGBK(t *testing.T) {
	s := "789c6378c1cec7a5cbcbc061c5b4898987b9050ed1f90c65b74c1825bd18c1b42890fecff09c81819191f13fc3c9f3bb169f5e7dfefeb5ef57f7199a305009308208e5b32bb6bcbf7014871200176e1df3"
	//s := "b1cb74001c00000000000d005100bd00789c6378c1cecb252ace6066c5b4898987b9050ed1f90cc5b74c18a5bc18c1b43490fecff09c81819191f13fc3c9f3bb169f5e7dfefeb5ef57f7199a305009308208e5b32bb6bcbf70148712002d7f1e13"
	bs, err := hex.DecodeString(s)
	if err != nil {
		t.Error(err)
		return
	}
	t.Log(string(bs[68:]))
	bs = UTF8ToGBK(bs)
	t.Log(string(bs))
}

func TestGetVolume(t *testing.T) {
	for _, want := range []float32{0, 0.5, 1, 10, 436, 587760.1875} {
		bits := math.Float32bits(want)
		if got := getVolume(bits); got != float64(want) {
			t.Errorf("getVolume(%08x) = %v, want %v", bits, got, want)
		}
		if got := getVolume2(bits); got != float64(want) {
			t.Errorf("getVolume2(%08x) = %v, want %v", bits, got, want)
		}
	}
}

func TestIsConvertibleBond(t *testing.T) {
	for _, code := range []string{"sh110075", "SH111000", "sh113001", "sh118001", "sz123064", "sz125001", "sz126001", "sz127001", "sz128001"} {
		if !IsConvertibleBond(code) {
			t.Errorf("IsConvertibleBond(%q) = false, want true", code)
		}
	}
	for _, code := range []string{"sh600000", "sz000001", "sh510300", "sz159919", "sh000001", "110075"} {
		if IsConvertibleBond(code) {
			t.Errorf("IsConvertibleBond(%q) = true, want false", code)
		}
	}
}

func TestFloat32(t *testing.T) {
	t.Log(Float32([]byte{0x00, 0x00, 0x20, 0x41})) //10
}
