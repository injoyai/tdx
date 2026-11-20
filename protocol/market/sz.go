package market

const (
	MarketSZ   ID       = "sz"
	ExchangeSZ Exchange = 0
)

type sz struct{ ignoreImport }

var _ Market = sz{}

func (sz) Info() MarketInfo {
	return MarketInfo{
		Prefix: "sz", Name: "深圳", TimeZone: "Asia/Shanghai", CloseTime: "15:30",
		Routes: []RouteInfo{{ID: MarketSZ, Exchange: ExchangeSZ, Name: "深圳", Protocol: ProtocolStandard, CodeSource: CodeListStandard, Session: DomesticSession}},
	}
}

func (sz) Resolve(code Code, _ InstrumentSnapshot) (Instrument, error) {
	return validateInstrument(ExchangeSZ, code, len(code.Symbol) == 6 && isDigits(code.Symbol))
}

func (sz) Rules(i Instrument) (InstrumentRules, error) {
	if err := checkExchange(i.Exchange, ExchangeSZ); err != nil {
		return InstrumentRules{}, err
	}
	kind := SecurityOther
	switch {
	case isSZETF(i.Code):
		kind = SecurityFund
	case isSZIndex(i.Code):
		kind = SecurityIndex
	case isSZBond(i.Code):
		kind = SecurityBond
	case isSZStock(i.Code):
		kind = SecurityStock
	}
	return standardSecurityInfo(kind), nil
}

func init() { registerMarket(sz{}) }
