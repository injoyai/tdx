package pull

import (
	"errors"
	"time"

	"github.com/injoyai/tdx/lib/xorms"
	"xorm.io/xorm"
)

// KlineDay 日线（单位：价格=元，Volume=股，Amount=元）。
// 一代码一个文件 day/{key}.db。
type KlineDay struct {
	Unix       int64 `xorm:"pk"` // 秒级时间戳，日线固定 15:00
	Open       float64
	High       float64
	Low        float64
	Close      float64
	Volume     int64   // 股
	Amount     float64 // 成交额（元）
	Turnover   float64 // 换手率（%），仅股票有，指数/ETF/期货等为 0
	FloatStock float64 // 流通股本（股），股票才有
	TotalStock float64 // 总股本（股），股票才有
}

// KlineMinute 1分钟线（全称，单位：价格=元，Volume=股，Amount=元）。
// 一代码一年一个文件 min/{key}/{year}.db。
type KlineMinute struct {
	Unix   int64 `xorm:"pk"` // 秒级时间戳
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume int64   // 股
	Amount float64 // 成交额（元）
}

// openDB 打开（必要时创建）指定 sqlite 文件并建表。
func openDB(filename string, table any) (*xorms.Engine, error) {
	db, err := xorms.NewSqlite(filename)
	if err != nil {
		return nil, err
	}
	if err := db.Sync2(table); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// LastUnix 查询表内最后一条记录的时间戳；空表返回 0。
func LastUnix(db *xorms.Engine, table any) (int64, error) {
	has, err := db.Desc("Unix").Get(table)
	if err != nil {
		return 0, err
	}
	if !has {
		return 0, nil
	}
	switch v := table.(type) {
	case *KlineDay:
		return v.Unix, nil
	case *KlineMinute:
		return v.Unix, nil
	}
	return 0, errors.New("pull: lastUnix 不支持的表格类型")
}

// upsertDay 增量写日线：删除 Unix>=from 后批量插入新数据（事务内，幂等）。
// from 通常为 last.Unix（覆盖最新可能变动的一根）。
// 空数据跳过写入：既避免创建空库文件，也防止"只删不插"侵蚀已有数据。
func upsertDay(db *xorms.Engine, from int64, ks []*KlineDay) error {
	if len(ks) == 0 {
		return nil
	}
	return db.SessionFunc(func(session *xorm.Session) error {
		if _, err := session.Where("Unix >= ?", from).Delete(new(KlineDay)); err != nil {
			return err
		}
		return insertBatch(session, dayCols, func(start, end int) error {
			_, err := session.Insert(ks[start:end])
			return err
		}, len(ks))
	})
}

// upsertMin 增量写分钟线：删除 Unix>=from 后批量插入新数据（事务内，幂等）。
// 空数据跳过写入，同 upsertDay。
func upsertMin(db *xorms.Engine, from int64, ks []*KlineMinute) error {
	if len(ks) == 0 {
		return nil
	}
	return db.SessionFunc(func(session *xorm.Session) error {
		if _, err := session.Where("Unix >= ?", from).Delete(new(KlineMinute)); err != nil {
			return err
		}
		return insertBatch(session, minCols, func(start, end int) error {
			_, err := session.Insert(ks[start:end])
			return err
		}, len(ks))
	})
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
	dayCols = 10 // Unix, Open, High, Low, Close, Volume, Amount, Turnover, FloatStock, TotalStock
	minCols = 7  // Unix, Open, High, Low, Close, Volume, Amount
)

// queryDay 按时间范围查询日线（升序）；start/end 为零值时表示不限制该端。
func queryDay(db *xorms.Engine, start, end time.Time) ([]*KlineDay, error) {
	ls := []*KlineDay{}
	err := rangeQuery(db, start, end).Asc("Unix").Find(&ls)
	return ls, err
}

// queryMin 按时间范围查询分钟线（升序）；start/end 为零值时表示不限制该端。
func queryMin(db *xorms.Engine, start, end time.Time) ([]*KlineMinute, error) {
	ls := []*KlineMinute{}
	err := rangeQuery(db, start, end).Asc("Unix").Find(&ls)
	return ls, err
}

// rangeQuery 构造时间范围查询条件；零值端点不限制。
func rangeQuery(db *xorms.Engine, start, end time.Time) *xorms.Session {
	q := db.Where("")
	if !start.IsZero() {
		q = q.Where("Unix >= ?", start.Unix())
	}
	if !end.IsZero() {
		q = q.Where("Unix <= ?", end.Unix())
	}
	return q
}
