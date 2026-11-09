package sqlite

import "github.com/injoyai/tdx/v2/extend/model"

type dayRecord struct {
	Time   string   `xorm:"pk"` // 源日期 YYYY-MM-DD，不附加收盘时刻
	Open   float64  // 开盘价，原币种/报价单位，不乘1000
	High   float64  // 最高价，原币种/报价单位，不乘1000
	Low    float64  // 最低价，原币种/报价单位，不乘1000
	Close  float64  // 收盘价/该周期最新价，原币种/报价单位，不乘1000
	Volume int64    // 统一 Client 的最小单位：股票为股，期货为张
	Amount *float64 // 原币种成交额；nil 表示未知，真实零使用非 nil 的 0
}

func (*dayRecord) TableName() string { return "KlineDay" }

type minuteRecord struct {
	Time   string   `xorm:"pk"` // 源日期时分 YYYY-MM-DD HH:MM
	Open   float64  // 开盘价，原币种/报价单位，不乘1000
	High   float64  // 最高价，原币种/报价单位，不乘1000
	Low    float64  // 最低价，原币种/报价单位，不乘1000
	Close  float64  // 收盘价/该周期最新价，原币种/报价单位，不乘1000
	Volume int64    // 统一 Client 的最小单位：股票为股，期货为张
	Amount *float64 // 原币种成交额；nil 表示未知，真实零使用非 nil 的 0
	Source string   `xorm:"default ''"` // 空=原生分钟线，trade=分笔合成
}

func (*minuteRecord) TableName() string { return "KlineMinute" }

type coverageRecord struct {
	ID            int    `xorm:"pk"`
	From          string // 源日期时分 YYYY-MM-DD HH:MM
	Through       string // 同一源时间口径的右开端点；原生扫描为空
	TradeFallback bool
}

func (*coverageRecord) TableName() string { return "minuteCoverage" }

func record(v any) any {
	switch v.(type) {
	case *model.KlineDay:
		return new(dayRecord)
	case *model.KlineMinute:
		return new(minuteRecord)
	}
	return v
}
