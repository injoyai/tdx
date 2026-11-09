package model

// KlineDay 日线；价格/金额使用证券原币种或报价单位，直接保存统一 Client 的成交量。
type KlineDay struct {
	Time   string   // 源日期 YYYY-MM-DD，不附加收盘时刻
	Open   float64  // 开盘价，原币种/报价单位，不乘1000
	High   float64  // 最高价，原币种/报价单位，不乘1000
	Low    float64  // 最低价，原币种/报价单位，不乘1000
	Close  float64  // 收盘价/该周期最新价，原币种/报价单位，不乘1000
	Volume int64    // 统一 Client 的最小单位：股票为股，期货为张
	Amount *float64 // 原币种成交额；nil 表示未知，真实零使用非 nil 的 0
}

// KlineMinute 1分钟线；量价单位及未确认金额语义与 KlineDay 相同。
type KlineMinute struct {
	Time   string   // 源日期时分 YYYY-MM-DD HH:MM
	Open   float64  // 开盘价，原币种/报价单位，不乘1000
	High   float64  // 最高价，原币种/报价单位，不乘1000
	Low    float64  // 最低价，原币种/报价单位，不乘1000
	Close  float64  // 收盘价/该周期最新价，原币种/报价单位，不乘1000
	Volume int64    // 统一 Client 的最小单位：股票为股，期货为张
	Amount *float64 // 原币种成交额；nil 表示未知，真实零使用非 nil 的 0
	Source string   // 空=原生分钟线，trade=分笔合成
}

// MinuteSourceTrade 标识分笔合成的近似分钟线。
const MinuteSourceTrade = "trade"
