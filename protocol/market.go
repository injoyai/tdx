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
	MarketIndex   Market = "cn/index"  // 沪深指数
	MarketEtfLof  Market = "cn/etf"    // ETF/LOF
	MarketBond    Market = "cn/bond"   // 可转债
	MarketOther   Market = "cn/other"  // 其他国内标准品种（含尚未细分的普通债券）
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
	VolumeScale int64 // 必须为正；国内未校准品种默认100，已确认股数/张数为1
}

// ToStorageVolume 换算为最小单位；禁止零/负倍率、负数量和整数溢出。
func (k KlineInfo) ToStorageVolume(volume int64) (int64, error) {
	if k.VolumeScale <= 0 || volume < 0 || volume > math.MaxInt64/k.VolumeScale {
		return 0, fmt.Errorf("成交量 %d 无法按倍率 %d 转换为最小单位", volume, k.VolumeScale)
	}
	return volume * k.VolumeScale, nil
}

// MarketInfo 是内置市场的日线和 1 分钟线配置。
type MarketInfo struct {
	Protocol        MarketProtocol
	Kind            string // 标准行情的解码分类（KindStock / KindIndex）；扩展行情不用此字段
	TickVolumeScale int64  // 报价（含五档）、分时和分笔的最小单位倍率，必须为正
	Day             KlineInfo
	Minute          KlineInfo
}

// Info 返回配置副本；未匹配市场使用国内标准行情默认配置，并返回 false 标明非内置市场。
func (m Market) Info() (MarketInfo, bool) {
	switch m {
	case MarketAStock, MarketEtfLof, MarketBond, MarketOther:
		// shortcut: 可转债和其他国内品种按用户指定默认100，逐接口校准后替换。
		return MarketInfo{
			Protocol:        ProtocolStandard,
			Kind:            KindStock,
			TickVolumeScale: SharesPerLot,
			Day:             KlineInfo{TypeKlineDay, SharesPerLot},
			Minute:          KlineInfo{TypeKlineMinute, SharesPerLot},
		}, true
	case MarketIndex, MarketBlock:
		// GetIndex 的解码器在日线、分钟线中均已将成交量乘 100。
		return MarketInfo{
			Protocol:        ProtocolStandard,
			Kind:            KindIndex,
			TickVolumeScale: SharesPerLot,
			Day:             KlineInfo{TypeKlineDay, 1},
			Minute:          KlineInfo{TypeKlineMinute, 1},
		}, true
	case MarketHK:
		return MarketInfo{
			Protocol:        ProtocolExtended,
			TickVolumeScale: 1,
			Day:             KlineInfo{TypeKlineDay2, SharesPerLot},
			Minute:          KlineInfo{TypeKlineMinute2, SharesPerLot},
		}, true
	case MarketFuture:
		return MarketInfo{
			Protocol:        ProtocolExtended,
			TickVolumeScale: 1,
			Day:             KlineInfo{TypeKlineDay2, 1},
			Minute:          KlineInfo{TypeKlineMinute2, 1},
		}, true
	case MarketUS:
		return MarketInfo{Protocol: ProtocolExtended, TickVolumeScale: 1, Day: KlineInfo{TypeKlineDay2, 1}, Minute: KlineInfo{TypeKlineMinute2, 1}}, true
	case MarketHKIndex:
		return MarketInfo{
			Protocol:        ProtocolExtended,
			TickVolumeScale: 1,
			// 此市场量字段含成交额口径，1仅表示原样保留，统一量接口仍拒绝。
			Day:    KlineInfo{TypeKlineDay2, 1},
			Minute: KlineInfo{TypeKlineMinute2, 1},
		}, true
	default:
		info, _ := MarketOther.Info()
		return info, false
	}
}
