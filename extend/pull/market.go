package pull

import "github.com/injoyai/tdx/v2/protocol"

// 别名保持 pull 的公开 API 与存储目录契约，基础配置由协议层共享。
type (
	Market         = protocol.Market
	MarketProtocol = protocol.MarketProtocol
	VolumeUnit     = protocol.VolumeUnit
	KlineInfo      = protocol.KlineInfo
	MarketInfo     = protocol.MarketInfo
)

const (
	MarketAStock        = protocol.MarketAStock
	MarketIndex         = protocol.MarketIndex
	MarketEtfLof        = protocol.MarketEtfLof
	MarketBlock         = protocol.MarketBlock
	MarketFuture        = protocol.MarketFuture
	MarketHK            = protocol.MarketHK
	MarketHKIndex       = protocol.MarketHKIndex
	MarketUS            = protocol.MarketUS
	ProtocolStandard    = protocol.ProtocolStandard
	ProtocolExtended    = protocol.ProtocolExtended
	VolumeDefault       = protocol.VolumeDefault
	VolumeShares        = protocol.VolumeShares
	VolumeHundredShares = protocol.VolumeHundredShares
)
