package market

import "testing"

func TestHongKongMarketRoutes(t *testing.T) {
	m := prefixMarkets["hk"]
	info := m.Info()
	if info.Prefix != "hk" || len(info.Routes) != 3 {
		t.Fatalf("unexpected Hong Kong market: %+v", info)
	}
	for _, ex := range []Exchange{ExchangeHK, ExchangeHKFund, ExchangeHI} {
		route, ok := ex.Route()
		if !ok || route.Protocol != ProtocolExtended || ex.String() != "hk" {
			t.Fatalf("missing Hong Kong route %d: %+v", ex, route)
		}
	}
	rules, err := m.Rules(Instrument{Exchange: ExchangeHI, Code: "HSI"})
	if err != nil {
		t.Fatal(err)
	}
	page := rules.Kline(TypeKlineDay, KlinePage)
	ranged := rules.Kline(TypeKlineMinute, KlineRange)
	if page.Category != TypeKlineDay2 || page.VolumeScale != 0 ||
		page.AmountField != AmountInAmount || page.AmountScale != 1_000_000 {
		t.Errorf("unexpected page rule: %+v", page)
	}
	if ranged.Category != TypeKlineMinute2 || ranged.VolumeScale != 0 ||
		ranged.AmountField != AmountInPosition || ranged.AmountScale != 1_000_000 {
		t.Errorf("unexpected range rule: %+v", ranged)
	}
}

func TestIndexTickFields(t *testing.T) {
	i, err := ResolveInstrument("sh.000001", nil)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := i.Rules()
	if !ok || r.Security != SecurityIndex || r.Quote.VolumeScale != 100 ||
		r.TimeShare.AmountScale != 10_000 || r.Trade.AmountScale != 100 {
		t.Fatalf("unexpected index rules: %+v", r)
	}
}

func TestDeclaredPeriodsAndAliases(t *testing.T) {
	rules := standardSecurityInfo(SecurityStock)
	rules.KlinePeriods = 1 << TypeKlineDay
	for _, category := range []uint8{TypeKlineDay, TypeKlineDay2} {
		if r := rules.Kline(category, KlinePage); !r.Supported || !r.Fields.Volume || r.Category != TypeKlineDay {
			t.Fatalf("daily alias %d: %+v", category, r)
		}
	}
	for _, category := range []uint8{TypeKlineMinute, TypeKlineWeek, 12, 255} {
		r := rules.Kline(category, KlinePage)
		if r.Supported || r.Fields != (Fields{}) {
			t.Fatalf("undeclared period %d: %+v", category, r)
		}
	}
	if !rules.Kline(TypeKlineMinute2, KlineRange).Supported {
		t.Fatal("minute range must use its own declared period")
	}
	if rules.Kline(TypeKline5Minute, KlineRange).Supported ||
		rules.Kline(TypeKlineDay, KlineMode(255)).Supported {
		t.Fatal("accepted an unsupported layout or range period")
	}
}

func TestMarketQueryCapabilities(t *testing.T) {
	for _, tc := range []struct {
		code       string
		ticks      bool
		dayAmount  bool
		dayVolume  bool
		tradePrice bool
	}{
		{"sh.000001", true, true, true, true},
		{"sh.600000", true, true, true, true},
		{"00823.49", true, true, true, true},
		{"hk.HSI", false, true, false, false},
		{"wi.A_IXIC", false, false, false, false},
		{"cff.IF2609", true, false, true, true},
		{"qhz.IMCI", true, false, true, false},
		{"us.AAPL", true, true, true, true},
	} {
		t.Run(tc.code, func(t *testing.T) {
			i, err := ParseInstrument(tc.code)
			if err != nil {
				t.Fatal(err)
			}
			rules, ok := i.Rules()
			if !ok || rules.Quote.Supported != tc.ticks || rules.TimeShare.Supported != tc.ticks ||
				rules.Trade.Supported != tc.ticks || rules.Trade.Fields.Price != tc.tradePrice {
				t.Fatalf("unexpected query support: %+v", rules)
			}
			day := rules.Kline(TypeKlineDay, KlinePage)
			if !day.Supported || !day.Fields.Price || day.Fields.Amount != tc.dayAmount || day.Fields.Volume != tc.dayVolume {
				t.Fatalf("unexpected daily fields: %+v", day)
			}
		})
	}
}
