package market

const (
	MarketDCE   ID       = "dce"
	ExchangeDCE Exchange = 29
)

type dce struct{ ignoreImport }

var _ Market = dce{}

func (dce) Info() MarketInfo {
	return MarketInfo{
		Prefix: "dce", Name: "大连期货", TimeZone: "Asia/Shanghai", CloseTime: "15:30",
		Routes: []RouteInfo{{ID: MarketDCE, Exchange: ExchangeDCE, Name: "大连期货", Protocol: ProtocolExtended, CodeSource: CodeListExtended}},
	}
}

func (dce) Resolve(code Code, _ InstrumentSnapshot) (Instrument, error) {
	letter, digit, valid := symbolParts(code.Symbol, "")
	return validateInstrument(ExchangeDCE, code, valid && letter && digit)
}

func (dce) Rules(i Instrument) (InstrumentRules, error) {
	if err := checkExchange(i.Exchange, ExchangeDCE); err != nil {
		return InstrumentRules{}, err
	}
	return futuresRules(), nil
}

func init() { registerMarket(dce{}) }
