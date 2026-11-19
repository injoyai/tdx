package market

// SecurityKind describes a security within its market.
type SecurityKind string

const (
	SecurityStock  SecurityKind = "stock"
	SecurityIndex  SecurityKind = "index"
	SecurityFund   SecurityKind = "fund"
	SecurityBond   SecurityKind = "bond"
	SecurityBlock  SecurityKind = "block"
	SecurityFuture SecurityKind = "future"
	SecurityOther  SecurityKind = "other"
)

// StandardRules returns the common standard protocol rules used by raw decoders.
func StandardRules() InstrumentRules { return standardSecurityInfo(SecurityOther) }

func standardSecurityInfo(kind SecurityKind) InstrumentRules {
	info := InstrumentRules{
		Protocol:       ProtocolStandard,
		Security:       kind,
		KlinePeriods:   AllPeriods,
		RangePeriods:   1 << TypeKlineMinute,
		DayFields:      Fields{Price: true, Volume: true, Amount: true},
		IntradayFields: Fields{Price: true, Volume: true, Amount: true},
		LongFields:     Fields{Price: true, Volume: true, Amount: true},
		Quote:          QuoteRule{Supported: true, Fields: Fields{Price: true, Volume: true}},
		Kind:           KindStock,
		Day:            KlineInfo{TypeKlineDay, SharesPerLot}, Minute: KlineInfo{TypeKlineMinute, SharesPerLot},
		LongVolumeScale:             SharesPerLot,
		TimeShare:                   TickRule{Supported: true, Fields: Fields{Price: true, Volume: true}, PriceDivisor: 1},
		Trade:                       TickRule{Supported: true, Fields: Fields{Price: true, Volume: true}, PriceDivisor: 1},
		standardMinuteVolumeDivisor: SharesPerLot,
		// 股本为万股，资产、收入和现金流等金额以千元编码。
		financeSharesScale: 10_000, financeAmountScale: 1_000,
	}
	info.setTickVolume(SharesPerLot)
	switch kind {
	case SecurityStock:
		info.SplitAuction = true
	case SecurityIndex:
		info.Kind = KindIndex
		info.Minute.VolumeScale = 0
		info.IntradayFields.Volume = false
		info.TimeShare = TickRule{Supported: true, Fields: Fields{Price: true, Amount: true}, AmountScale: 10_000, PriceDivisor: 1}
		info.Trade = TickRule{Supported: true, Fields: Fields{Price: true, Amount: true}, AmountScale: 100, PriceDivisor: 1}
	case SecurityBlock:
		info.Kind = KindIndex
		info.Day.VolumeScale, info.Minute.VolumeScale, info.LongVolumeScale = 1, 1, 1
	case SecurityBond:
		info.setTickVolume(BondsPerLot)
		info.Minute.VolumeScale = BondsPerLot
		info.Day.VolumeScale, info.LongVolumeScale = 100*BondsPerLot, 100*BondsPerLot
		info.standardMinuteVolumeDivisor = 1
		info.TimeShare.PriceDivisor, info.Trade.PriceDivisor = 100, 10
		info.Quote.UseCodeDecimals = true
	case SecurityFund:
		info.Trade.PriceDivisor = 10
		info.Quote.UseCodeDecimals = true
		info.SplitAuction = true
	case SecurityOther:
		info.Quote.UseCodeDecimals = true
	}
	return info
}
