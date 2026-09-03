// Package market 实现各市场的拉取 Unit（可插拔）。
// 港股指数市场（恒生系/中华系）：走扩展行情(7727) ExBars，市场编码 27。
package market

import (
	"github.com/injoyai/tdx/extend/pull"
	"github.com/injoyai/tdx/protocol"
)

// 港股指数市场编码（扩展行情 ExInstruments.Market）。
// 2026-09 实测：市场27 含恒生系 HZ50xx/恒指 HSI/VHSI、中华系 CES100/CES120 等 331 只，
// 品种 Category 与类型无关（全 cat=5），无法细分；恒生指数家族天然成组，故整市场接入。
const marketHKIndex = 27 // 香港指数

// 港股指数日K/分钟 category 与港股主板一致（TypeKlineDay=9 / TypeKlineMinute2=8）。
const (
	hkIndexDayCategory    = protocol.TypeKlineDay     // 日K category
	hkIndexMinuteCategory = protocol.TypeKlineMinute2 // 1分钟K category
)

// HKIndex 港股指数市场。
// 品种代码为字母/字母数字混合，如 HSI（恒指）、CES100、HZ5021。
type HKIndex struct{ exUnit }

var _ pull.Unit = (*HKIndex)(nil)

func init() {
	pull.Register(&HKIndex{exUnit{
		name:           pull.MarketHKIndex,
		markets:        []uint8{marketHKIndex},
		dayCategory:    hkIndexDayCategory,
		minuteCategory: hkIndexMinuteCategory,
	}})
}
