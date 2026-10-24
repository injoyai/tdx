package protocol

import (
	"encoding/binary"
	"math"
	"testing"
)

var benchmarkResult any

func BenchmarkProtocol(b *testing.B) {
	b.Run("Uint32", func(b *testing.B) {
		data := []byte{0, 0, 32, 65}
		var v uint32
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v = Uint32(data)
		}
		benchmarkResult = v
	})
	b.Run("GBKName", func(b *testing.B) {
		data := []byte{0xc6, 0xbd, 0xb0, 0xb2, 0xd2, 0xf8, 0xd0, 0xd0, 0, 0}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkResult = GBKToUTF8(data)
		}
	})
	b.Run("Frame", func(b *testing.B) {
		f := &Frame{MsgID: 1, Control: Control01, Type: TypeKline, Data: make([]byte, 26)}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkResult = f.Bytes()
		}
	})
	b.Run("Codes1000", func(b *testing.B) {
		data := make([]byte, 2+1000*29)
		binary.LittleEndian.PutUint16(data, 1000)
		for i := 0; i < 1000; i++ {
			copy(data[2+i*29:], "600000")
			copy(data[10+i*29:], []byte{0xc6, 0xbd, 0xb0, 0xb2, 0xd2, 0xf8, 0xd0, 0xd0})
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			r, err := MCode.Decode(data)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkResult = r
		}
	})
	b.Run("Klines800", func(b *testing.B) {
		data := make([]byte, 2+800*16)
		binary.LittleEndian.PutUint16(data, 800)
		for i := 0; i < 800; i++ {
			row := data[2+i*16:]
			binary.LittleEndian.PutUint32(row, 20250102)
			row[4] = 10
			binary.LittleEndian.PutUint32(row[8:], math.Float32bits(100))
			binary.LittleEndian.PutUint32(row[12:], math.Float32bits(1000))
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			r, err := MKline.Decode(data, KlineCache{Type: TypeKlineDay})
			if err != nil {
				b.Fatal(err)
			}
			benchmarkResult = r
		}
	})
	b.Run("Trades1800", func(b *testing.B) {
		data := make([]byte, 2, 2+1800*8)
		binary.LittleEndian.PutUint16(data, 1800)
		for i := 0; i < 1800; i++ {
			data = append(data, 0x3a, 2, 0xa8, 0x0f, 1, 3, 1, 0)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			r, err := MTrade.Decode(data, TradeCache{Date: "20250102", Code: "sz000001"})
			if err != nil {
				b.Fatal(err)
			}
			benchmarkResult = r
		}
	})
}
