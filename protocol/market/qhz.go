package market

const (
	MarketQHZ   ID       = "qhz"
	ExchangeQHZ Exchange = 42
)

type qhz struct{ ignoreImport }

var _ Market = qhz{}

func (qhz) Info() MarketInfo {
	return MarketInfo{
		Prefix: "qhz", Name: "期货类指数", TimeZone: "Asia/Shanghai", CloseTime: "15:30",
		Routes: []RouteInfo{{ID: MarketQHZ, Exchange: ExchangeQHZ, Name: "期货类指数", Protocol: ProtocolExtended, CodeSource: CodeListExtended}},
	}
}

func (qhz) Resolve(code Code, _ InstrumentSnapshot) (Instrument, error) {
	letter, _, valid := symbolParts(code.Symbol, "")
	return validateInstrument(ExchangeQHZ, code, valid && letter)
}

func (qhz) Rules(i Instrument) (InstrumentRules, error) {
	if err := checkExchange(i.Exchange, ExchangeQHZ); err != nil {
		return InstrumentRules{}, err
	}
	rules := futuresRules()
	rules.Security = SecurityIndex
	rules.Trade.PriceDivisor = 0
	rules.Trade.Fields.Price = false
	return rules, nil
}

func init() { registerMarket(qhz{}) }
