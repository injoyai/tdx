package protocol

import "testing"

func TestParseInstrument(t *testing.T) {
	for _, tc := range []struct {
		input string
		ex    Exchange
		code  string
	}{
		{"sz000001", ExchangeSZ, "000001"}, {" SH.000001 ", ExchangeSH, "000001"},
		{"000001", ExchangeSZ, "000001"}, {"600000.SH", ExchangeSH, "600000"},
		{"bj899050", ExchangeBJ, "899050"}, {"899050", ExchangeBJ, "899050"},
		{"上海600000", ExchangeSH, "600000"}, {"00700", ExchangeHK, "00700"},
		{"HK.00700", ExchangeHK, "00700"}, {"hk.HSI", ExchangeHI, "HSI"},
		{"HKCES120", ExchangeHI, "CES120"}, {"HKL", ExchangeHI, "HKL"},
		{"HSI", ExchangeHI, "HSI"}, {"HSI.HK", ExchangeHI, "HSI"},
		{"hi.HZ5021", ExchangeHI, "HZ5021"}, {"SHOP", ExchangeUS, "SHOP"},
		{"us.bRk.B", ExchangeUS, "BRK.B"}, {"BRK.B", ExchangeUS, "BRK.B"},
		{"cff.IF2609", ExchangeCFF, "IF2609"}, {"CFF/IF2609", ExchangeCFF, "IF2609"},
		{"shf.cu2609", ExchangeSHF, "cu2609"}, {"IF2609.CFF", ExchangeCFF, "IF2609"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			i, err := ParseInstrument(tc.input)
			if err != nil || i.Exchange != tc.ex || i.Code != tc.code {
				t.Fatalf("ParseInstrument(%q)=%+v, %v", tc.input, i, err)
			}
			back, err := ParseInstrument(i.String())
			if err != nil || back != i {
				t.Fatal("canonical code did not round-trip", back, err)
			}
		})
	}
	for _, s := range []string{"", "hk.", "us.", "sh.12345", "sh.AAPL", "cff.IF", "IF2609", "us.A..B", "us.A-", "us.1234", "hk.123456", "hk.ABCDEFGHIJ", "us.ABCDEFGHIJ", "cff.IF$2609", "nq400001", "cffo123456", "gfeo123456", "us.股票"} {
		if i, err := ParseInstrument(s); err == nil {
			t.Errorf("accepted invalid/unsupported code %q as %s", s, i)
		}
	}
}

func TestInstrumentBlockMarket(t *testing.T) {
	for _, code := range []string{"sh880001", "sh881001", "880001"} {
		i, err := ParseInstrument(code)
		if err != nil {
			t.Fatal(err)
		}
		market, err := i.Market()
		if err != nil || market != MarketBlock {
			t.Fatal(code, market, err)
		}
	}
	for _, code := range []string{"sz880001", "bj881001", "sh000001"} {
		i, err := ParseInstrument(code)
		if err != nil {
			t.Fatal(err)
		}
		market, err := i.Market()
		if err != nil || market == MarketBlock {
			t.Fatal(code, market, err)
		}
	}
}
