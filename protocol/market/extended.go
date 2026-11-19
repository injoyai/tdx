package market

func extendedRules(kind SecurityKind) InstrumentRules {
	return InstrumentRules{
		Protocol: ProtocolExtended, Security: kind,
		KlinePeriods: AllPeriods, RangePeriods: 1 << TypeKlineMinute2,
		DayFields: Fields{Price: true}, IntradayFields: Fields{Price: true}, LongFields: Fields{Price: true},
		TimeShare: TickRule{PriceDivisor: 1},
		Day:       KlineInfo{Category: TypeKlineDay2}, Minute: KlineInfo{Category: TypeKlineMinute2},
	}
}

func quantityRules(kind SecurityKind, dayVolume, longVolume int64) InstrumentRules {
	rules := extendedRules(kind)
	fields := Fields{Price: true, Volume: true}
	rules.DayFields, rules.IntradayFields, rules.LongFields = fields, fields, fields
	rules.Day.VolumeScale, rules.Minute.VolumeScale, rules.LongVolumeScale = dayVolume, dayVolume, longVolume
	rules.Quote = QuoteRule{Supported: true, Fields: fields, VolumeScale: 1}
	rules.TimeShare = TickRule{Supported: true, Fields: fields, PriceDivisor: 1, VolumeScale: 1}
	rules.Trade = TickRule{Supported: true, Fields: fields, PriceDivisor: 1_000, VolumeScale: 1}
	return rules
}

func equityRules(dayVolume, longVolume int64) InstrumentRules {
	rules := quantityRules(SecurityOther, dayVolume, longVolume)
	rules.DayFields.Amount, rules.IntradayFields.Amount, rules.LongFields.Amount = true, true, true
	rules.extendedKlineAmountScale = 1
	return rules
}

func futuresRules() InstrumentRules {
	rules := quantityRules(SecurityFuture, 1, 1)
	rules.Trade.Nature = TradeFutures
	return rules
}
