// Package market 实现各市场的拉取 Unit（可插拔）。
// 港股市场：走扩展行情(7727) ExBars 拉取日线/分钟线。
package market

import (
	"github.com/injoyai/tdx/extend/pull"
	"github.com/injoyai/tdx/protocol"
)

// 港股市场编码（扩展行情 ExInstruments.Market）。
const marketHK = 31 // 香港交易所

// 港股日K category=TypeKlineDay(=9)（exhq_live_test.go 实测 ExBars(9,31,"00700",0,5)）；
// 分钟 category=8（TypeKlineMinute2）。与期货（日K=4）不同，故单独覆盖 dayCategory。
const (
	hkDayCategory    = protocol.TypeKlineDay     // 港股日K category（实测=9）
	hkMinuteCategory = protocol.TypeKlineMinute2 // 1分钟K category
)

// HK 港股市场。
// 品种代码为 5 位数字，如 00700（腾讯）。日K category=9、分钟 category=8。
type HK struct{ exUnit }

var _ pull.Unit = (*HK)(nil)

func init() {
	pull.Register(&HK{exUnit{
		name:           pull.MarketHK,
		markets:        []uint8{marketHK},
		dayCategory:    hkDayCategory,
		minuteCategory: hkMinuteCategory,
	}})
}
