package market

const (
	MarketBJ   ID       = "bj"
	ExchangeBJ Exchange = 2
)

type bj struct{ ignoreImport }

var _ Market = bj{}

func (bj) Info() MarketInfo {
	return MarketInfo{
		Prefix: "bj", Name: "北京", TimeZone: "Asia/Shanghai", CloseTime: "15:30",
		Routes: []RouteInfo{{
			ID: MarketBJ, Exchange: ExchangeBJ, Name: "北京", Protocol: ProtocolStandard,
			CodeSource: CodeListZHBReport,
			// 标准协议不支持北交所代码分页，改取 zhb.zip 中的北交所名录。
			Report:  &CodeReport{Member: "tdxbjmore.cfg", Parser: ReportTdxBjMore},
			Session: DomesticSession,
		}},
	}
}

func (bj) Resolve(code Code, _ InstrumentSnapshot) (Instrument, error) {
	return validateInstrument(ExchangeBJ, code, len(code.Symbol) == 6 && isDigits(code.Symbol))
}

func (bj) Rules(i Instrument) (InstrumentRules, error) {
	if err := checkExchange(i.Exchange, ExchangeBJ); err != nil {
		return InstrumentRules{}, err
	}
	kind := SecurityOther
	switch {
	case isBJIndex(i.Code):
		kind = SecurityIndex
	case isBJStock(i.Code):
		kind = SecurityStock
	}
	return standardSecurityInfo(kind), nil
}

func init() { registerMarket(bj{}) }
