package market

const (
	MarketDCE   Market   = "dce"
	ExchangeDCE Exchange = 29 // 扩展行情
)

func init() { registerMarket(MarketDCE, futuresDefinition(ExchangeDCE, "dce", "大连期货")) }
