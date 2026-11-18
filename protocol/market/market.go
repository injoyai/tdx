// Package market defines securities, exchange routing, schedules, and units.
package market

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// Market identifies a registered market and is also its storage key.
type Market string

func (m Market) String() string { return string(m) }

type marketDefinition struct {
	exchange      Exchange
	prefix        string
	name          string
	info          MarketInfo
	securityInfo  func(SecurityKind) MarketInfo
	parse         func(Instrument) (Instrument, bool)
	parseExplicit func(Instrument) (Instrument, bool)
	lookup        snapshotRule
	kinds         []securityRule
	security      SecurityKind
	bareSymbols   []string

	// Shared aliases such as hk resolve to the primary market before parsing.
	sharedPrefix     bool
	preferBareSymbol bool
	discoverBlocks   bool
	tradeNature      func(uint16, uint32, int32) (int, string)
}

var (
	marketDefinitions   = map[Market]marketDefinition{}
	exchangeMarkets     = map[Exchange]Market{}
	markets             []Market
	instrumentExchanges []Exchange
	bareInstruments     = map[string]Exchange{}
	snapshotMarkets     []Market
	snapshotExchanges   = map[Exchange]bool{}
)

// registerMarket is called by the file defining a built-in market during init.
func registerMarket(m Market, def marketDefinition) {
	if _, exists := marketDefinitions[m]; exists {
		panic(fmt.Sprintf("重复定义市场 %s", m))
	}
	if _, exists := exchangeMarkets[def.exchange]; exists {
		panic(fmt.Sprintf("重复定义协议市场 %d", def.exchange))
	}
	if m == "" || m.String() != strings.ToLower(m.String()) ||
		def.prefix == "" || def.prefix != strings.ToLower(def.prefix) ||
		def.parse == nil || (def.info.Protocol != ProtocolStandard && def.info.Protocol != ProtocolExtended) {
		panic(fmt.Sprintf("无效市场定义 %q", m))
	}
	for _, symbol := range def.bareSymbols {
		symbol = strings.ToUpper(symbol)
		if _, exists := bareInstruments[symbol]; exists {
			panic(fmt.Sprintf("重复定义裸代码 %q", symbol))
		}
		bareInstruments[symbol] = def.exchange
	}
	marketDefinitions[m] = def
	exchangeMarkets[def.exchange] = m
	if def.lookup.code != nil {
		snapshotMarkets = append(snapshotMarkets, m)
		for _, ex := range def.lookup.exchanges {
			snapshotExchanges[ex] = true
		}
	}
	markets = append(markets, m)
	slices.Sort(markets)
	if !def.sharedPrefix {
		for _, ex := range instrumentExchanges {
			if ex.String() == def.prefix {
				panic(fmt.Sprintf("重复定义市场简称 %q", def.prefix))
			}
		}
		instrumentExchanges = append(instrumentExchanges, def.exchange)
		slices.SortFunc(instrumentExchanges, func(a, b Exchange) int {
			if d := len(b.String()) - len(a.String()); d != 0 {
				return d
			}
			return strings.Compare(a.String(), b.String())
		})
	}
}

// Markets returns the built-in market keys in lexical order.
func Markets() []Market { return slices.Clone(markets) }

// StandardExchanges returns exchanges supported by the standard wire protocol.
func StandardExchanges() []Exchange {
	var out []Exchange
	for _, m := range markets {
		def := marketDefinitions[m]
		if def.info.Protocol == ProtocolStandard {
			out = append(out, def.exchange)
		}
	}
	slices.Sort(out)
	return out
}

// Exchange returns the market's wire identifier.
func (m Market) Exchange() (Exchange, bool) {
	def, ok := marketDefinitions[m]
	return def.exchange, ok
}

// DiscoversBlocks reports whether this market includes sector indices from block_zs.dat.
func (m Market) DiscoversBlocks() bool { return marketDefinitions[m].discoverBlocks }

// Info returns the market's common settings. Use Instrument.Info for
// instrument-specific prices, amounts, and volume units.
func (m Market) Info() (MarketInfo, bool) {
	if def, ok := marketDefinitions[m]; ok {
		return def.info, true
	}
	return standardSecurityInfo(SecurityOther), false
}

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

// MarketInfo 是内置市场的时区、收盘时间、行情类型与单位配置。
type MarketInfo struct {
	Protocol                    MarketProtocol
	TimeZone                    string // 已确定的市场 IANA 时区，供调度使用；不用于解释行情源时间
	CloseTime                   string // 当地收盘边界 HH:MM；不含拉取延迟，美股包含盘后时段
	Kind                        string // 标准行情的解码分类（KindStock / KindIndex）；扩展行情不用此字段
	TickVolumeScale             int64  // 报价（含五档）、分时和分笔的成交量倍率，0 表示协议不提供成交量
	MinuteAmountScale           int64  // 分时数量槽位存金额时的换算倍率，0 表示仍按量处理
	TradeAmountScale            int64  // 分笔数量槽位存金额时的换算倍率，0 表示仍按量处理
	Day                         KlineInfo
	Minute                      KlineInfo
	LongVolumeScale             int64 // 周、月、季、年 K 线成交量倍率，0 表示协议不提供成交量
	QuoteUsesCodeDecimals       bool
	SplitAuction                bool    // 标准股票与 ETF/LOF 使用竞价拆分分钟线
	MinutePriceDivisor          float64 // 分时解码值再除以此倍率
	TradePriceDivisor           float64 // 标准分笔的附加除数，或扩展分笔原始价除数；0 表示未知
	extendedKlineAmountScale    float64
	standardMinuteVolumeDivisor int64
	financeSharesScale          float64
	financeAmountScale          float64
}

// KlineVolumeScale 返回指定周期的成交量换算倍率。
func (m MarketInfo) KlineVolumeScale(category uint8) int64 {
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
func (m MarketInfo) HasKlineVolume(category uint8) bool {
	return m.KlineVolumeScale(category) > 0
}

// QuotePriceScale 将标准报价解码值校准到代码表提供的价格精度。
func (m MarketInfo) QuotePriceScale(decimal int8) float64 {
	if m.QuoteUsesCodeDecimals {
		return math.Pow10(2 - int(decimal))
	}
	return 1
}

// ExtendedTradePrice 将扩展分笔价格转为报价单位，仅在价格尺度已确认时返回 true。
func (m MarketInfo) ExtendedTradePrice(raw uint32) (float64, bool) {
	if m.Protocol != ProtocolExtended || m.TradePriceDivisor <= 0 {
		return 0, false
	}
	return float64(raw) / m.TradePriceDivisor, true
}

// HasExtendedKlineAmount 报告该市场和周期的扩展协议金额是否可用。
func (m MarketInfo) HasExtendedKlineAmount(category uint8) bool {
	return m.ExtendedKlineAmountScale(category) > 0
}

// ExtendedKlineAmountScale 返回扩展 K 线金额转为原币种的倍率，0 表示协议不提供金额。
func (m MarketInfo) ExtendedKlineAmountScale(category uint8) float64 {
	switch category {
	case TypeKlineDay2, TypeKlineWeek, TypeKlineMonth, TypeKlineQuarter, TypeKlineYear:
		return m.extendedKlineAmountScale
	}
	if IsIntraday(category) {
		return m.extendedKlineAmountScale
	}
	return 0
}

// StandardKlineVolume normalizes the standard wire quantity slot.
func (m MarketInfo) StandardKlineVolume(volume int64, category uint8, kind string) int64 {
	if IsIntraday(category) || category == TypeKlineDay2 {
		volume /= m.standardMinuteVolumeDivisor
	}
	if kind == KindIndex {
		volume *= SharesPerLot
	}
	return volume
}

// FinanceScales returns the standard protocol's share and currency scales.
func (m MarketInfo) FinanceScales() (shares, amount float64) {
	return m.financeSharesScale, m.financeAmountScale
}
