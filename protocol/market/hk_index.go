package market

const (
	MarketHKIndex Market   = "hk/index"
	ExchangeHI    Exchange = 27 // 扩展行情
)

func init() {
	def := extendedDefinition(ExchangeHI, "hk", "港股指数", func(i Instrument) (Instrument, bool) {
		return upperSymbol(i, "")
	})
	def.sharedPrefix, def.security = true, SecurityIndex
	def.bareSymbols = []string{"HSI", "HKL", "VHSI", "CES100", "GEM"}
	def.info.TimeZone, def.info.CloseTime = "Asia/Hong_Kong", "16:10"
	// 数量槽位用于金额，K 线金额以原币种的百万为单位。
	def.info.extendedKlineAmountScale = 1_000_000
	registerMarket(MarketHKIndex, def)
}
