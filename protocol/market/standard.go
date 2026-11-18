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

type securityRule struct {
	kind    SecurityKind
	matches func(string) bool
}

func standardDefinition(ex Exchange, prefix, name string, kinds ...securityRule) marketDefinition {
	return marketDefinition{
		exchange: ex, prefix: prefix, name: name,
		info: standardSecurityInfo(SecurityOther), securityInfo: standardSecurityInfo,
		parse: standardInstrumentCode, kinds: kinds,
	}
}

func standardSecurityInfo(kind SecurityKind) MarketInfo {
	info := MarketInfo{
		Protocol: ProtocolStandard, TimeZone: "Asia/Shanghai", CloseTime: "15:30",
		Kind: KindStock, TickVolumeScale: SharesPerLot,
		Day: KlineInfo{TypeKlineDay, SharesPerLot}, Minute: KlineInfo{TypeKlineMinute, SharesPerLot},
		LongVolumeScale: SharesPerLot, MinutePriceDivisor: 1, TradePriceDivisor: 1,
		standardMinuteVolumeDivisor: SharesPerLot,
		// 股本为万股，资产、收入和现金流等金额以千元编码。
		financeSharesScale: 10_000, financeAmountScale: 1_000,
	}
	switch kind {
	case SecurityStock:
		info.SplitAuction = true
	case SecurityIndex:
		info.Kind = KindIndex
		info.Minute.VolumeScale = 0
		info.MinuteAmountScale, info.TradeAmountScale = 10_000, 100
	case SecurityBlock:
		info.Kind = KindIndex
		info.Day.VolumeScale, info.Minute.VolumeScale, info.LongVolumeScale = 1, 1, 1
	case SecurityBond:
		info.TickVolumeScale, info.Minute.VolumeScale = BondsPerLot, BondsPerLot
		info.Day.VolumeScale, info.LongVolumeScale = 100*BondsPerLot, 100*BondsPerLot
		info.standardMinuteVolumeDivisor = 1
		info.MinutePriceDivisor, info.TradePriceDivisor = 100, 10
		info.QuoteUsesCodeDecimals = true
	case SecurityFund:
		info.TradePriceDivisor = 10
		info.QuoteUsesCodeDecimals = true
		info.SplitAuction = true
	case SecurityOther:
		info.QuoteUsesCodeDecimals = true
	}
	return info
}
