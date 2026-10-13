package protocol

import (
	"encoding/binary"
	"encoding/hex"
	"testing"
)

func TestCheckedDecodersRejectTruncatedRecords(t *testing.T) {
	code := make([]byte, 31)
	binary.LittleEndian.PutUint16(code, 1)
	copy(code[2:], "600000")
	code[22] = 2
	kline := make([]byte, 18)
	binary.LittleEndian.PutUint16(kline, 1)
	binary.LittleEndian.PutUint32(kline[2:], 20250102)
	copy(kline[6:], []byte{10, 2, 4, 0x41})
	index := append(append([]byte(nil), kline...), 3, 0, 5, 0)
	quote := make([]byte, 60)
	binary.LittleEndian.PutUint16(quote[2:], 1)
	quote[4] = ExchangeSH.Uint8()
	copy(quote[5:], "600000")
	quote[13] = 10
	for _, tc := range []struct {
		name   string
		data   []byte
		decode func([]byte) error
	}{
		{"codes", code, func(b []byte) error { _, err := MCode.Decode(b); return err }},
		{"klines", kline, func(b []byte) error { _, err := MKline.Decode(b, KlineCache{Type: TypeKlineDay}); return err }},
		{"index", index, func(b []byte) error {
			_, err := MKline.Decode(b, KlineCache{Type: TypeKlineDay, Kind: KindIndex})
			return err
		}},
		{"quotes", quote, func(b []byte) error { _, err := MQuote.DecodeChecked(b); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.decode(tc.data); err != nil {
				t.Fatal("valid record:", err)
			}
			for n := 0; n < len(tc.data); n++ {
				if err := tc.decode(tc.data[:n]); err == nil {
					t.Fatalf("accepted truncated record at byte %d", n)
				}
			}
		})
	}
	bars, _ := MKline.Decode(index, KlineCache{Type: TypeKlineDay, Kind: KindIndex})
	b := bars.List[0]
	if b.Open != 10 || b.Close != 12 || b.High != 14 || b.Low != 9 || b.UpCount != 3 || b.DownCount != 5 {
		t.Fatalf("index fields changed: %+v", b)
	}
	q, _ := MQuote.DecodeChecked(quote)
	if len(q) != 1 || q[0].Code != "600000" || q[0].Kline.Close != 100 || q[0].BuyLevel[0].Price != 100 {
		t.Fatalf("quote fields changed: %+v", q)
	}
	if got := MQuote.Decode(quote[:len(quote)-1]); got != nil {
		t.Fatal("legacy API returned partial quotes")
	}
	// A valid first record must not mask a truncated second record.
	binary.LittleEndian.PutUint16(quote[2:], 2)
	if got, err := MQuote.DecodeChecked(quote); err == nil || got != nil {
		t.Fatal("partial response accepted", got, err)
	}
}

func TestCheckedDecodersRejectInvalidIntegers(t *testing.T) {
	for _, bad := range [][]byte{{0x80}, {0x80, 0x80, 0x80, 0x80, 0x10}, {0x80, 0x80, 0x80, 0x80, 0x80, 0}} {
		kline := append([]byte{1, 0, 0, 0, 0, 0}, bad...)
		if _, err := MKline.Decode(kline, KlineCache{Type: TypeKlineDay}); err == nil {
			t.Fatalf("accepted malformed Kline integer %x", bad)
		}
		quote := append(make([]byte, 13), bad...)
		quote[2] = 1
		if _, err := MQuote.DecodeChecked(quote); err == nil {
			t.Fatalf("accepted malformed quote integer %x", bad)
		}
	}
}

func TestMetadataDecodersRejectTruncatedRecords(t *testing.T) {
	minute := []byte{1, 0, 0, 0, 0, 0, 10, 0, 5}
	gbbq := make([]byte, 11+29)
	gbbq[9] = 1
	copy(gbbq[12:18], "000001")
	binary.LittleEndian.PutUint32(gbbq[19:23], 20261012)
	gbbq[23] = 1
	cat := make([]byte, 2+152)
	cat[0] = 1
	content := make([]byte, 12+3)
	content[10] = 3
	copy(content[12:], "abc")
	for _, tc := range []struct {
		name   string
		data   []byte
		decode func([]byte) error
	}{
		{"minute", minute, func(b []byte) error { _, err := MMinute.Decode(b); return err }},
		{"history minute", minute, func(b []byte) error { _, err := MHistoryMinute.Decode(b); return err }},
		{"gbbq", gbbq, func(b []byte) error { _, err := MGbbq.Decode(b); return err }},
		{"company category", cat, func(b []byte) error { _, err := MCompanyCat.Decode(b); return err }},
		{"company content", content, func(b []byte) error { _, err := MCompanyContent.Decode(b); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.decode(tc.data); err != nil {
				t.Fatal("valid response:", err)
			}
			for n := 0; n < len(tc.data); n++ {
				if err := tc.decode(tc.data[:n]); err == nil {
					t.Fatalf("accepted truncation at byte %d", n)
				}
			}
		})
	}
	history, err := MHistoryMinute.Decode(minute)
	if err != nil || history.Count != 1 || history.List[0].Price != 100 || history.List[0].Number != 5 || history.List[0].Time != "09:31" {
		t.Fatal("valid minute fields changed", history, err)
	}
	for _, bad := range [][]byte{{0x80}, {0x80, 0x80, 0x80, 0x80, 0x10}} {
		b := append([]byte{1, 0, 0, 0, 0, 0}, bad...)
		if _, err := MMinute.Decode(b); err == nil {
			t.Fatal("accepted invalid minute integer", bad)
		}
		if _, err := MHistoryMinute.Decode(b); err == nil {
			t.Fatal("accepted invalid history integer", bad)
		}
	}
	if cats, err := MCompanyCat.Decode([]byte{0, 0}); err != nil || len(cats) != 0 {
		t.Fatal("valid empty directory", cats, err)
	}
	if text, err := MCompanyContent.Decode(content); err != nil || text != "abc" {
		t.Fatal(text, err)
	}
}

func TestCheckedQuoteCapturedSample(t *testing.T) {
	data, err := hex.DecodeString("0136020000303030303031320bb2124c56105987e6d10cf212b78fa801ae01293dc54e8bd740acb8670086ca1e0001af36ba0c4102b467b6054203a68a0184094304891992114405862685108d0100000000e8ff320b0136303030333859098005464502468defd10cc005bed2668e05be15804d8ba12cb3b13a0083c3034100badc029d014201bc990384f70443029da503b7af074403a6e501b9db044504a6e2028dd5048d050000000000005909")
	if err != nil {
		t.Fatal(err)
	}
	quotes, err := MQuote.DecodeChecked(data)
	if err != nil || len(quotes) != 2 {
		t.Fatal("captured quote decoding", len(quotes), err)
	}
	if quotes[0].Code != "000001" || quotes[1].Code != "600038" || quotes[0].Kline.Close != 12020 || quotes[1].Kline.Close != 3200 {
		t.Fatal("quote record boundaries or prices changed", quotes)
	}
	for n := 0; n < len(data); n++ {
		if _, err := MQuote.DecodeChecked(data[:n]); err == nil {
			t.Fatalf("accepted captured quote truncated at byte %d", n)
		}
	}
}
