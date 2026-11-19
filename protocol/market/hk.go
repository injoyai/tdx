package market

import (
	"fmt"
	"strings"
)

const (
	MarketHK       ID       = "hk/stock"
	MarketHKFund   ID       = "hk/fund"
	MarketHKIndex  ID       = "hk/index"
	ExchangeHK     Exchange = 31
	ExchangeHKFund Exchange = 49
	ExchangeHI     Exchange = 27
)

type hk struct{}

var _ Market = hk{}

func (hk) Info() MarketInfo {
	return MarketInfo{
		Prefix: "hk", Name: "香港交易所", TimeZone: "Asia/Hong_Kong", CloseTime: "16:10",
		Routes: []RouteInfo{
			{ID: MarketHK, Exchange: ExchangeHK, Name: "香港交易所", Protocol: ProtocolExtended, CodeSource: CodeListExtended, Snapshot: true},
			{ID: MarketHKFund, Exchange: ExchangeHKFund, Name: "香港基金", Protocol: ProtocolExtended, CodeSource: CodeListExtended, Snapshot: true},
			{ID: MarketHKIndex, Exchange: ExchangeHI, Name: "港股指数", Protocol: ProtocolExtended, CodeSource: CodeListExtended, BareSymbols: []string{"HSI", "HKL", "VHSI", "CES100", "GEM"}},
		},
	}
}

func (hk) Resolve(code Code, snapshot InstrumentSnapshot) (Instrument, error) {
	switch code.Exchange {
	case ExchangeHK, ExchangeHKFund:
		if len(code.Symbol) == 5 && isDigits(code.Symbol) {
			if !code.explicit {
				return snapshot.resolve(InstrumentLookup{Code: code.Symbol, Exchanges: []Exchange{ExchangeHK, ExchangeHKFund}, prefix: "hk"})
			}
			return Instrument{Exchange: code.Exchange, Code: code.Symbol}, nil
		}
		if code.Exchange == ExchangeHKFund || code.explicit {
			return validateInstrument(code.Exchange, code, false)
		}
		code.Exchange = ExchangeHI
	case ExchangeHI:
	default:
		return Instrument{}, fmt.Errorf("行情市场 %d 不属于 hk", code.Exchange)
	}
	letter, _, valid := symbolParts(code.Symbol, "")
	code.Symbol = strings.ToUpper(code.Symbol)
	return validateInstrument(ExchangeHI, code, valid && letter)
}

func (hk) Rules(i Instrument) (InstrumentRules, error) {
	switch i.Exchange {
	case ExchangeHK, ExchangeHKFund:
		rules := equityRules(SharesPerLot, 10_000)
		rules.Security = SecurityStock
		if i.Exchange == ExchangeHKFund {
			rules.Security = SecurityFund
		}
		rules.Trade.Nature = TradeHK
		return rules, nil
	case ExchangeHI:
		rules := extendedRules(SecurityIndex)
		rules.extendedKlineAmountScale = 1_000_000
		rules.DayFields = Fields{Price: true, Amount: true}
		rules.IntradayFields, rules.LongFields = rules.DayFields, rules.DayFields
		return rules, nil
	default:
		return InstrumentRules{}, fmt.Errorf("行情市场 %d 不属于 hk", i.Exchange)
	}
}

func init() { registerMarket(hk{}) }
