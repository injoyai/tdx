package market

import (
	"fmt"
	"math"
)

// MarketProtocol 行情协议，分别对应标准行情 7709 和扩展行情 7727。
type MarketProtocol string

const (
	ProtocolStandard MarketProtocol = "standard"
	ProtocolExtended MarketProtocol = "extended"
)

// KlineInfo 记录请求类型及原始解码量转为最小单位的倍率。
type KlineInfo struct {
	Category    uint8
	VolumeScale int64 // 转为股数或张数的倍率，0 表示协议不提供成交量
}

// ToStorageVolume 换算为最小单位；禁止零/负倍率、负数量和整数溢出。
func (k KlineInfo) ToStorageVolume(volume int64) (int64, error) {
	if k.VolumeScale <= 0 || volume < 0 || volume > math.MaxInt64/k.VolumeScale {
		return 0, fmt.Errorf("成交量 %d 无法按倍率 %d 转换为最小单位", volume, k.VolumeScale)
	}
	return volume * k.VolumeScale, nil
}

// KlineMode identifies the response layout used for a K-line query.
type KlineMode uint8

const (
	KlinePage KlineMode = iota
	KlineRange
)

// KlineAmount identifies the field carrying the amount in a decoded record.
type KlineAmount uint8

const (
	NoKlineAmount KlineAmount = iota
	AmountInAmount
	AmountInPosition
)

// Fields describes the unified numeric fields provided by an SDK query.
type Fields struct {
	Price  bool
	Volume bool
	Amount bool
}

// Periods is the set of protocol categories accepted by an SDK query.
type Periods uint16

const AllPeriods Periods = (1 << (TypeKlineYear + 1)) - 1

func (p Periods) Supports(category uint8) bool {
	return category <= TypeKlineYear && p&(1<<category) != 0
}

// KlineRule describes the requested category and its decoded data.
type KlineRule struct {
	KlineInfo
	Supported    bool
	Fields       Fields
	AmountField  KlineAmount
	AmountScale  float64
	Kind         string // standard protocol decoder selection
	SplitAuction bool
}

// Kline maps a requested period and response layout to their protocol rules.
func (m InstrumentRules) Kline(category uint8, mode KlineMode) KlineRule {
	requested := category
	switch category {
	case TypeKlineDay, TypeKlineDay2:
		category = m.Day.Category
	case TypeKlineMinute, TypeKlineMinute2:
		category = m.Minute.Category
	}
	rule := KlineRule{
		KlineInfo: KlineInfo{Category: category, VolumeScale: m.KlineVolumeScale(category)},
		Kind:      m.Kind, SplitAuction: m.SplitAuction,
	}
	switch {
	case category == m.Day.Category:
		rule.Fields = m.DayFields
	case IsIntraday(category):
		rule.Fields = m.IntradayFields
	default:
		rule.Fields = m.LongFields
	}
	switch mode {
	case KlinePage:
		rule.Supported = m.KlinePeriods.Supports(category)
	case KlineRange:
		rule.Supported = m.RangePeriods.Supports(category) && (requested == TypeKlineMinute || requested == TypeKlineMinute2)
	}
	if !rule.Supported {
		rule.Fields = Fields{}
		return rule
	}
	if m.Protocol == ProtocolExtended && mode == KlineRange {
		rule.Fields.Amount = m.RangeAmountScale != 0
	}
	if !rule.Fields.Amount {
		return rule
	}
	if m.Protocol == ProtocolStandard {
		rule.AmountField, rule.AmountScale = AmountInAmount, 1
	} else if mode == KlineRange {
		rule.AmountField, rule.AmountScale = AmountInPosition, m.RangeAmountScale
	} else {
		rule.AmountField, rule.AmountScale = AmountInAmount, m.ExtendedKlineAmountScale(category)
	}
	return rule
}

// QuoteRule describes five-level quote quantity and price precision.
type QuoteRule struct {
	Supported       bool
	Fields          Fields
	VolumeScale     int64
	UseCodeDecimals bool
}

// TickRule describes the quantity slot and price encoding of a minute or trade tick.
// AmountScale is used when the quantity slot carries money.
type TickRule struct {
	Nature       TradeNature
	Supported    bool
	Fields       Fields
	VolumeScale  int64
	AmountScale  int64
	PriceDivisor float64
}

// InstrumentRules describes the wire fields and units for a resolved security.
// Supported declarations describe SDK normalization support. They do not promise
// that a server holds data for every security and requested date.
type InstrumentRules struct {
	KlinePeriods                          Periods
	RangePeriods                          Periods
	DayFields, IntradayFields, LongFields Fields
	Protocol                              MarketProtocol
	Security                              SecurityKind
	Kind                                  string // standard K-line decoder
	Day, Minute                           KlineInfo
	LongVolumeScale                       int64
	SplitAuction                          bool
	// TradeFallback allows pull to synthesize missing minute bars from
	// historical trades. It is independent from SplitAuction: a venue may split
	// the auction without permitting trade-based minute completion.
	TradeFallback    bool
	Quote            QuoteRule
	TimeShare, Trade TickRule

	// Extended K-line amount scales convert source fields to the quote currency.
	// Paged bars use Amount. Range bars use the Position slot.
	DayAmountScale, MinuteAmountScale, LongAmountScale, RangeAmountScale float64

	// StandardMinuteVolumeDivisor divides the raw standard intraday quantity slot.
	StandardMinuteVolumeDivisor int64
	FinanceSharesScale          float64
	FinanceAmountScale          float64
}

func (m *InstrumentRules) setTickVolume(scale int64) {
	m.Quote.VolumeScale = scale
	m.TimeShare.VolumeScale = scale
	m.Trade.VolumeScale = scale
}

// KlineVolumeScale 返回指定周期的成交量换算倍率。
func (m InstrumentRules) KlineVolumeScale(category uint8) int64 {
	switch {
	case category == TypeKlineDay || category == TypeKlineDay2:
		return m.Day.VolumeScale
	case IsIntraday(category):
		return m.Minute.VolumeScale
	default:
		return m.LongVolumeScale
	}
}

// HasKlineVolume 报告该市场及周期是否提供可用的成交数量。
func (m InstrumentRules) HasKlineVolume(category uint8) bool {
	rule := m.Kline(category, KlinePage)
	return rule.Supported && rule.Fields.Volume
}

// PriceScale 将标准报价解码值校准到代码表提供的价格精度。
func (m QuoteRule) PriceScale(decimal int8) float64 {
	if m.UseCodeDecimals {
		return math.Pow10(2 - int(decimal))
	}
	return 1
}

// ExtendedTradePrice 将扩展分笔价格转为报价单位，仅在价格尺度已确认时返回 true。
func (m InstrumentRules) ExtendedTradePrice(raw uint32) (float64, bool) {
	if !m.Trade.Supported || !m.Trade.Fields.Price || m.Protocol != ProtocolExtended || m.Trade.PriceDivisor <= 0 {
		return 0, false
	}
	return float64(raw) / m.Trade.PriceDivisor, true
}

// HasExtendedKlineAmount 报告该市场和周期的扩展协议金额是否可用。
func (m InstrumentRules) HasExtendedKlineAmount(category uint8) bool {
	rule := m.Kline(category, KlinePage)
	return rule.Supported && rule.Fields.Amount
}

// ExtendedKlineAmountScale 返回扩展 K 线金额转为原币种的倍率，0 表示协议不提供金额。
func (m InstrumentRules) ExtendedKlineAmountScale(category uint8) float64 {
	switch {
	case category == TypeKlineDay || category == TypeKlineDay2:
		return m.DayAmountScale
	case IsIntraday(category):
		return m.MinuteAmountScale
	case AllPeriods.Supports(category):
		return m.LongAmountScale
	default:
		return 0
	}
}

// StandardKlineVolume normalizes the standard wire quantity slot.
func (m InstrumentRules) StandardKlineVolume(volume int64, category uint8, kind string) int64 {
	if (IsIntraday(category) || category == TypeKlineDay2) && m.StandardMinuteVolumeDivisor > 0 {
		volume /= m.StandardMinuteVolumeDivisor
	}
	if kind == KindIndex {
		volume *= SharesPerLot
	}
	return volume
}

// StandardKlineBusinessVolume 将标准协议原始数量槽位按证券和周期规则换算为最小单位。
//
// 与 StandardKlineVolume 不同，分钟除数和倍率在一次换算中合并计算，不会先整除而丢失余数。
// 余数部分不足一个最小单位时按整数截断，其余情况保留全部数量；负量与溢出返回错误。
func (m InstrumentRules) StandardKlineBusinessVolume(volume int64, category uint8, kind string) (int64, error) {
	if volume < 0 {
		return 0, fmt.Errorf("成交量 %d 不能为负", volume)
	}
	scale := m.KlineVolumeScale(category)
	if scale <= 0 {
		return 0, nil
	}
	if kind == KindIndex {
		if scale > math.MaxInt64/SharesPerLot {
			return 0, fmt.Errorf("成交量 %d 的换算倍率溢出", volume)
		}
		scale *= SharesPerLot
	}
	divisor := int64(1)
	if IsIntraday(category) || category == TypeKlineDay2 {
		divisor = m.StandardMinuteVolumeDivisor
		if divisor <= 0 {
			return 0, fmt.Errorf("分钟成交量除数无效: %d", divisor)
		}
	}
	if divisor == 1 {
		if volume > math.MaxInt64/scale {
			return 0, fmt.Errorf("成交量 %d 无法按倍率 %d 转换为最小单位", volume, scale)
		}
		return volume * scale, nil
	}
	whole, rest := volume/divisor, volume%divisor
	if whole > math.MaxInt64/scale || rest > math.MaxInt64/scale {
		return 0, fmt.Errorf("成交量 %d 无法按倍率 %d 转换为最小单位", volume, scale)
	}
	out := whole * scale
	add := rest * scale / divisor
	if out > math.MaxInt64-add {
		return 0, fmt.Errorf("成交量 %d 无法按倍率 %d 转换为最小单位", volume, scale)
	}
	return out + add, nil
}

// FinanceScales returns the standard protocol's share and currency scales.
func (m InstrumentRules) FinanceScales() (shares, amount float64) {
	return m.FinanceSharesScale, m.FinanceAmountScale
}
