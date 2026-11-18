package market

const (
	MarketUS   Market   = "us/stock"
	ExchangeUS Exchange = 74 // 扩展行情
)

func init() {
	def := extendedDefinition(ExchangeUS, "us", "美国股票", func(i Instrument) (Instrument, bool) {
		return upperSymbol(i, ".-")
	})
	def.info.TimeZone, def.info.CloseTime = "America/New_York", "20:00"
	def.info.Day.VolumeScale, def.info.Minute.VolumeScale = 1, 1
	def.info.LongVolumeScale, def.info.TickVolumeScale = 100, 1
	def.info.TradePriceDivisor = 1_000
	def.info.extendedKlineAmountScale = 1
	registerMarket(MarketUS, def)
}
