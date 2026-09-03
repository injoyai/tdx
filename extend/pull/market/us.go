// Package market 实现各市场的拉取 Unit（可插拔）。
// 美股市场：走扩展行情(7727) ExBars 拉取日线/分钟线。
package market

import (
	"github.com/injoyai/tdx/extend/pull"
	"github.com/injoyai/tdx/protocol"
)

// 美股市场编码（扩展行情 ExInstruments.Market）。
const marketUS = 74 // 美国股票

// 美股日K category 与港股一致用 TypeKlineDay(=9)（参照港股实测），分钟用 TypeKlineMinute2(=8)。
const (
	usDayCategory    = protocol.TypeKlineDay     // 美股日K category
	usMinuteCategory = protocol.TypeKlineMinute2 // 1分钟K category
)

// US 美股市场。
// 品种代码为纯字母，如 AAPL。
type US struct{ exUnit }

var _ pull.Unit = (*US)(nil)

func init() {
	pull.Register(&US{exUnit{
		name:           pull.MarketUS,
		markets:        []uint8{marketUS},
		dayCategory:    usDayCategory,
		minuteCategory: usMinuteCategory,
	}})
}
