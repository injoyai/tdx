package market

const (
	MarketInternationalIndex Market   = "global/index"
	ExchangeWI               Exchange = 12 // 扩展行情
)

func init() {
	def := extendedDefinition(ExchangeWI, "wi", "国际指数", func(i Instrument) (Instrument, bool) {
		return upperSymbol(i, "._")
	})
	def.preferBareSymbol, def.security = true, SecurityIndex
	// 该市场覆盖多个地区，调度由调用方配置。
	registerMarket(MarketInternationalIndex, def)
}
