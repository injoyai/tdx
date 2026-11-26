package market

const (
	MarketCSI   ID       = "csi"
	ExchangeCSI Exchange = 62
)

type csi struct{}

func (csi) Info() MarketInfo {
	return MarketInfo{
		Prefix: "csi", Name: "中证指数", TimeZone: "Asia/Shanghai", CloseTime: "15:30",
		Routes: []RouteInfo{{ID: MarketCSI, Exchange: ExchangeCSI, Name: "中证指数", Protocol: ProtocolExtended, CodeSource: CodeListExtended}},
	}
}

func (csi) Resolve(code Code, _ InstrumentSnapshot) (Instrument, error) {
	return validateInstrument(ExchangeCSI, code, len(code.Symbol) == 6 && isDigits(code.Symbol))
}

func (csi) Rules(i Instrument) (InstrumentRules, error) {
	if err := checkExchange(i.Exchange, ExchangeCSI); err != nil {
		return InstrumentRules{}, err
	}
	return extendedRules(SecurityIndex), nil
}

func init() { registerMarket(csi{}) }
