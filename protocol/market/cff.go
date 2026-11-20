package market

const (
	MarketCFF   ID       = "cff"
	ExchangeCFF Exchange = 47
)

type cff struct{ ignoreImport }

var _ Market = cff{}

func (cff) Info() MarketInfo {
	return MarketInfo{
		Prefix: "cff", Name: "中金期货", TimeZone: "Asia/Shanghai", CloseTime: "15:30",
		Routes: []RouteInfo{{ID: MarketCFF, Exchange: ExchangeCFF, Name: "中金期货", Protocol: ProtocolExtended, CodeSource: CodeListExtended, QuoteListCategory: 3}},
	}
}

func (cff) Resolve(code Code, _ InstrumentSnapshot) (Instrument, error) {
	letter, digit, valid := symbolParts(code.Symbol, "")
	return validateInstrument(ExchangeCFF, code, valid && letter && digit)
}

func (cff) Rules(i Instrument) (InstrumentRules, error) {
	if err := checkExchange(i.Exchange, ExchangeCFF); err != nil {
		return InstrumentRules{}, err
	}
	return futuresRules(), nil
}

func init() { registerMarket(cff{}) }
