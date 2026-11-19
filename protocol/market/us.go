package market

import "strings"

const (
	MarketUS   ID       = "us/stock"
	ExchangeUS Exchange = 74
)

type us struct{}

var _ Market = us{}

func (us) Info() MarketInfo {
	return MarketInfo{
		Prefix: "us", Name: "美国股票", TimeZone: "America/New_York", CloseTime: "20:00",
		Routes: []RouteInfo{{ID: MarketUS, Exchange: ExchangeUS, Name: "美国股票", Protocol: ProtocolExtended, CodeSource: CodeListExtended}},
	}
}

func (us) Resolve(code Code, _ InstrumentSnapshot) (Instrument, error) {
	letter, _, valid := symbolParts(code.Symbol, ".-")
	code.Symbol = strings.ToUpper(code.Symbol)
	return validateInstrument(ExchangeUS, code, valid && letter)
}

func (us) Rules(i Instrument) (InstrumentRules, error) {
	if err := checkExchange(i.Exchange, ExchangeUS); err != nil {
		return InstrumentRules{}, err
	}
	return equityRules(1, 100), nil
}

func init() { registerMarket(us{}) }
