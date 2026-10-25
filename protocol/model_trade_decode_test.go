package protocol

import (
	"encoding/binary"
	"testing"
)

func TestTradeDecodeBounds(t *testing.T) {
	cache := TradeCache{Date: "20250102", Code: "sz000001"}
	for _, historical := range []bool{false, true} {
		name := "current"
		if historical {
			name = "historical"
		}
		t.Run(name, func(t *testing.T) {
			decode := MTrade.Decode
			header := []byte{1, 0}
			// 09:30、价格差 1000 分、1 手；实时数据另有单数。
			record := []byte{0x3a, 2, 0xa8, 0x0f, 1, 3, 1, 0}
			if historical {
				decode = MHistoryTrade.Decode
				header = append(header, 0, 0, 0, 0)
				record = []byte{0x3a, 2, 0xa8, 0x0f, 1, 1, 0}
			}
			valid := append(append([]byte(nil), header...), record...)
			r, err := decode(valid, cache)
			if err != nil || len(r.List) != 1 || r.List[0].Price != 10 ||
				r.List[0].Volume != 1 || r.List[0].Status != 1 || r.List[0].Time.Format("15:04") != "09:30" {
				t.Fatal("valid record changed", r, err)
			}
			if !historical && r.List[0].Number != 3 {
				t.Fatal("real-time order count lost", r.List[0].Number)
			}
			for n := 0; n < len(valid); n++ {
				if _, err := decode(valid[:n], cache); err == nil {
					t.Fatalf("accepted truncated record of length %d", n)
				}
			}
			extraRecord := append([]byte(nil), valid...)
			binary.LittleEndian.PutUint16(extraRecord, 2)
			if _, err := decode(extraRecord, cache); err == nil {
				t.Fatal("accepted missing second record")
			}
			for _, price := range [][]byte{{0x80}, {0x80, 0x80, 0x80, 0x80, 0x10}, {0x80, 0x80, 0x80, 0x80, 0x80, 0}} {
				bad := append(append([]byte(nil), header...), 0x3a, 2)
				bad = append(bad, price...)
				if _, err := decode(bad, cache); err == nil {
					t.Fatal("accepted invalid variable-length integer", price)
				}
			}
		})
	}
}

func TestTradeIntegerLimits(t *testing.T) {
	for _, tc := range []struct {
		bs   []byte
		want int
	}{
		{[]byte{0}, 0},
		{[]byte{0x41}, -1},
		{[]byte{0xbf, 0xff, 0xff, 0xff, 0x0f}, 2147483647},
		{[]byte{0xff, 0xff, 0xff, 0xff, 0x0f}, -2147483647},
	} {
		rest, got, err := cutIntChecked(tc.bs)
		if err != nil || got != tc.want || len(rest) != 0 {
			t.Fatal(tc.bs, got, err)
		}
	}
}
