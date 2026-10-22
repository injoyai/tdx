package protocol

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/hex"
	"runtime"
	"testing"
)

func TestDecodeLimitsInflatedResponse(t *testing.T) {
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	if _, err := w.Write(make([]byte, 8<<20)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	frame := make([]byte, 16+compressed.Len())
	binary.LittleEndian.PutUint16(frame[12:14], uint16(compressed.Len()))
	binary.LittleEndian.PutUint16(frame[14:16], 1)
	copy(frame[16:], compressed.Bytes())
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	resp, err := Decode(frame)
	runtime.ReadMemStats(&after)
	if err == nil || resp != nil {
		t.Fatal("accepted oversized response", resp, err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 1<<20 {
		t.Fatalf("allocated %d bytes before rejecting declared length 1", allocated)
	}
}

func TestFrameBytes(t *testing.T) {
	f := Frame{
		MsgID:   1,
		Control: 1,
		Type:    TypeConnect,
		Data:    []byte{0x01},
	}
	hex := f.Bytes().HEX()
	t.Log(hex)
	if hex != "0c0100000001030003000d0001" {
		t.Error("编码错误")
	}
}

func TestBytes(t *testing.T) {
	t.Log(hex.EncodeToString(Bytes(uint32(1))))
	t.Log(hex.EncodeToString(Bytes(uint16(0x0d00))))
}

func TestDecode(t *testing.T) {
	s := "b1cb74001c00000000000d005100bd00789c6378c1cecb252ace6066c5b4898987b9050ed1f90cc5b74c18a5bc18c1b43490fecff09c81819191f13fc3c9f3bb169f5e7dfefeb5ef57f7199a305009308208e5b32bb6bcbf70148712002d7f1e13"
	bs, err := hex.DecodeString(s)
	if err != nil {
		t.Error(err)
		return
	}
	resp, err := Decode(bs)
	if err != nil {
		t.Error(err)
		return
	}

	t.Log(len(resp.Data))

	t.Log(hex.EncodeToString(resp.Data))

	t.Log(string(resp.Data))

	t.Log(string(GBKToUTF8(resp.Data)))

	t.Log(string(GBKToUTF8(resp.Data[68:])))

	t.Log(MConnect.Decode(resp.Data))

	//result, err := DecodeSecurityList(resp.Data)
	//if err != nil {
	//	t.Error(err)
	//	return
	//}
	//
	//t.Log(hex.EncodeToString(resp.Data))
	//
	//t.Log(result)
}

func TestReadFrom(t *testing.T) {
	s := "b1cb74001c00000000000d005100bd00789c6378c12ec325c7cb2061c5b4898987b9050ed1f90c2db74c1825bd18c1b42890fecff09c81819191f13fc3c9f3bb169f5e7dfefeb5ef57f7199a305009308208e5b32bb6bcbf701487120031c61e1e"
	bs, err := hex.DecodeString(s)
	if err != nil {
		t.Error(err)
		return
	}
	result, err := ReadFrom(bytes.NewReader(bs))
	if err != nil {
		t.Error(err)
		return
	}
	t.Log(hex.EncodeToString(result))
	resp, err := Decode(result)
	if err != nil {
		t.Error(err)
		return
	}
	t.Log(hex.EncodeToString(resp.Data))
	t.Log(string(resp.Data))
}
