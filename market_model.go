package tdx

import "github.com/injoyai/tdx/v2/protocol"

// 统一结果的价格使用品种原币种/报价单位，浮点值保留底层解码精度。
// VolumeUnit 只描述对应接口；unknown 表示保留解码数值且不做推测换算。
const VolumeUnknown protocol.VolumeUnit = "unknown"

// KlineResp 是统一 K 线结果，Category 为实际协议周期，List 按时间升序排列。
type KlineResp struct {
	Instrument protocol.Instrument
	Category   uint8 // 实际发送的协议周期
	VolumeUnit protocol.VolumeUnit
	Count      int
	List       []Kline
}

// Kline 是统一 K 线记录，Standard/Extended/RangeExtended 按查询来源保留原始字段。
type Kline struct {
	Time          string // 源数据的日期/墙上时间 YYYY-MM-DD HH:MM；不推断时区或夜盘交易日
	Open          float64
	High          float64
	Low           float64
	Close         float64
	Volume        int64
	Amount        *float64 // nil 表示该市场的金额语义未确认
	Standard      *protocol.Kline
	Extended      *protocol.ExKline
	RangeExtended *protocol.ExRangeKline // 区间协议的持仓和结算价等原始字段
}

// Quote 是统一五档报价，价格使用品种原币种/报价单位，成交量以 VolumeUnit 为准。
type Quote struct {
	Instrument protocol.Instrument
	PreClose   float64
	Open       float64
	High       float64
	Low        float64
	Price      float64
	Volume     int64
	VolumeUnit protocol.VolumeUnit
	Bid        [5]float64
	BidVolume  [5]int64
	Ask        [5]float64
	AskVolume  [5]int64
	Standard   *protocol.Quote
	Extended   *protocol.ExQuote
}

// MinuteResp 是统一分时结果，Date 保留历史查询日期，不推断夜盘日期。
type MinuteResp struct {
	Instrument protocol.Instrument
	Date       string // 请求日期 YYYYMMDD，不按行号推断夜盘日期
	VolumeUnit protocol.VolumeUnit
	Count      int
	List       []Minute
}

// Minute 是统一分时记录，Standard 或 Extended 保留对应协议的原始数据。
type Minute struct {
	Time     string // HH:MM
	Price    float64
	Volume   int64
	Standard *protocol.PriceNumber
	Extended *protocol.ExMinuteTick
}

// TradeResp 是统一分笔结果；跨日查询的 Date 为空，交易日期保留在每条记录中。
type TradeResp struct {
	Instrument protocol.Instrument
	Date       string
	VolumeUnit protocol.VolumeUnit
	Count      int
	List       []Trade
}

// Trade 是统一分笔记录；价格尺度未确认时 Price 为 nil，原始值仍可从 Extended 读取。
type Trade struct {
	Date      string   // 历史查询日期 YYYYMMDD；跨日结果逐条保留，不推断当日扩展日期
	Time      string   // HH:MM:SS，源数据无秒时为 00
	Price     *float64 // 扩展分笔原始整数价格尺度未确认，保留 Extended.Price，公共价格为 nil
	Volume    int64
	Direction int // 1 买，-1 卖，0 中性
	Standard  *protocol.Trade
	Extended  *protocol.ExTradeTick
}

// Code 是指定市场的品种信息，价格精度等协议特有字段保留在 Standard 或 Extended 中。
type Code struct {
	Instrument protocol.Instrument
	Name       string
	Standard   *protocol.Code
	Extended   *protocol.ExInstrument
}

// CodeResp 是统一代码列表结果，Count 为本次返回数量，不是市场品种总数。
type CodeResp struct {
	Count int
	List  []Code
}

// Exchange 保留服务器市场编号；发现市场不表示统一行情路由已接入。
type Exchange struct {
	Market    uint16
	Name      string
	ShortName string
	Protocol  protocol.MarketProtocol
	Extended  *protocol.ExMarket
}
