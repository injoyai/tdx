package pull

import (
	"errors"

	"github.com/injoyai/tdx/v2/protocol"

	"github.com/injoyai/tdx/v2/lib/xorms"
	"xorm.io/xorm"
)

// KlineDay 日线；价格/金额使用证券原币种或报价单位，成交量按 protocol.Market.Info 换算。
// 一代码一个文件 day/{key}.db。
type KlineDay struct {
	Time          string  `xorm:"pk"` // 源日期 YYYY-MM-DD，不附加收盘时刻
	TradingDate   string  // 已确认交易日 YYYY-MM-DD；未知为空
	Open          float64 // 开盘价，原币种/报价单位，不乘1000
	High          float64 // 最高价，原币种/报价单位，不乘1000
	Low           float64 // 最低价，原币种/报价单位，不乘1000
	Close         float64 // 收盘价/该周期最新价，原币种/报价单位，不乘1000
	Volume        int64   // 已确认市场为股；其他市场按默认倍率存储，单位待确认
	Amount        float64 // 原币种成交额，AmountUnknown=true 时为 0
	AmountUnknown bool    `xorm:"default 0"` // true 时 Amount=0，不能作为真实零成交额
	Turnover      float64 // 换手率（%），仅股票有，指数/ETF/期货等为 0
	FloatStock    float64 // 流通股本（股），股票才有
	TotalStock    float64 // 总股本（股），股票才有
}

// KlineMinute 1分钟线；量价单位及未确认金额语义与 KlineDay 相同。
// 一代码一年一个文件 min/{key}/{year}.db。
type KlineMinute struct {
	Time          string  `xorm:"pk"` // 源日期时分 YYYY-MM-DD HH:MM
	TradingDate   string  // 已确认交易日 YYYY-MM-DD；不从夜盘日期推断
	Open          float64 // 开盘价，原币种/报价单位，不乘1000
	High          float64 // 最高价，原币种/报价单位，不乘1000
	Low           float64 // 最低价，原币种/报价单位，不乘1000
	Close         float64 // 收盘价/该周期最新价，原币种/报价单位，不乘1000
	Volume        int64   // 已确认市场为股；其他市场按默认倍率存储，单位待确认
	Amount        float64 // 原币种成交额，AmountUnknown=true 时为 0
	AmountUnknown bool    `xorm:"default 0"`
	Source        string  `xorm:"default ''"` // 空=原生分钟线，trade=分笔合成
}

// MinuteSourceTrade 标识分笔合成的近似分钟线。
const MinuteSourceTrade = "trade"

// openDB 打开（必要时创建）指定 sqlite 文件并建表。
func openDB(filename string, table any) (*xorms.Engine, error) {
	db, err := xorms.NewSQLite(filename)
	if err != nil {
		return nil, err
	}
	if err := db.Sync2(table); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// LastTime 查询表内最后一条记录的源时间；空表返回空字符串。
func LastTime(db *xorms.Engine, table any) (string, error) {
	has, err := db.Desc("Time").Get(table)
	if err != nil {
		return "", err
	}
	if !has {
		return "", nil
	}
	switch v := table.(type) {
	case *KlineDay:
		return v.Time, nil
	case *KlineMinute:
		return v.Time, nil
	}
	return "", errors.New("pull: LastTime 不支持的表格类型")
}

// upsertDay 增量写日线：删除 Time>=from 后批量插入新数据（事务内，幂等）。
// from 通常为 last.Time（覆盖最新可能变动的一根）。
// 空数据跳过写入：既避免创建空库文件，也防止"只删不插"侵蚀已有数据。
func upsertDay(db *xorms.Engine, from string, ks []*KlineDay) error {
	if len(ks) == 0 {
		return nil
	}
	return db.SessionFunc(func(session *xorm.Session) error {
		if _, err := session.Where("Time >= ?", from).Delete(new(KlineDay)); err != nil {
			return err
		}
		return insertBatch(session, dayCols, func(start, end int) error {
			_, err := session.Insert(ks[start:end])
			return err
		}, len(ks))
	})
}

// upsertMin 增量写分钟线：删除 Time>=from 后批量插入新数据（事务内，幂等）。
// 空数据跳过写入，同 upsertDay。
func upsertMin(db *xorms.Engine, from string, ks []*KlineMinute) error {
	if len(ks) == 0 {
		return nil
	}
	return db.SessionFunc(func(session *xorm.Session) error {
		return writeMin(session, from, ks)
	})
}

func writeMin(session *xorm.Session, from string, ks []*KlineMinute) error {
	if _, err := session.Where("Time >= ?", from).Delete(new(KlineMinute)); err != nil {
		return err
	}
	return insertBatch(session, minCols, func(start, end int) error {
		_, err := session.Insert(ks[start:end])
		return err
	}, len(ks))
}

// insertBatch 分批插入，规避 sqlite 单语句占位符上限（SQLITE_MAX_VARIABLE_NUMBER，常见 999）。
// cols 为每行字段数，fn 逐批调用（批内复用同一 session，整批失败即中止）。
func insertBatch(session *xorm.Session, cols int, fn func(start, end int) error, total int) error {
	// 每批行数 = 990 / 字段数，留出余量（上限常见 999）
	n := 990 / cols
	if n < 1 {
		n = 1
	}
	for start := 0; start < total; start += n {
		end := start + n
		if end > total {
			end = total
		}
		if err := fn(start, end); err != nil {
			return err
		}
	}
	return nil
}

// dayCols / minCols 每行字段数（与 KlineDay / KlineMinute 结构体字段数一致，
// 用于 insertBatch 计算分批大小）。
const (
	dayCols = 12 // Time, TradingDate, OHLC, Volume, Amount, AmountUnknown, Turnover, FloatStock, TotalStock
	minCols = 10 // Time, TradingDate, OHLC, Volume, Amount, AmountUnknown, Source
)

// queryDay 按时间范围查询日线（升序）；start/end 为空字符串时表示不限制该端。
func queryDay(db *xorms.Engine, start, end string) ([]*KlineDay, error) {
	if err := validateRange(start, end, "2006-01-02"); err != nil {
		return nil, err
	}
	ls := []*KlineDay{}
	err := rangeQuery(db, start, end).Asc("Time").Find(&ls)
	return ls, err
}

// queryMin 按时间范围查询分钟线（升序）；start/end 为空字符串时表示不限制该端。
func queryMin(db *xorms.Engine, start, end string) ([]*KlineMinute, error) {
	if err := validateRange(start, end, protocol.TimeLayout); err != nil {
		return nil, err
	}
	ls := []*KlineMinute{}
	err := rangeQuery(db, start, end).Asc("Time").Find(&ls)
	return ls, err
}

// rangeQuery 构造时间范围查询条件；空字符串端点不限制。
func rangeQuery(db *xorms.Engine, start, end string) *xorms.Session {
	q := db.Where("")
	if start != "" {
		q = q.Where("Time >= ?", start)
	}
	if end != "" {
		q = q.Where("Time <= ?", end)
	}
	return q
}
