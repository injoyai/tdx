// Package market 实现各市场的拉取 Unit（可插拔）。
// 美股市场：走扩展行情(7727) GetKline 拉取日线/分钟线。
package market

import (
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

// US 美股市场。
// 品种代码为纯字母，如 AAPL。
type US struct{ exUnit }

var _ pull.Unit = (*US)(nil)

func init() {
	pull.Register(&US{exUnit{
		name:    protocol.MarketUS,
		markets: []protocol.Exchange{protocol.ExchangeUS},
	}})
}
