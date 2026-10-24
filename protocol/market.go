package protocol

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

// KlineInfo 记录请求类型及原始解码量转为最小单位的倍率。
type KlineInfo struct {
	Category    uint8
	VolumeScale int64 // 1=已经是股/张，100=百股；0=尚未确认
}

// ToStorageVolume 换算为 pull 存储数值；未确认项保留旧的 ×100 行为。
// shortcut: 港股指数单位未确认，取得交叉验证样本后移除默认倍率。
func (k KlineInfo) ToStorageVolume(volume int64) int64 {
	if k.VolumeScale > 0 {
		return volume * k.VolumeScale
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
			Kind:     KindStock,
			Day:      KlineInfo{TypeKlineDay, SharesPerLot},
			Minute:   KlineInfo{TypeKlineMinute, SharesPerLot},
		}, true
	case MarketIndex, MarketBlock:
		// GetIndex 的解码器在日线、分钟线中均已将成交量乘 100。
		return MarketInfo{
			Protocol: ProtocolStandard,
			Kind:     KindIndex,
			Day:      KlineInfo{TypeKlineDay, 1},
			Minute:   KlineInfo{TypeKlineMinute, 1},
		}, true
	case MarketHK:
		return MarketInfo{
			Protocol: ProtocolExtended,
			Day:      KlineInfo{TypeKlineDay2, SharesPerLot},
			Minute:   KlineInfo{TypeKlineMinute2, SharesPerLot},
		}, true
	case MarketFuture:
		return MarketInfo{
			Protocol: ProtocolExtended,
			Day:      KlineInfo{TypeKlineDay2, 1},
			Minute:   KlineInfo{TypeKlineMinute2, 1},
		}, true
	case MarketUS:
		return MarketInfo{Protocol: ProtocolExtended, Day: KlineInfo{TypeKlineDay, 1}, Minute: KlineInfo{TypeKlineMinute2, 1}}, true
	case MarketHKIndex:
		return MarketInfo{
			Protocol: ProtocolExtended,
			Day:      KlineInfo{TypeKlineDay, 0},
			Minute:   KlineInfo{TypeKlineMinute2, 0},
		}, true
	default:
		return MarketInfo{}, false
	}
}
