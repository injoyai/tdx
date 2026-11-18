package market

const (
	MarketSHF   Market   = "shf"
	ExchangeSHF Exchange = 30          // 扩展行情
	ExchangeINE Exchange = ExchangeSHF // 上海能源与上期所共用协议编号
)

func init() { registerMarket(MarketSHF, futuresDefinition(ExchangeSHF, "shf", "上海期货")) }
