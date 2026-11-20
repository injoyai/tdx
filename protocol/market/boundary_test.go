package market

import "testing"

// TestRulesBoundary 固定共享构造函数的各市场公开结果，作为后续注册表改造的对照基线。
// 样本覆盖同一构造函数的目标市场、同市场其他路由，以及共用构造函数的其他市场。
func TestRulesBoundary(t *testing.T) {
	for _, tc := range []struct {
		code              string
		security          SecurityKind
		protocol          MarketProtocol
		day, minute, long int64
		splitAuction      bool
		tradeDivisor      float64
		tradeNature       TradeNature
		quoteSupported    bool
		dayAmount         bool
		extendedAmount    bool
	}{
		{"sh600000", SecurityStock, ProtocolStandard, 100, 100, 100, true, 1, TradeNeutral, true, true, true},
		{"sz000001", SecurityStock, ProtocolStandard, 100, 100, 100, true, 1, TradeNeutral, true, true, true},
		{"bj920001", SecurityStock, ProtocolStandard, 100, 100, 100, true, 1, TradeNeutral, true, true, true},
		{"sh000001", SecurityIndex, ProtocolStandard, 100, 0, 100, false, 1, TradeNeutral, true, true, true},
		{"sh510300", SecurityFund, ProtocolStandard, 100, 100, 100, true, 10, TradeNeutral, true, true, true},
		{"sz159915", SecurityFund, ProtocolStandard, 100, 100, 100, true, 10, TradeNeutral, true, true, true},
		{"sh110075", SecurityBond, ProtocolStandard, 1000, 10, 1000, false, 10, TradeNeutral, true, true, true},
		{"sh880001", SecurityBlock, ProtocolStandard, 1, 1, 1, false, 1, TradeNeutral, true, true, true},
		{"00700.31", SecurityStock, ProtocolExtended, 100, 100, 10000, false, 1000, TradeHK, true, true, true},
		{"00823.49", SecurityFund, ProtocolExtended, 100, 100, 10000, false, 1000, TradeHK, true, true, true},
		{"HSI", SecurityIndex, ProtocolExtended, 0, 0, 0, false, 0, TradeNeutral, false, true, true},
		{"us.AAPL", SecurityOther, ProtocolExtended, 1, 1, 100, false, 1000, TradeNeutral, true, true, true},
		{"cff.IF2609", SecurityFuture, ProtocolExtended, 1, 1, 1, false, 1000, TradeFutures, true, false, false},
		{"qhz.IMCI", SecurityIndex, ProtocolExtended, 1, 1, 1, false, 0, TradeFutures, true, false, false},
	} {
		t.Run(tc.code, func(t *testing.T) {
			i, err := ParseInstrument(tc.code)
			if err != nil {
				t.Fatal(err)
			}
			rules, ok := i.Rules()
			if !ok {
				t.Fatalf("%s 未提供行情规则", tc.code)
			}
			if rules.Security != tc.security || rules.Protocol != tc.protocol {
				t.Fatalf("security/protocol: %s/%s", rules.Security, rules.Protocol)
			}
			if rules.Day.VolumeScale != tc.day || rules.Minute.VolumeScale != tc.minute ||
				rules.LongVolumeScale != tc.long {
				t.Fatalf("volume scales: day=%d minute=%d long=%d",
					rules.Day.VolumeScale, rules.Minute.VolumeScale, rules.LongVolumeScale)
			}
			if rules.SplitAuction != tc.splitAuction {
				t.Fatalf("split auction: %v", rules.SplitAuction)
			}
			if rules.Trade.PriceDivisor != tc.tradeDivisor || rules.Trade.Nature != tc.tradeNature {
				t.Fatalf("trade: divisor=%v nature=%v", rules.Trade.PriceDivisor, rules.Trade.Nature)
			}
			if rules.Quote.Supported != tc.quoteSupported {
				t.Fatalf("quote supported: %v", rules.Quote.Supported)
			}
			if rules.DayFields.Amount != tc.dayAmount {
				t.Fatalf("day amount field: %v", rules.DayFields.Amount)
			}
			if got := rules.HasExtendedKlineAmount(TypeKlineDay2); got != tc.extendedAmount {
				t.Fatalf("extended amount: %v", got)
			}
		})
	}
}

// TestQuoteListCategoryDeclared 保证批量报价类别来自路由声明，而不是调用方猜测。
func TestQuoteListCategoryDeclared(t *testing.T) {
	for ex, want := range map[Exchange]uint8{
		ExchangeHK: 2, ExchangeHKFund: 2,
		ExchangeCFF: 3, ExchangeQHZ: 3,
		ExchangeUS: 0, ExchangeSH: 0,
	} {
		route, ok := ex.Route()
		if !ok {
			t.Fatalf("市场 %d 未注册", ex)
		}
		if route.QuoteListCategory != want {
			t.Fatalf("市场 %d 报价类别=%d，期望 %d", ex, route.QuoteListCategory, want)
		}
	}
}
