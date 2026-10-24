// Package market 实现各市场的拉取 Unit（可插拔）。
// 港股市场：走扩展行情(7727) GetKline 拉取日线/分钟线。
package market

import (
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

// 港股市场编码（扩展行情 ExInstrument.Market）。
const marketHK = 31 // 香港交易所

// HK 港股市场。
// 品种代码为 5 位数字，如 00700（腾讯）；K 线配置由 protocol.MarketHK.Info 提供。
type HK struct{ exUnit }

var _ pull.Unit = (*HK)(nil)

func init() {
	pull.Register(&HK{exUnit{
		name:    protocol.MarketHK,
		markets: []uint8{marketHK},
	}})
}
