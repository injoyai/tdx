package market

const (
	MarketSHF   ID       = "shf"
	ExchangeSHF Exchange = 30
	ExchangeINE Exchange = ExchangeSHF // 上海能源与上期所共用协议编号
)

type shf struct{}

var _ Market = shf{}

func (shf) Info() MarketInfo {
	return MarketInfo{
		Prefix: "shf", Name: "上海期货", TimeZone: "Asia/Shanghai", CloseTime: "15:30",
		Routes: []RouteInfo{{ID: MarketSHF, Exchange: ExchangeSHF, Name: "上海期货", Protocol: ProtocolExtended, CodeSource: CodeListExtended}},
	}
}

func (shf) Resolve(code Code, _ InstrumentSnapshot) (Instrument, error) {
	letter, digit, valid := symbolParts(code.Symbol, "")
	return validateInstrument(ExchangeSHF, code, valid && letter && digit)
}

func (shf) Rules(i Instrument) (InstrumentRules, error) {
	if err := checkExchange(i.Exchange, ExchangeSHF); err != nil {
		return InstrumentRules{}, err
	}
	return futuresRules(), nil
}

func init() { registerMarket(shf{}) }
