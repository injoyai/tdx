package market

const (
	MarketQHZ   Market   = "qhz"
	ExchangeQHZ Exchange = 42 // 扩展行情
)

func init() {
	def := futuresDefinition(ExchangeQHZ, "qhz", "期货类指数")
	def.parse, def.security = futuresIndexCode, SecurityIndex
	// 期货类指数的分笔价格尺度尚未确认。
	def.info.TradePriceDivisor = 0
	registerMarket(MarketQHZ, def)
}
