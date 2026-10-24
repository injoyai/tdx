package tdx

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"testing"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestUnifiedVolumeJSON(t *testing.T) {
	for _, value := range []any{Quote{}, KlineResp{}, MinuteResp{}, TradeResp{}} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		if _, ok := fields["VolumeUnit"]; ok {
			t.Fatalf("%T still exports VolumeUnit", value)
		}
	}
}

func TestUnifiedQuoteVolume(t *testing.T) {
	c := marketTestClient(t, func(_, b []byte) []byte {
		data := testQuoteResponse(b)
		data[21] = 2
		for j := 0; j < 5; j++ {
			data[33+4*j], data[34+4*j] = 3, 4
		}
		return data
	}, func(_ []byte, b []byte) []byte {
		data := make([]byte, 150)
		copy(data, b)
		binary.LittleEndian.PutUint32(data[42:], 2)
		for j := 0; j < 5; j++ {
			binary.LittleEndian.PutUint32(data[90+4*j:], 3)
			binary.LittleEndian.PutUint32(data[130+4*j:], 4)
		}
		return data
	})
	for _, code := range []string{"sz000001", "hk00700", "usAAPL", "cffIF2609"} {
		quotes, err := c.GetQuote(code)
		if err != nil {
			t.Fatal(code, err)
		}
		q := quotes[0]
		scale := int64(1)
		if code == "sz000001" {
			scale = 100
		}
		if q.Volume != 2*scale {
			t.Fatal(code, q)
		}
		for j := 0; j < 5; j++ {
			if q.BidVolume[j] != 3*scale || q.AskVolume[j] != 4*scale {
				t.Fatal(code, q)
			}
		}
		if q.Standard != nil && (q.Standard.Kline.Volume != 2 || q.Standard.BuyLevel[0].Number != 3) {
			t.Fatal("raw standard quote changed", q.Standard)
		}
		if q.Extended != nil && (q.Extended.ZongLiang != 2 || q.Extended.BidVol[0] != 3) {
			t.Fatal("raw extended quote changed", q.Extended)
		}
	}
}

func TestScaleVolumeBounds(t *testing.T) {
	for _, tc := range []struct {
		volume, scale, want int64
		valid               bool
	}{
		{0, 100, 0, true}, {123, 100, 12300, true},
		{math.MaxInt64, 1, math.MaxInt64, true},
		{math.MaxInt64 / 100, 100, math.MaxInt64 / 100 * 100, true},
		{math.MaxInt64/100 + 1, 100, 0, false}, {-1, 100, 0, false}, {1, 0, 0, false},
	} {
		got, err := scaleVolume(tc.volume, tc.scale)
		if (err == nil) != tc.valid || got != tc.want {
			t.Fatal(tc, got, err)
		}
	}
}

func TestForeignIndexVolumeRejected(t *testing.T) {
	for _, code := range []string{"hk.HSI", "hk.CES120"} {
		i, info, err := instrumentInfo(code)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := klineVolumeScale(i, info.Day.Category, info); err == nil {
			t.Fatal("unconfirmed Kline unit accepted", code)
		}
		if _, err := volumeScale(i); err == nil {
			t.Fatal("unconfirmed tick unit accepted", code)
		}
	}
	i, info, _ := instrumentInfo("sh000001")
	if scale, err := klineVolumeScale(i, protocol.TypeKlineDay, info); err != nil || scale != 1 {
		t.Fatal("index scaled twice", scale, err)
	}
}

func TestDomesticDefaultVolume(t *testing.T) {
	for _, code := range []string{"sh110075", "sz123126", "sh019001", "sz131810", "sh000001"} {
		i, info, err := instrumentInfo(code)
		if err != nil {
			t.Fatal(err)
		}
		if scale, err := volumeScale(i); err != nil || scale != 100 {
			t.Fatal(code, scale, err)
		}
		want := int64(100)
		if protocol.IsIndex(standardCode(i)) {
			want = 1
		}
		if scale, err := klineVolumeScale(i, info.Day.Category, info); err != nil || scale != want {
			t.Fatal(code, scale, err)
		}
	}
}

func TestDomesticDefaultKlines(t *testing.T) {
	c := marketTestClient(t, func(_ []byte, b []byte) []byte {
		if b[8] == protocol.TypeKlineMinute {
			return completionStandardBars([]uint32{20261009}, []uint16{571}, false)
		}
		return testStandardBar(false)
	}, nil)
	for _, tc := range []struct {
		code                    string
		minuteVolume, minuteRaw int64
	}{{"sh110075", 10000, 100}, {"sz123126", 10000, 100}, {"sh019001", 100, 1}, {"sz131810", 100, 1}} {
		code := tc.code
		day, err := c.GetKlineDay(code, 0, 1)
		if err != nil || day.Count != 1 || day.List[0].Volume != 10000 || day.List[0].Standard.Volume != 100 {
			t.Fatal(code, day, err)
		}
		minute, err := c.GetKlineMinute(code, 0, 1)
		if err != nil || minute.Count != 1 || minute.List[0].Volume != tc.minuteVolume || minute.List[0].Standard.Volume != tc.minuteRaw {
			t.Fatal(code, minute, err)
		}
	}
}
