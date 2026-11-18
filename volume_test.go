package tdx

import (
	"encoding/binary"
	"encoding/hex"
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
	for _, code := range []string{"sz000001", "sh000001", "hk00700", "usAAPL", "cffIF2609"} {
		quotes, err := c.GetQuote(code)
		if err != nil {
			t.Fatal(code, err)
		}
		q := quotes[0]
		scale := int64(1)
		if code == "sz000001" || code == "sh000001" {
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

func TestForeignIndexTickVolumeRejected(t *testing.T) {
	for _, code := range []string{"hk.HSI", "hk.CES120"} {
		i, _, err := instrumentInfo(code)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := volumeScale(i); err == nil {
			t.Fatal("unconfirmed tick unit accepted", code)
		}
	}
}

func TestHKIndexKlineWithoutVolume(t *testing.T) {
	const rawAmount float32 = 198051.5625 // 恒指 2026-11-17 日线的报文金额，单位百万港元。
	c := marketTestClient(t, nil, func(h, b []byte) []byte {
		if b[0] != byte(protocol.ExchangeHI) {
			t.Errorf("request routed to wrong market: %d", b[0])
		}
		bar := testExtendedBar(20261117)
		binary.LittleEndian.PutUint32(bar[40:44], math.Float32bits(rawAmount))
		binary.LittleEndian.PutUint32(bar[44:48], 19805)
		if protocol.IsIntraday(b[10]) {
			binary.LittleEndian.PutUint16(bar[20:22], (2026-2004)*2048+1117)
			binary.LittleEndian.PutUint16(bar[22:24], 16*60)
		}
		if binary.LittleEndian.Uint16(h[10:12]) == protocol.TypeExBarsRange {
			return bar[6:]
		}
		return bar
	})
	for _, code := range []string{"hk.HSI", "hk.CES120", "hk.CES100"} {
		check := func(r *KlineResp, err error) {
			t.Helper()
			if err != nil || r.Count != 1 {
				t.Fatal(code, r, err)
			}
			row := r.List[0]
			if row.Open != 10.25 || row.Close != 10.5 || row.Volume != 0 {
				t.Fatal(code, row)
			}
			if row.Amount == nil || *row.Amount != 198051562500 {
				t.Fatalf("%s amount = %v, want %d", code, row.Amount, int64(198051562500))
			}
			if row.Extended != nil {
				if row.Extended.Trade != 19805 || row.Extended.Amount != float64(rawAmount) {
					t.Fatal(row.Extended)
				}
			} else if row.RangeExtended == nil || row.RangeExtended.Trade != 19805 || row.RangeExtended.Position != math.Float32bits(rawAmount) {
				t.Fatal(row.RangeExtended)
			}
		}
		check(c.GetKlineDay(code, 0, 1))
		check(c.GetKlineMinute(code, 0, 1))
		check(c.GetKlineWeek(code, 0, 1))
		check(c.GetKlineRange(code, "20261117", "20261117"))
	}
}

func TestDomesticDefaultVolume(t *testing.T) {
	for _, code := range []string{"sh019001", "sz131810", "sh000001"} {
		i, info, err := instrumentInfo(code)
		if err != nil {
			t.Fatal(err)
		}
		if scale, err := volumeScale(i); err != nil || scale != 100 {
			t.Fatal(code, scale, err)
		}
		if scale, err := klineVolumeScale(i, info.Day.Category, info); err != nil || scale != 100 {
			t.Fatal(code, scale, err)
		}
	}
}

func TestConvertibleBondUnits(t *testing.T) {
	c := marketTestClient(t, func(h, b []byte) []byte {
		switch binary.LittleEndian.Uint16(h[10:12]) {
		case protocol.TypeCode:
			data := make([]byte, 31)
			binary.LittleEndian.PutUint16(data, 1)
			code := "113052"
			if b[0] == byte(protocol.ExchangeSZ) {
				code = "123243"
			}
			copy(data[2:8], code)
			data[22] = 4
			return data
		case protocol.TypeQuote:
			data := testQuoteResponse(b)
			data[21] = 2
			data[33], data[34] = 3, 4
			return data
		case protocol.TypeHistoryMinute:
			return []byte{1, 0, 0, 0, 0, 0, 0xa8, 0x0f, 0, 2} // 10 解码元，2 手。
		case protocol.TypeTrade:
			return []byte{1, 0, 0x3a, 2, 0xa8, 0x0f, 2, 1, 0, 0}
		case protocol.TypeHistoryTrade:
			return []byte{1, 0, 0, 0, 0, 0, 0x3a, 2, 0xa8, 0x0f, 2, 0, 0}
		case protocol.TypeKline:
			if protocol.IsIntraday(b[8]) {
				return completionStandardBars([]uint32{20261009}, []uint16{571}, false)
			}
		}
		return testStandardBar(false)
	}, nil)
	for _, code := range []string{"sh113052", "sz123243"} {
		q, err := c.GetQuote(code)
		if err != nil || len(q) != 1 || q[0].Price != 0.1 || q[0].Volume != 20 || q[0].BidVolume[0] != 30 || q[0].AskVolume[0] != 40 {
			t.Fatal(code, q, err)
		}
		for category := uint8(0); category <= protocol.TypeKlineYear; category++ {
			r, err := c.GetKline(category, code, 0, 1)
			if err != nil || r.Count != 1 {
				t.Fatal(code, category, r, err)
			}
			want := int64(100000)
			if protocol.IsIntraday(category) {
				want = 1000
			}
			row := r.List[0]
			if row.Volume != want || row.Standard.Volume != 100 || row.Close != 10 || row.Amount == nil || *row.Amount != 1000 {
				t.Fatalf("%s category %d: %+v, want volume %d", code, category, row, want)
			}
		}
		r, err := c.GetHistoryMinute("20261009", code)
		if err != nil || r.Count != 1 || r.List[0].Price != 0.1 || r.List[0].Volume != 20 {
			t.Fatal(code, r, err)
		}
		for _, fetch := range []func() (*TradeResp, error){
			func() (*TradeResp, error) { return c.GetTrade(code, 0, 1) },
			func() (*TradeResp, error) { return c.GetHistoryTrade("20261009", code, 0, 1) },
		} {
			r, err := fetch()
			if err != nil || r.Count != 1 || r.List[0].Price == nil || *r.List[0].Price != 1 || r.List[0].Volume != 20 {
				t.Fatal(code, r, err)
			}
		}
	}
}

func TestIndexKlineVolumeScaleByPeriod(t *testing.T) {
	for _, tc := range []struct {
		code     string
		day      int64
		intraday int64
	}{
		{"sh000001", 100, 0},
		{"sh000012", 100, 0},
		{"sz399001", 100, 0},
		{"bj899050", 100, 0},
		{"sh880001", 1, 1},
		{"sh881001", 1, 1},
		{"hk.HSI", 0, 0},
		{"hk.CES120", 0, 0},
	} {
		i, info, err := instrumentInfo(tc.code)
		if err != nil {
			t.Fatal(tc.code, err)
		}
		if info.Day.VolumeScale != tc.day {
			t.Fatalf("%s day configuration = %d, want %d", tc.code, info.Day.VolumeScale, tc.day)
		}
		for category := uint8(0); category <= protocol.TypeKlineYear; category++ {
			if category == protocol.TypeKlineDay2 || category == protocol.TypeKlineMinute2 {
				continue // 公开入口先将周期别名映射到市场的实际周期。
			}
			want := tc.day
			if protocol.IsIntraday(category) {
				want = tc.intraday
			}
			if got, err := klineVolumeScale(i, category, info); err != nil || got != want {
				t.Fatalf("%s category %d: scale=%d err=%v, want %d", tc.code, category, got, err, want)
			}
		}
	}
}

func TestStandardIndexTickAmount(t *testing.T) {
	c := marketTestClient(t, func(h, _ []byte) []byte {
		switch binary.LittleEndian.Uint16(h[10:12]) {
		case protocol.TypeHistoryMinute:
			return []byte{2, 0, 0, 0, 0, 0, 0xa8, 0x0f, 0, 2, 0, 0, 0}
		case protocol.TypeTrade:
			return []byte{1, 0, 0x3a, 2, 0xa8, 0x0f, 2, 1, 2, 0}
		case protocol.TypeHistoryTrade:
			return []byte{1, 0, 0, 0, 0, 0, 0x3a, 2, 0xa8, 0x0f, 2, 2, 0}
		}
		return testStandardBar(true)
	}, nil)
	for _, code := range []string{"sh000001", "sz399001", "bj899050"} {
		for _, historical := range []bool{false, true} {
			var minute *MinuteResp
			var trade *TradeResp
			var err error
			if historical {
				minute, err = c.GetHistoryMinute("20261009", code)
			} else {
				minute, err = c.GetMinute(code)
			}
			if err != nil || minute.Count != 2 {
				t.Fatal(code, minute, err)
			}
			row := minute.List[0]
			if row.Volume != 0 || row.Amount == nil || *row.Amount != 20000 || row.Standard.Number != 2 || row.Price != 10 {
				t.Fatal(code, row)
			}
			if zero := minute.List[1]; zero.Amount == nil || *zero.Amount != 0 || zero.Volume != 0 {
				t.Fatal("known zero amount lost", code, zero)
			}
			if historical {
				trade, err = c.GetHistoryTrade("20261009", code, 0, 1)
			} else {
				trade, err = c.GetTrade(code, 0, 1)
			}
			if err != nil || trade.Count != 1 {
				t.Fatal(code, trade, err)
			}
			tick := trade.List[0]
			if tick.Volume != 0 || tick.Amount == nil || *tick.Amount != 200 || tick.Standard.Volume != 2 ||
				tick.Price == nil || *tick.Price != 10 || tick.Direction != 0 {
				t.Fatal(code, tick)
			}
		}
	}
}

func TestCapturedStandardIndexDayVolume(t *testing.T) {
	// 7709 的深证成指日线响应，2026-11-13 至 2026-11-17。
	data, err := hex.DecodeString("0300f928350180cddb0aead120bd8b02ca8224d0abaa4a55114e537801e309fc283501e0cd0adaaa1700f49e1d5ebe9e4adf34355319023209fd283501eab90a8689048af113f0fb055026a44a7d9f32537e02bd08")
	if err != nil {
		t.Fatal(err)
	}
	c := marketTestClient(t, func(_, b []byte) []byte {
		if b[0] != byte(protocol.ExchangeSZ) || string(b[2:8]) != "399001" || b[8] != protocol.TypeKlineDay {
			t.Error("captured index request mismatch", b)
		}
		return data
	}, nil)
	r, err := c.GetKlineDay("sz399001", 0, 3)
	if err != nil || r.Count != 3 {
		t.Fatal(r, err)
	}
	last := r.List[2]
	if last.Time != "2026-11-17" || last.Close != 10638.52 || last.Amount == nil || *last.Amount != 767_179_948_032 ||
		last.Standard.Volume != 537_885_600 || last.Volume != 53_788_560_000 {
		t.Fatalf("unexpected index day: %+v, raw=%+v", last, last.Standard)
	}
	ks, err := r.Klines()
	if err != nil || ks[2].Volume != 537_885_600 {
		t.Fatal("incorrect lot volume for indicators", ks, err)
	}
}

func TestStandardIndexIntradayVolumeUnavailable(t *testing.T) {
	c := marketTestClient(t, func(_, b []byte) []byte {
		if !protocol.IsIntraday(b[8]) {
			t.Error("expected an intraday request", b)
		}
		return completionStandardBars([]uint32{20261009}, []uint16{571}, true)
	}, nil)
	for _, tc := range []struct {
		code string
		want int64
	}{
		{"sh000001", 0},
		{"sz399001", 0},
		{"bj899050", 0},
		{"sh880001", 100},
	} {
		for _, category := range []uint8{protocol.TypeKlineMinute, protocol.TypeKlineMinute2,
			protocol.TypeKline5Minute, protocol.TypeKline15Minute, protocol.TypeKline30Minute, protocol.TypeKline60Minute} {
			r, err := c.GetKline(category, tc.code, 0, 1)
			if err != nil || r.Count != 1 {
				t.Fatal(tc.code, category, r, err)
			}
			k := r.List[0]
			if k.Volume != tc.want || k.Standard.Volume != 100 || k.Close != 10 ||
				k.Amount == nil || *k.Amount != 1000 || k.Standard.UpCount != 3 || k.Standard.DownCount != 2 {
				t.Fatal(tc.code, category, k, k.Standard)
			}
			if tc.want == 0 {
				ks, err := r.Klines()
				if err != nil || ks[0].Volume != 0 || r.List[0].Standard.Volume != 100 {
					t.Fatal("metric adapter must use the normalized volume", ks, err)
				}
			}
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
	}{{"sh019001", 100, 1}, {"sz131810", 100, 1}} {
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
