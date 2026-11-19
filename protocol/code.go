package protocol

import "github.com/injoyai/tdx/v2/protocol/market"

// DecodeCode 解析证券代码,返回(交易所, 去掉前缀的代码主体, 错误)。
//
// 已接入行情的品种复用 ParseInstrument 的解析与规范化规则，支持以下形式:
//  1. 带交易所前缀,如 "sz000001"/"sh600000"/"hk00700"/"usAAPL"/"cffIF2609"/"上海600000",
//     前缀支持小写缩写(见 Exchange.String())、大写("SZ000001")或中文名("上海600000")。
//  2. 带点后缀,如 "000001.SZ"/"600000.SH"/"00700.HK"/"AAPL.US"/"IF2609.CFF",
//     后缀为交易所缩写(大小写均可);美股点代码如 "BRK.B" 因后缀 B 不是交易所而按美股代码解析。
//  3. 裸 6 位数字代码,自动识别并补前缀(A股/基金/指数/可转债/板块指数),如 "000001"→sz000001。
//  4. 裸非 6 位代码,根据代码形态推断市场:
//     - 5 位港股数字代码需要品种快照确认协议编号，调用 ResolveInstrument；已知编号可写 00700.31
//     - 纯字母代码(如 AAPL/BRK.B)→ 美国(ExchangeUS)
//     - 字母+数字的合约代码(如 IF2609)→ 无法确定交易所,提示使用显式前缀
//
// 港美股主体转大写，期货保留大小写，A股仍为6位数字；原始协议的其他交易所保留旧格式校验。
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
