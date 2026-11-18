package protocol

import (
	"fmt"
	"math"
)

// Market 市场标识枚举（Unit.Name() / Code.Market / Config.Codes 路由结果）。
//
// 定义在协议层，统一 Client 和 pull 共享路由、周期与单位契约。
type Market string

// 内置市场。注意与 Exchange（uint8 协议层市场编码）是两套体系：
// 这里的 Market 是"拉取市场类别"（一个 Unit 一类，如期货覆盖多个交易所），
// 值同时用作存储目录名、日志前缀与增量去重键的一部分，勿随意改动。
const (
	// 枚举值即存储目录路径（Code.DirName 直接返回枚举值），
	// 两级「地区/资产」形式，与磁盘布局完全一致。
	MarketAStock  Market = "cn/stock"  // 沪深股票
	MarketIndex   Market = "cn/index"  // 沪深北标准指数
	MarketEtfLof  Market = "cn/etf"    // ETF/LOF
	MarketBond    Market = "cn/bond"   // 可转债
	MarketOther   Market = "cn/other"  // 其他国内标准品种（含尚未细分的普通债券）
	MarketBlock   Market = "cn/block"  // 板块指数
	MarketFuture  Market = "cn/future" // 期货（中金/郑商/大商/上期/广期等，走扩展行情）
	MarketHK      Market = "hk/stock"  // 港股主板（走扩展行情）
	MarketHKFund  Market = "hk/fund"   // 香港基金（含 ETF，扩展行情市场 49）
	MarketHKIndex Market = "hk/index"  // 港股指数（恒生系/中华系，扩展行情市场27）
	MarketUS      Market = "us/stock"  // 美股（股票/ETF/指数混合，走扩展行情；协议层无法区分）
)

// String 实现 fmt.Stringer。
func (m Market) String() string { return string(m) }

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
func (m MarketInfo) ExtendedTradePrice(ex Exchange, raw uint32) (float64, bool) {
	// 期货指数与期货合约共用市场，但期货指数的分笔价格尺度尚未确认。
	if m.Protocol != ProtocolExtended || m.TradePriceDivisor <= 0 || ex == ExchangeQHZ {
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

func (m MarketInfo) standardKlineVolume(volume int64, category uint8, kind string) int64 {
	if IsIntraday(category) || category == TypeKlineDay2 {
		volume /= m.standardMinuteVolumeDivisor
	}
	if kind == KindIndex {
		volume = ToShares(volume)
	}
	return volume
}

func standardMarketInfo(code string) MarketInfo {
	if i, err := ParseInstrument(code); err == nil {
		if m, err := i.Market(); err == nil {
			if info, ok := m.Info(); ok && info.Protocol == ProtocolStandard {
				return info
			}
		}
	}
	info, _ := MarketOther.Info()
	return info
}

// extendedTradeMarket 兼顾统一市场路由与底层分笔解码器已支持的市场编号。
func extendedTradeMarket(ex Exchange) Market {
	switch ex {
	case 48: // 旧港股协议市场
		return MarketHK
	case ExchangeCFFO, ExchangeCZCO, ExchangeDCEO, ExchangeSHFO, ExchangeGFEO, ExchangeSHO, ExchangeSZO:
		return MarketFuture
	default:
		m, _ := (Instrument{Exchange: ex}).Market()
		return m
	}
}

// Info 返回配置副本；未匹配市场使用国内标准行情默认配置，并返回 false 标明非内置市场。
func (m Market) Info() (MarketInfo, bool) {
	info := MarketInfo{MinutePriceDivisor: 1}
	switch m {
	case MarketAStock, MarketEtfLof, MarketOther, MarketBond, MarketIndex, MarketBlock:
		info.Protocol, info.TimeZone, info.CloseTime = ProtocolStandard, "Asia/Shanghai", "15:30"
		info.TradePriceDivisor = 1
		info.Kind = KindStock
		info.Day = KlineInfo{TypeKlineDay, SharesPerLot}
		info.Minute = KlineInfo{TypeKlineMinute, SharesPerLot}
		info.LongVolumeScale, info.TickVolumeScale = SharesPerLot, SharesPerLot
		info.standardMinuteVolumeDivisor = SharesPerLot
		// 标准财务协议的股本为万股，资产、收入和现金流等金额以千元编码。
		info.financeSharesScale, info.financeAmountScale = 10_000, 1_000
		switch m {
		case MarketIndex:
			info.Kind = KindIndex
			// 标准指数分钟线的数量槽位是成交额的缩放值。
			info.Minute.VolumeScale = 0
			// 分时为万元，分笔为百元；报价和 K 线另有独立量字段。
			info.MinuteAmountScale, info.TradeAmountScale = 10_000, 100
		case MarketBlock:
			info.Kind = KindIndex
			info.Minute.VolumeScale = 1
			info.Day.VolumeScale, info.LongVolumeScale = 1, 1
		case MarketBond:
			info.TickVolumeScale, info.Minute.VolumeScale = BondsPerLot, BondsPerLot
			// 日线及更长周期的解码量每单位为 100 手。
			info.Day.VolumeScale, info.LongVolumeScale = 100*BondsPerLot, 100*BondsPerLot
			info.standardMinuteVolumeDivisor = 1
			info.MinutePriceDivisor, info.TradePriceDivisor = 100, 10
			info.QuoteUsesCodeDecimals = true
		case MarketEtfLof:
			info.TradePriceDivisor = 10
			info.QuoteUsesCodeDecimals = true
		case MarketOther:
			info.QuoteUsesCodeDecimals = true
		}
	case MarketHK, MarketHKFund, MarketHKIndex, MarketUS, MarketFuture:
		info.Protocol = ProtocolExtended
		info.Day, info.Minute = KlineInfo{TypeKlineDay2, 1}, KlineInfo{TypeKlineMinute2, 1}
		info.LongVolumeScale, info.TickVolumeScale = 1, 1
		switch m {
		case MarketHK, MarketHKFund:
			info.TimeZone, info.CloseTime = "Asia/Hong_Kong", "16:10"
			info.TradePriceDivisor = 1_000
			info.Day.VolumeScale, info.Minute.VolumeScale = SharesPerLot, SharesPerLot
			info.LongVolumeScale = 10_000
			info.extendedKlineAmountScale = 1
		case MarketHKIndex:
			info.TimeZone, info.CloseTime = "Asia/Hong_Kong", "16:10"
			// 指数报文的量槽位用于金额，K 线金额以原币种的百万为单位。
			info.Day.VolumeScale, info.Minute.VolumeScale, info.LongVolumeScale, info.TickVolumeScale = 0, 0, 0, 0
			info.extendedKlineAmountScale = 1_000_000
		case MarketUS:
			info.TimeZone, info.CloseTime = "America/New_York", "20:00"
			info.TradePriceDivisor = 1_000
			info.LongVolumeScale = 100
			info.extendedKlineAmountScale = 1
		case MarketFuture:
			info.TimeZone, info.CloseTime = "Asia/Shanghai", "15:30"
			info.TradePriceDivisor = 1_000
		}
	default:
		info, _ := MarketOther.Info()
		return info, false
	}
	return info, true
}
