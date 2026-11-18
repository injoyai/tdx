package market

const (
	MarketCFF   Market   = "cff"
	ExchangeCFF Exchange = 47 // 扩展行情
)

func init() { registerMarket(MarketCFF, futuresDefinition(ExchangeCFF, "cff", "中金期货")) }
