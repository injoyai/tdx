package pull

import "github.com/injoyai/tdx/v2/protocol"

// Market 市场标识枚举（Unit.Name() / Code.Market / Config.Codes 路由结果）。
//
// 定义在 pull 包（而非 market 子包）是因为注册表与 Code 模型都在本包，
// market 子包注册时直接引用常量，避免魔法字符串散落。
type Market string

// 内置市场。注意与 protocol.Exchange（uint8 协议层市场编码）是两套体系：
// 这里的 Market 是"拉取市场类别"（一个 Unit 一类，如期货覆盖多个交易所），
// 值同时用作存储目录名、日志前缀与增量去重键的一部分，勿随意改动。
const (
	// 枚举值即存储目录路径（Code.DirName 直接返回枚举值），
	// 两级「地区/资产」形式，与磁盘布局完全一致。
	MarketAStock  Market = "cn/stock"  // 沪深股票
	MarketIndex   Market = "cn/index"  // 沪深指数
	MarketEtfLof  Market = "cn/etf"    // ETF/LOF
	MarketBlock   Market = "cn/block"  // 板块指数
	MarketFuture  Market = "cn/future" // 期货（中金/郑商/大商/上期/广期等，走扩展行情）
	MarketHK      Market = "hk/stock"  // 港股主板（走扩展行情）
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

// VolumeUnit 是 K 线 API 解码后的单位，不适用于报价、分笔或本地文件。
type VolumeUnit string

const (
	VolumeDefault       VolumeUnit = "default"        // 未确认，暂按百股换算
	VolumeShares        VolumeUnit = "shares"         // 股
	VolumeHundredShares VolumeUnit = "hundred_shares" // 百股（协议中的手，不是交易所买卖单位）
)

// KlineInfo 记录请求类型及解码后的成交量单位。
type KlineInfo struct {
	Category   uint8
	VolumeUnit VolumeUnit
}

// ToStorageVolume 换算为 pull 存储数值；默认单位沿用旧的 ×100 行为。
// shortcut: 未确认单位暂按百股，取得该市场及周期的交叉验证样本后调整。
func (k KlineInfo) ToStorageVolume(volume int64) int64 {
	if k.VolumeUnit == VolumeShares {
		return volume
	}
	return ToShares(volume)
}

// MarketInfo 是内置市场的日线和 1 分钟线配置。
type MarketInfo struct {
	Protocol MarketProtocol
	Kind     string // 标准行情的解码分类（KindStock / KindIndex）；扩展行情不用此字段
	Day      KlineInfo
	Minute   KlineInfo
}

// Info 按已有市场枚举取得配置副本；自定义市场没有内置配置时返回 false。
func (m Market) Info() (MarketInfo, bool) {
	switch m {
	case MarketAStock, MarketEtfLof:
		return MarketInfo{
			Protocol: ProtocolStandard,
			Kind:     protocol.KindStock,
			Day:      KlineInfo{protocol.TypeKlineDay, VolumeHundredShares},
			Minute:   KlineInfo{protocol.TypeKlineMinute, VolumeHundredShares},
		}, true
	case MarketIndex, MarketBlock:
		// GetIndex 的解码器在日线、分钟线中均已将成交量乘 100。
		return MarketInfo{
			Protocol: ProtocolStandard,
			Kind:     protocol.KindIndex,
			Day:      KlineInfo{protocol.TypeKlineDay, VolumeShares},
			Minute:   KlineInfo{protocol.TypeKlineMinute, VolumeShares},
		}, true
	case MarketHK:
		return MarketInfo{
			Protocol: ProtocolExtended,
			Day:      KlineInfo{protocol.TypeKlineDay2, VolumeHundredShares},
			Minute:   KlineInfo{protocol.TypeKlineMinute2, VolumeDefault},
		}, true
	case MarketFuture:
		return MarketInfo{
			Protocol: ProtocolExtended,
			Day:      KlineInfo{protocol.TypeKlineDay2, VolumeDefault},
			Minute:   KlineInfo{protocol.TypeKlineMinute2, VolumeDefault},
		}, true
	case MarketHKIndex, MarketUS:
		return MarketInfo{
			Protocol: ProtocolExtended,
			Day:      KlineInfo{protocol.TypeKlineDay, VolumeDefault},
			Minute:   KlineInfo{protocol.TypeKlineMinute2, VolumeDefault},
		}, true
	default:
		return MarketInfo{}, false
	}
}
