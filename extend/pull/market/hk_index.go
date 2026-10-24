// Package market 实现各市场的拉取 Unit（可插拔）。
// 港股指数市场（恒生系/中华系）：走扩展行情(7727) GetKline，市场编码 27。
package market

import (
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

// 港股指数市场编码（扩展行情 ExInstrument.Market）。
// 2026-09 实测：市场27 含恒生系 HZ50xx/恒指 HSI/VHSI、中华系 CES100/CES120 等 331 只，
// 品种 Category 与类型无关（全 cat=5），无法细分；恒生指数家族天然成组，故整市场接入。
const marketHKIndex = 27 // 香港指数

// HKIndex 港股指数市场。
// 品种代码为字母/字母数字混合，如 HSI（恒指）、CES100、HZ5021。
type HKIndex struct{ exUnit }

var _ pull.Unit = (*HKIndex)(nil)

func init() {
	pull.Register(&HKIndex{exUnit{
		name:    protocol.MarketHKIndex,
		markets: []uint8{marketHKIndex},
	}})
}
