package market

import "strings"

const (
	MarketInternationalIndex ID       = "global/index"
	ExchangeWI               Exchange = 12
)

type wi struct{}

var _ Market = wi{}

func (wi) Info() MarketInfo {
	return MarketInfo{
		Prefix: "wi", Name: "国际指数", PreferBareSymbol: true,
		Routes: []RouteInfo{{ID: MarketInternationalIndex, Exchange: ExchangeWI, Name: "国际指数", Protocol: ProtocolExtended, CodeSource: CodeListExtended}},
	}
}

func (wi) Resolve(code Code, _ InstrumentSnapshot) (Instrument, error) {
	letter, _, valid := symbolParts(code.Symbol, "._")
	code.Symbol = strings.ToUpper(code.Symbol)
	return validateInstrument(ExchangeWI, code, valid && letter)
}

func (wi) Rules(i Instrument) (InstrumentRules, error) {
	if err := checkExchange(i.Exchange, ExchangeWI); err != nil {
		return InstrumentRules{}, err
	}
	return extendedRules(SecurityIndex), nil
}

func init() { registerMarket(wi{}) }
