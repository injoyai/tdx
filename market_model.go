package tdx

import "github.com/injoyai/tdx/v2/protocol"

// 统一结果的价格使用品种原币种/报价单位，浮点值保留底层解码精度。
// VolumeUnit 只描述对应接口；unknown 表示保留解码数值且不做推测换算。
const VolumeUnknown protocol.VolumeUnit = "unknown"

type MarketKlineResp struct {
	Instrument protocol.Instrument
	Category   uint8 // 实际发送的协议周期
	VolumeUnit protocol.VolumeUnit
	Count      int
	List       []MarketKline
}

type MarketKline struct {
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

type MarketQuote struct {
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

type MarketMinuteResp struct {
	Instrument protocol.Instrument
	Date       string // 请求日期 YYYYMMDD，不按行号推断夜盘日期
	VolumeUnit protocol.VolumeUnit
	Count      int
	List       []MarketMinute
}

type MarketMinute struct {
	Time     string // HH:MM
	Price    float64
	Volume   int64
	Standard *protocol.PriceNumber
	Extended *protocol.ExMinuteTick
}

type MarketTradeResp struct {
	Instrument protocol.Instrument
	Date       string
	VolumeUnit protocol.VolumeUnit
	Count      int
	List       []MarketTrade
}

type MarketTrade struct {
	Date      string   // 历史查询日期 YYYYMMDD；跨日结果逐条保留，不推断当日扩展日期
	Time      string   // HH:MM:SS，源数据无秒时为 00
	Price     *float64 // 扩展分笔原始整数价格尺度未确认，保留 Extended.Price，公共价格为 nil
	Volume    int64
	Direction int // 1 买，-1 卖，0 中性
	Standard  *protocol.Trade
	Extended  *protocol.ExTradeTick
}

type MarketCode struct {
	Instrument protocol.Instrument
	Name       string
	Standard   *protocol.Code
	Extended   *protocol.ExInstrument
}

type MarketCodeResp struct {
	Count int
	List  []MarketCode
}

// MarketExchange 保留服务器市场编号；发现市场不表示统一行情路由已接入。
type MarketExchange struct {
	Market    uint16
	Name      string
	ShortName string
	Protocol  protocol.MarketProtocol
	Extended  *protocol.ExMarket
}
