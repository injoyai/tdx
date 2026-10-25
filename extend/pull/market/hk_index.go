// Package market 实现各市场的拉取 Unit（可插拔）。
// 港股指数市场（恒生系/中华系）：走扩展行情(7727) GetKline，市场编码 27。
package market

import (
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

// HKIndex 港股指数市场。
// 品种代码为字母/字母数字混合，如 HSI（恒指）、CES100、HZ5021。
// 恒生系与中华系指数按协议市场整体接入，不依赖品种 Category 细分。
type HKIndex struct{ exUnit }

var _ pull.Unit = (*HKIndex)(nil)

func init() {
	pull.Register(&HKIndex{exUnit{
		name:    protocol.MarketHKIndex,
		markets: []protocol.Exchange{protocol.ExchangeHI},
	}})
}
