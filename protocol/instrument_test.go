package protocol

import (
	"errors"
	"strings"
	"testing"
)

func TestParseInstrument(t *testing.T) {
	for _, tc := range []struct {
		input string
		ex    Exchange
		code  string
	}{
		{"sz000001", ExchangeSZ, "000001"}, {" SH.000001 ", ExchangeSH, "000001"},
		{"000001", ExchangeSZ, "000001"}, {"600000.SH", ExchangeSH, "600000"},
		{"bj899050", ExchangeBJ, "899050"}, {"899050", ExchangeBJ, "899050"},
		{"上海600000", ExchangeSH, "600000"},
		{"00700.31", ExchangeHK, "00700"}, {"hk.HSI", ExchangeHI, "HSI"},
		{"02800.49", ExchangeHKFund, "02800"}, {"香港基金02800", ExchangeHKFund, "02800"},
		{"02800.香港基金", ExchangeHKFund, "02800"},
		{"HKCES120", ExchangeHI, "CES120"}, {"HKL", ExchangeHI, "HKL"},
		{"HSI", ExchangeHI, "HSI"}, {"HSI.HK", ExchangeHI, "HSI"},
		{"hk.HZ5021", ExchangeHI, "HZ5021"}, {"us.SHOP", ExchangeUS, "SHOP"},
		{"us.bRk.B", ExchangeUS, "BRK.B"},
		{"us.WIX", ExchangeUS, "WIX"}, {"WING.US", ExchangeUS, "WING"},
		{"WIX.US", ExchangeUS, "WIX"}, {"USB.Us", ExchangeUS, "USB"},
		{"US.BRK.B", ExchangeUS, "BRK.B"}, {"BRK.B.US", ExchangeUS, "BRK.B"},
		{"wi.A_IXIC", ExchangeWI, "A_IXIC"}, {"Wi.a_ndx", ExchangeWI, "A_NDX"},
		{"wi.A_SOX", ExchangeWI, "A_SOX"}, {"A_IXIC.12", ExchangeWI, "A_IXIC"},
		{"B_SDAX.K.WI", ExchangeWI, "B_SDAX.K"},
		{"cff.IF2609", ExchangeCFF, "IF2609"}, {"US.ANT", ExchangeUS, "ANT"},
		{"shf.cu2609", ExchangeSHF, "cu2609"}, {"IF2609.CFF", ExchangeCFF, "IF2609"},
		{"qhz.IMCI", ExchangeQHZ, "IMCI"}, {"IMCI.QHZ", ExchangeQHZ, "IMCI"}, {"T001.QHZ", ExchangeQHZ, "T001"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			i, err := ParseInstrument(tc.input)
			if err != nil || i.Exchange != tc.ex || i.Code != tc.code {
				t.Fatalf("ParseInstrument(%q)=%+v, %v", tc.input, i, err)
			}
			back, err := ParseInstrument(i.QueryCode())
			if err != nil || back != i {
				t.Fatal("canonical code did not round-trip", back, err)
			}
		})
	}
	for _, s := range []string{"", "hk.", "us.", "sh.12345", "sh.AAPL", "cff.IF", "IF2609", "us.A..B", "us.A-", "us.1234", "hk.123456", "hk.ABCDEFGHIJ", "us.ABCDEFGHIJ", "cff.IF$2609", "nq.400001", "cffo.123456", "gfeo.123456", "us.股票"} {
		if i, err := ParseInstrument(s); err == nil {
			t.Errorf("accepted invalid/unsupported code %q as %s", s, i)
		}
	}
	for _, s := range []string{"wi.", "wi.1234", "wi.A__IXIC", "wi.A_IXIC.", "wi.ABCDEFGHIJ", "qhz.1234", "qhz.IM_CI", "us.A_IXIC", "AAPL.330"} {
		if i, err := ParseInstrument(s); err == nil {
			t.Errorf("accepted invalid code %q as %s", s, i)
		}
	}
}

func TestParseInstrumentMarketPriority(t *testing.T) {
	for _, tc := range []struct {
		input string
		ex    Exchange
		code  string
	}{
		// 代码主体中的点不能被更短的市场前缀抢占。
		{"WIX.US", ExchangeUS, "WIX"}, {"us.WIX", ExchangeUS, "WIX"},
		{"USB.US", ExchangeUS, "USB"}, {"BRK.B.US", ExchangeUS, "BRK.B"},
		// 裸代码推断保持不变。
		{"000001", ExchangeSZ, "000001"}, {"us.SHOP", ExchangeUS, "SHOP"},
		// 裸品种符号与完整市场键。
		{"HKL", ExchangeHI, "HKL"},
		{"hk/fund.02800", ExchangeHKFund, "02800"},
		{"02800.hk/fund", ExchangeHKFund, "02800"},
		{"cff.IF2609", ExchangeCFF, "IF2609"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			i, err := ParseInstrument(tc.input)
			if err != nil || i.Exchange != tc.ex || i.Code != tc.code {
				t.Fatalf("ParseInstrument(%q)=%+v, %v", tc.input, i, err)
			}
			back, err := ParseInstrument(i.QueryCode())
			if err != nil || back != i {
				t.Fatal("canonical code did not round-trip", back, err)
			}
			ex, code, err := DecodeCode(tc.input)
			if err != nil || ex != tc.ex || code != tc.code {
				t.Fatalf("DecodeCode(%q)=%d %q, %v", tc.input, ex, code, err)
			}
		})
	}
}

func TestInstrumentRequiresMarketDot(t *testing.T) {
	for _, input := range []string{
		"ANT", "SHOP", "BRK.B", "WIX", "WING", "usANT", "usBRK.B", "us/ANT", "us/stockAAPL",
		"cffIF2609", "cff/IF2609", "shfcu2609", "QHZIMCI", "CSI000300", "WIA_IXIC",
		"hk/00823", "hk/stock00700", "hk/fund02800", "hk/indexHSI",
		"nq400001", "cffo123456", "gfeo123456",
	} {
		if i, err := ParseInstrument(input); err == nil {
			t.Errorf("ParseInstrument(%q) accepted %s", input, i)
		}
		if ex, code, err := DecodeCode(input); err == nil {
			t.Errorf("DecodeCode(%q) accepted %v %q", input, ex, code)
		}
	}
}

func TestParseInstrumentMarketNumberBounds(t *testing.T) {
	// 编号越界必须报错，不能按 uint8 回绕成其他市场。
	for _, input := range []string{"600000.257", "000001.999", "00700.256"} {
		i, err := ParseInstrument(input)
		if err == nil || i.Exchange != 0 || !strings.Contains(err.Error(), "0-255") {
			t.Fatalf("%q 未按编号越界报错: %+v, %v", input, i, err)
		}
	}
	// 合规但未接入的编号保持原始协议编号，不得映射到其他市场。
	if i, err := ParseInstrument("00700.44"); err == nil || i.Exchange != 0 || !strings.Contains(err.Error(), "44") {
		t.Fatalf("未接入编号未报错: %+v, %v", i, err)
	}
	for _, input := range []string{"600000.-1", "600000.257.SH"} {
		if i, err := ParseInstrument(input); err == nil {
			t.Fatalf("accepted invalid market number %q as %s", input, i)
		}
	}
	if _, err := ParseExchange("257"); err == nil || !strings.Contains(err.Error(), "0-255") {
		t.Fatalf("ParseExchange(257) 未按越界报错: %v", err)
	}
	if _, err := ParseExchange("-1"); err == nil {
		t.Fatal("ParseExchange(-1) accepted")
	}
}

func TestInstrumentBlockMarket(t *testing.T) {
	for _, code := range []string{"sh880001", "sh881001", "880001"} {
		i, err := ParseInstrument(code)
		if err != nil {
			t.Fatal(err)
		}
		market, err := i.Market()
		if err != nil || market != MarketSH || i.Kind() != SecurityBlock {
			t.Fatal(code, market, err)
		}
	}
	for _, code := range []string{"sz880001", "bj881001", "sh000001"} {
		i, err := ParseInstrument(code)
		if err != nil {
			t.Fatal(err)
		}
		market, err := i.Market()
		if err != nil || i.Kind() == SecurityBlock {
			t.Fatal(code, market, err)
		}
	}
}

func TestDomesticInstrumentMarkets(t *testing.T) {
	for _, tc := range []struct {
		code   string
		market MarketID
		kind   SecurityKind
	}{
		{"sh110075", MarketSH, SecurityBond}, {"sz123126", MarketSZ, SecurityBond},
		{"sh118001", MarketSH, SecurityBond}, {"sh019001", MarketSH, SecurityOther},
		{"sz131810", MarketSZ, SecurityOther}, {"sz000001", MarketSZ, SecurityStock},
		{"sh000001", MarketSH, SecurityIndex}, {"sh510300", MarketSH, SecurityFund},
		{"bj920001", MarketBJ, SecurityStock}, {"bj899050", MarketBJ, SecurityIndex},
	} {
		i, err := ParseInstrument(tc.code)
		if err != nil {
			t.Fatal(err)
		}
		if market, err := i.Market(); err != nil || market != tc.market || i.Kind() != tc.kind {
			t.Fatal(tc.code, market, i.Kind(), err)
		}
	}
	for _, market := range Markets() {
		ex, _ := market.Exchange()
		info, ok := (Instrument{Exchange: ex}).Rules()
		if !ok || info.Day.VolumeScale < 0 || info.Minute.VolumeScale < 0 || info.Quote.VolumeScale < 0 || info.LongVolumeScale < 0 {
			t.Fatalf("%s has invalid volume scale: %+v", market, info)
		}
	}
}

func TestExtendedTradePriceScale(t *testing.T) {
	for _, ex := range []Exchange{ExchangeHK, ExchangeHKFund, ExchangeUS, ExchangeCFF, ExchangeCZC, ExchangeDCE, ExchangeSHF, ExchangeGFE, ExchangeHI, ExchangeQHZ} {
		info, known := (Instrument{Exchange: ex}).Rules()
		if !known {
			t.Fatal(ex)
		}
		got, ok := info.ExtendedTradePrice(23660)
		if ex == ExchangeHI || ex == ExchangeQHZ {
			if ok {
				t.Fatal("unconfirmed price scale", ex, got)
			}
		} else if !ok || got != 23.66 {
			t.Fatal(ex, got, ok)
		}
	}
}

func TestHongKongFundWithKnownExchange(t *testing.T) {
	i, err := ParseInstrument("00823.49")
	if err != nil || i.Exchange != ExchangeHKFund || i.String() != "hk.00823" {
		t.Fatalf("known market 49: %+v, %v", i, err)
	}
}

func TestNumericHongKongRequiresSnapshot(t *testing.T) {
	for _, input := range []string{
		"00700", "hk00700", "HK.00700", "00700.HK", "hk09988", "02800", "02800.HK",
		"HK02849", "hk.02850", "HK03199", "hk03200", "hk03499", "hk03500",
		"hk09801", "hk09850", "hk82800", "hk83200", "hk.02800",
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := ParseInstrument(input); err == nil || !strings.Contains(err.Error(), "品种快照") {
				t.Fatalf("ambiguous wire market accepted: %v", err)
			}
			if _, _, err := DecodeCode(input); err == nil {
				t.Fatal("DecodeCode bypassed snapshot routing")
			}
			lookup, needed := LookupInstrument(input)
			if !needed || lookup.Candidates() != "31、49" {
				t.Fatalf("missing snapshot candidates: %+v", lookup)
			}
		})
	}
	snapshot := NewInstrumentSnapshot([]ExInstrument{
		{Market: 31, Code: "02800"}, {Market: 49, Code: "00823"},
	})
	for input, want := range map[string]Exchange{"HK.02800": ExchangeHK, "hk.00823": ExchangeHKFund} {
		calls := 0
		got, err := ResolveInstrument(input, func(lookup InstrumentLookup) (InstrumentSnapshot, error) {
			calls++
			if lookup.Candidates() != "31、49" {
				t.Fatal(lookup)
			}
			return snapshot, nil
		})
		if err != nil || got.Exchange != want || calls != 1 {
			t.Fatalf("%s: %+v, calls=%d, %v", input, got, calls, err)
		}
	}
	sentinel := errors.New("snapshot unavailable")
	if _, err := ResolveInstrument("hk.00823", func(InstrumentLookup) (InstrumentSnapshot, error) {
		return InstrumentSnapshot{}, sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("snapshot loader error lost: %v", err)
	}
	if _, needed := LookupInstrument("00823.49"); needed {
		t.Fatal("explicit wire market requested a snapshot")
	}
}
