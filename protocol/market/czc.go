package market

const (
	MarketCZC   ID       = "czc"
	ExchangeCZC Exchange = 28
)

type czc struct{}

var _ Market = czc{}

func (czc) Info() MarketInfo {
	return MarketInfo{
		Prefix: "czc", Name: "郑州期货", TimeZone: "Asia/Shanghai", CloseTime: "15:30",
		Routes: []RouteInfo{{ID: MarketCZC, Exchange: ExchangeCZC, Name: "郑州期货", Protocol: ProtocolExtended, CodeSource: CodeListExtended}},
	}
}

func (czc) Resolve(code Code, _ InstrumentSnapshot) (Instrument, error) {
	letter, digit, valid := symbolParts(code.Symbol, "")
	return validateInstrument(ExchangeCZC, code, valid && letter && digit)
}

func (czc) Rules(i Instrument) (InstrumentRules, error) {
	if err := checkExchange(i.Exchange, ExchangeCZC); err != nil {
		return InstrumentRules{}, err
	}
	return futuresRules(), nil
}

func init() { registerMarket(czc{}) }
