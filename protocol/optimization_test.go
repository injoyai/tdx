package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/big"
	"math/rand"
	"reflect"
	"testing"
	"testing/iotest"
	"time"

	"github.com/injoyai/conv"
)

func TestAuctionRejectsEveryTruncatedPrefix(t *testing.T) {
	data := make([]byte, 2+2*16)
	binary.LittleEndian.PutUint16(data, 2)
	if r, err := MCallAuction.Decode(data); err != nil || len(r.List) != 2 {
		t.Fatal("complete auction", r, err)
	}
	for n := 0; n < len(data); n++ {
		if r, err := MCallAuction.Decode(data[:n]); err == nil || r != nil {
			t.Fatalf("accepted auction prefix %d", n)
		}
	}
}

func TestBlockFileChecked(t *testing.T) {
	data := make([]byte, 386+2*2813)
	binary.LittleEndian.PutUint16(data[384:], 2)
	for i, name := range []string{"First", "Second"} {
		row := data[386+i*2813:]
		copy(row, name)
		binary.LittleEndian.PutUint16(row[9:], 400)
		copy(row[13:], "1600000")
		copy(row[13+399*7:], "0000001")
	}
	r, err := ParseBlockFile(data)
	if err != nil || len(r) != 2 || r[0].Name != "First" || r[1].Name != "Second" ||
		!reflect.DeepEqual(r[0].Codes, []string{"1600000", "0000001"}) {
		t.Fatal("complete file", r, err)
	}
	for n := 0; n < len(data); n++ {
		if r, err := ParseBlockFile(data[:n]); !errors.Is(err, io.ErrUnexpectedEOF) || r != nil {
			t.Fatalf("accepted truncated file %d: %v", n, err)
		}
	}
	for _, row := range []int{386, 386 + 2813} {
		bad := bytes.Clone(data)
		binary.LittleEndian.PutUint16(bad[row+9:], 401)
		if r, err := ParseBlockFile(bad); err == nil || r != nil {
			t.Fatal("accepted oversized component region", r, err)
		}
	}
	empty := make([]byte, 386)
	if r, err := ParseBlockFile(empty); err != nil || r == nil || len(r) != 0 {
		t.Fatal("valid empty file changed", r, err)
	}
}

func TestReadFromFragmentedResyncAndFollowingFrame(t *testing.T) {
	frame := make([]byte, 19)
	copy(frame, []byte{0xb1, 0xcb, 0x74, 0})
	frame[4] = 0x1c
	binary.LittleEndian.PutUint16(frame[12:], 3)
	binary.LittleEndian.PutUint16(frame[14:], 3)
	copy(frame[16:], "abc")
	for _, junk := range [][]byte{nil, {1}, {1, 2}, {1, 2, 3}, {1, 2, 3, 4}, {0xb1, 0xcb, 0xb1, 0xcb, 0x74}} {
		stream := append(bytes.Clone(junk), frame...)
		stream = append(stream, frame...)
		r := iotest.OneByteReader(bytes.NewReader(stream))
		for i := 0; i < 2; i++ {
			got, err := ReadFrom(r)
			if err != nil || !bytes.Equal(got, frame) {
				t.Fatalf("junk %x frame %d: %x %v", junk, i, got, err)
			}
		}
	}
	for n := 0; n < len(frame); n++ {
		if got, err := ReadFrom(bytes.NewReader(frame[:n])); err == nil || got != nil {
			t.Fatalf("accepted frame prefix %d", n)
		}
	}
}

func TestIntegerSqrtFullRange(t *testing.T) {
	cases := []int64{-1, math.MinInt64, 0, 1, 2, 3, 4, 15, 16, 17, 1<<53 - 1, 1<<53 + 1, 3037000499*3037000499 - 1, 3037000499 * 3037000499, math.MaxInt64}
	rng := rand.New(rand.NewSource(3))
	for i := 0; i < 1000; i++ {
		cases = append(cases, rng.Int63())
	}
	for _, x := range cases {
		var want int64
		if x > 0 {
			want = new(big.Int).Sqrt(big.NewInt(x)).Int64()
		}
		if got := I64Sqrt(x); got != want {
			t.Fatalf("sqrt(%d)=%d, want %d", x, got, want)
		}
	}
}

func TestFloatUnitAdvancesUnits(t *testing.T) {
	for _, tc := range []struct {
		value, want float64
		unit        string
	}{
		{0, 0, ""}, {-1e9, -1e9, ""}, {10000, 10000, ""},
		{10001, 1.0001, "万"}, {1e8, 10000, "万"}, {1e9, 10, "亿"}, {1e12, 10000, "亿"},
	} {
		v, u := FloatUnit(tc.value)
		if v != tc.want || u != tc.unit {
			t.Fatalf("FloatUnit(%g)=%g%s", tc.value, v, u)
		}
	}
}

func TestMergeRetainsFieldsAndHandlesLargePeriod(t *testing.T) {
	at := time.Date(2025, 1, 2, 9, 31, 0, 0, Location())
	ks := Klines{
		{Time: at, Last: 9000, Open: 10000, High: 11000, Low: 9500, Close: 10500, Order: 2, Volume: 1, Amount: 1000000, UpCount: 3, DownCount: 4},
		{Time: at.Add(time.Minute), Last: 10500, Open: 10500, High: 12000, Low: 10000, Close: 11500, Order: 5, Volume: 2, Amount: 2000000, UpCount: 6, DownCount: 7},
	}
	original := []Kline{*ks[0], *ks[1]}
	for _, n := range []int{2, math.MaxInt} {
		got := ks.Merge(n)
		want := Kline{Time: at.Add(time.Minute), Last: 9000, Open: 10000, High: 12000, Low: 9500, Close: 11500, Order: 7, Volume: 3, Amount: 3000000, UpCount: 6, DownCount: 7}
		if len(got) != 1 || *got[0] != want {
			t.Fatalf("Merge(%d)=%+v", n, got)
		}
	}
	for _, n := range []int{5, math.MaxInt} {
		got := ks.Merge241(n)
		if got[1].Last != 9000 || got[1].Order != 7 || got[1].UpCount != 6 || got[1].DownCount != 7 {
			t.Fatalf("Merge241(%d) lost fields: %+v", n, got[1])
		}
	}
	if *ks[0] != original[0] || *ks[1] != original[1] {
		t.Fatal("merge mutated source")
	}
}

func TestEmptyDecoderJSONCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name   string
		decode func() (any, error)
		want   string
	}{
		{"code", func() (any, error) { return MCode.Decode([]byte{0, 0}) }, `{"Count":0,"List":null}`},
		{"kline", func() (any, error) { return MKline.Decode([]byte{0, 0}, KlineCache{}) }, `{"Count":0,"List":null}`},
		{"trade", func() (any, error) { return MTrade.Decode([]byte{0, 0}, TradeCache{}) }, `{"Count":0,"List":null}`},
		{"history trade", func() (any, error) { return MHistoryTrade.Decode(make([]byte, 6), TradeCache{}) }, `{"Count":0,"List":null}`},
		{"minute", func() (any, error) { return MMinute.Decode(make([]byte, 6)) }, `{"Count":0,"List":null}`},
		{"history minute", func() (any, error) { return MHistoryMinute.Decode(make([]byte, 6)) }, `{"Count":0,"List":null}`},
		{"quote", func() (any, error) { return MQuote.Decode(make([]byte, 4)) }, `[]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, e := tc.decode()
			if e != nil {
				t.Fatal(e)
			}
			got, e := json.Marshal(r)
			if e != nil || string(got) != tc.want {
				t.Fatalf("empty JSON=%s %v", got, e)
			}
		})
	}
}

func TestTradeClockAndDateValidation(t *testing.T) {
	for _, clock := range []uint16{0, 570, 690, 781, 1439, 1440, 65535} {
		data := []byte{1, 0, 0, 0, 10, 1, 2, 0, 0}
		binary.LittleEndian.PutUint16(data[2:], clock)
		r, err := MTrade.Decode(data, TradeCache{Date: "20250102", Code: "sz000001"})
		if clock >= 1440 {
			if err == nil || r != nil {
				t.Fatal("accepted invalid clock", clock)
			}
			continue
		}
		want := time.Date(2025, 1, 2, int(clock)/60, int(clock)%60, 0, 0, Location())
		if err != nil || r.List[0].Time != want {
			t.Fatal("clock changed", clock, r, err)
		}
	}
	if _, err := MTrade.Decode([]byte{1, 0, 0, 0, 10, 1, 2, 0, 0}, TradeCache{Date: "20250230"}); err == nil {
		t.Fatal("accepted invalid date")
	}
}

func FuzzEncodingLegacyCompatibility(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4})
	f.Add([]byte{0x81, 0, 0xff})
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 4096 {
			t.Skip()
		}
		if Uint16(b) != conv.Uint16(Reverse(b)) || Uint32(b) != conv.Uint32(Reverse(b)) {
			t.Fatal("integer result changed")
		}
		if !bytes.Equal(GBKToUTF8(b), legacyGBK(b)) {
			t.Fatal("GBK result changed")
		}
	})
}
