package model

import "github.com/injoyai/tdx/v2/protocol"

// Code 统一的代码模型，跨市场通用。
type Code struct {
	Market protocol.Market // 市场标识，如 protocol.MarketAStock/protocol.MarketHK/protocol.MarketUS/protocol.MarketFuture；用于路由同步与隔离证券身份
	Code   string          // 规范代码，标准市场带 sh/sz/bj；其他市场如 00700 / AAPL / cff/IF2609
	Name   string          // 名称（可选）
}

// Key 返回带前缀的证券键，如 sh600000、hk.00700、hk.HSI、US.AAPL 或
// cn/future.cff/IF2609。用于日志和进度显示，存储文件名由具体后端决定。
func (c Code) Key() string {
	switch c.Market {
	case protocol.MarketAStock, protocol.MarketIndex, protocol.MarketEtfLof, protocol.MarketBlock, protocol.MarketBond, protocol.MarketOther:
		// 标准行情代码自带交易所前缀（sh/sz/bj），直接使用
		return c.Code
	case protocol.MarketHK, protocol.MarketHKIndex:
		return protocol.ExchangeHK.String() + "." + c.Code
	case protocol.MarketUS:
		return "US." + c.Code
	default:
		return c.Market.String() + "." + c.Code
	}
}
