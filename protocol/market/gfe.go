package market

const (
	MarketGFE   Market   = "gfe"
	ExchangeGFE Exchange = 66 // 扩展行情
)

func init() { registerMarket(MarketGFE, futuresDefinition(ExchangeGFE, "gfe", "广州期货")) }
