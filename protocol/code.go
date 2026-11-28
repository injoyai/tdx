package protocol

import "github.com/injoyai/tdx/v2/protocol/market"

// DecodeCode 返回协议市场编号和证券代码主体。支持市场.代码、代码.市场及代码.编号。
// 沪深北港也支持紧凑前缀和约定的裸代码。其他市场使用点分隔，如 us.ANT、cff.IF2609。
// 与 ParseInstrument 相比，DecodeCode 也支持尚未接入统一 Client 的原始协议市场。
func DecodeCode(code string) (Exchange, string, error) { return market.DecodeCode(code) }

// IsStock 是否是股票,示例sz000001
func IsStock(code string) bool { return market.IsStock(code) }

// IsConvertibleBond reports whether code belongs to a current convertible-bond code range.
func IsConvertibleBond(code string) bool { return market.IsConvertibleBond(code) }

func IsSZStock(code string) bool { return market.IsSZStock(code) }

func IsSHStock(code string) bool { return market.IsSHStock(code) }

func IsBJStock(code string) bool { return market.IsBJStock(code) }

// IsETF 是否是基金,示例sz159558
func IsETF(code string) bool { return market.IsETF(code) }

// IsIndex 是否是指数,sh000001,sz399001,bj899100
// 板块指数(880xxx 行业/概念, 881xxx 地域)归属上海交易所(ExchangeSH), 也视为指数。
func IsIndex(code string) bool { return market.IsIndex(code) }

// AddPrefix 添加股票/基金/指数/可转债代码前缀,针对股票/基金/指数/可转债生效,例如000001,会增加前缀sz000001(平安银行),而不是sh000001(上证指数)
// 板块指数(880xxx/881xxx)增加前缀 sh,例如 880741 -> sh880741(归属上海交易所)。
// 可转债: 沪市(110/111/113/118)增加前缀 sh, 深市(123/125/126/127/128)增加前缀 sz。
func AddPrefix(code string) string { return market.AddPrefix(code) }
