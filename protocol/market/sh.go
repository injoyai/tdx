package market

const (
	MarketSH   ID       = "sh"
	ExchangeSH Exchange = 1
)

type sh struct{ ignoreImport }

var _ Market = sh{}

func (sh) Info() MarketInfo {
	return MarketInfo{
		Prefix: "sh", Name: "上海", TimeZone: "Asia/Shanghai", CloseTime: "15:30",
		Routes: []RouteInfo{{ID: MarketSH, Exchange: ExchangeSH, Name: "上海", Protocol: ProtocolStandard, CodeSource: CodeListStandard, DiscoverBlocks: true}},
	}
}

func (sh) Resolve(code Code, _ InstrumentSnapshot) (Instrument, error) {
	return validateInstrument(ExchangeSH, code, len(code.Symbol) == 6 && isDigits(code.Symbol))
}

func (sh) Rules(i Instrument) (InstrumentRules, error) {
	if err := checkExchange(i.Exchange, ExchangeSH); err != nil {
		return InstrumentRules{}, err
	}
	kind := SecurityOther
	switch {
	case isBlock(i.Code):
		kind = SecurityBlock
	case isSHETF(i.Code):
		kind = SecurityFund
	case isSHIndex(i.Code):
		kind = SecurityIndex
	case isSHBond(i.Code):
		kind = SecurityBond
	case isSHStock(i.Code):
		kind = SecurityStock
	}
	return standardSecurityInfo(kind), nil
}

func init() { registerMarket(sh{}) }
