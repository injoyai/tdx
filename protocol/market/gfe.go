package market

const (
	MarketGFE   ID       = "gfe"
	ExchangeGFE Exchange = 66
)

type gfe struct{ ignoreImport }

var _ Market = gfe{}

func (gfe) Info() MarketInfo {
	return MarketInfo{
		Prefix: "gfe", Name: "广州期货", TimeZone: "Asia/Shanghai", CloseTime: "15:30",
		Routes: []RouteInfo{{ID: MarketGFE, Exchange: ExchangeGFE, Name: "广州期货", Protocol: ProtocolExtended, CodeSource: CodeListExtended}},
	}
}

func (gfe) Resolve(code Code, _ InstrumentSnapshot) (Instrument, error) {
	letter, digit, valid := symbolParts(code.Symbol, "")
	return validateInstrument(ExchangeGFE, code, valid && letter && digit)
}

func (gfe) Rules(i Instrument) (InstrumentRules, error) {
	if err := checkExchange(i.Exchange, ExchangeGFE); err != nil {
		return InstrumentRules{}, err
	}
	return futuresRules(), nil
}

func init() { registerMarket(gfe{}) }
