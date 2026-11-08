package model

import "github.com/injoyai/tdx/v2/protocol"

// Code 统一的代码模型，跨市场通用。
type Code struct {
	Market protocol.Market // 市场标识，如 protocol.MarketAStock/protocol.MarketHK/protocol.MarketUS/protocol.MarketFuture；用于路由同步与隔离证券身份
	Code   string          // 规范代码，标准市场带 sh/sz/bj；其他市场如 00700 / AAPL / cff/IF2609
	Name   string          // 名称（可选）
}

// Key 返回带完整市场前缀的唯一键，如 sh600000 / HK00700 / US.AAPL /
// future.cff/IF2609 / hk/index.HSI。用于日志/进度条显示，天然避免跨市场撞名；
// 注意存储文件名不用它（见 FileName，目录已含市场信息，文件名用原始代码）。
func (c Code) Key() string {
	switch c.Market {
	case protocol.MarketAStock, protocol.MarketIndex, protocol.MarketEtfLof, protocol.MarketBlock, protocol.MarketBond, protocol.MarketOther:
		// 标准行情代码自带交易所前缀（sh/sz/bj），直接使用
		return c.Code
	case protocol.MarketHK:
		return "HK" + c.Code
	case protocol.MarketUS:
		return "US." + c.Code
	default:
		return c.Market.String() + "." + c.Code
	}
}
