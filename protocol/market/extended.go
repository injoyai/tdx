package market

func extendedDefinition(ex Exchange, prefix, name string, parse func(Instrument) (Instrument, bool)) marketDefinition {
	return marketDefinition{
		exchange: ex, prefix: prefix, name: name, parse: parse,
		info: MarketInfo{
			Protocol: ProtocolExtended, MinutePriceDivisor: 1,
			Day: KlineInfo{Category: TypeKlineDay2}, Minute: KlineInfo{Category: TypeKlineMinute2},
		},
	}
}

func futuresDefinition(ex Exchange, prefix, name string) marketDefinition {
	def := extendedDefinition(ex, prefix, name, contractCode)
	def.security, def.tradeNature = SecurityFuture, futuresTradeNature
	def.info.TimeZone, def.info.CloseTime = "Asia/Shanghai", "15:30"
	def.info.Day.VolumeScale, def.info.Minute.VolumeScale = 1, 1
	def.info.LongVolumeScale, def.info.TickVolumeScale = 1, 1
	def.info.TradePriceDivisor = 1_000
	return def
}
