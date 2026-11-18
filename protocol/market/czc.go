package market

const (
	MarketCZC   Market   = "czc"
	ExchangeCZC Exchange = 28 // 扩展行情
)

func init() { registerMarket(MarketCZC, futuresDefinition(ExchangeCZC, "czc", "郑州期货")) }
