package sqlite

import (
	"errors"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
	"xorm.io/xorm"
)

// openDB 打开（必要时创建）指定 sqlite 文件并建表。
func openDB(filename string, table any) (*xorm.Engine, error) {
	db, err := Open(filename)
	if err != nil {
		return nil, err
	}
	if err := db.Sync2(record(table)); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// LastTime 查询表内最后一条记录的源时间；空表返回空字符串。
func LastTime(db *xorm.Engine, table any) (string, error) {
	has, err := db.Desc("Time").Get(table)
	if err != nil {
		return "", err
	}
	if !has {
		return "", nil
	}
	switch v := table.(type) {
	case *model.KlineDay:
		return v.Time, nil
	case *model.KlineMinute:
		return v.Time, nil
	}
	return "", errors.New("pull: LastTime 不支持的表格类型")
}

// upsertDay 增量写日线：删除 Time>=from 后批量插入新数据（事务内，幂等）。
// from 通常为 last.Time（覆盖最新可能变动的一根）。
// 空数据跳过写入：既避免创建空库文件，也防止"只删不插"侵蚀已有数据。
func upsertDay(db *xorm.Engine, from string, ks []*model.KlineDay) error {
	if len(ks) == 0 {
		return nil
	}
	return transaction(db, func(session *xorm.Session) error {
		if _, err := session.Where("Time >= ?", from).Delete(new(model.KlineDay)); err != nil {
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
func upsertMin(db *xorm.Engine, from string, ks []*model.KlineMinute) error {
	if len(ks) == 0 {
		return nil
	}
	return transaction(db, func(session *xorm.Session) error {
		return writeMin(session, from, ks)
	})
}

func writeMin(session *xorm.Session, from string, ks []*model.KlineMinute) error {
	if _, err := session.Where("Time >= ?", from).Delete(new(model.KlineMinute)); err != nil {
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
	dayCols = 7 // Time, OHLC, Volume, Amount
	minCols = 8 // Time, OHLC, Volume, Amount, Source
)

// queryDay 按时间范围查询日线（升序）；start/end 为空字符串时表示不限制该端。
func queryDay(db *xorm.Engine, start, end string) ([]*model.KlineDay, error) {
	if err := model.ValidateTimeRange(start, end, "2006-01-02"); err != nil {
		return nil, err
	}
	ls := []*model.KlineDay{}
	err := rangeQuery(db, start, end).Asc("Time").Find(&ls)
	return ls, err
}

// queryMin 按时间范围查询分钟线（升序）；start/end 为空字符串时表示不限制该端。
func queryMin(db *xorm.Engine, start, end string) ([]*model.KlineMinute, error) {
	if err := model.ValidateTimeRange(start, end, protocol.TimeLayout); err != nil {
		return nil, err
	}
	ls := []*model.KlineMinute{}
	err := rangeQuery(db, start, end).Asc("Time").Find(&ls)
	return ls, err
}

// rangeQuery 构造时间范围查询条件；空字符串端点不限制。
func rangeQuery(db *xorm.Engine, start, end string) *xorm.Session {
	q := db.Where("")
	if start != "" {
		q = q.Where("Time >= ?", start)
	}
	if end != "" {
		q = q.Where("Time <= ?", end)
	}
	return q
}

func transaction(db *xorm.Engine, fn func(*xorm.Session) error) error {
	_, err := db.Transaction(func(tx *xorm.Session) (any, error) { return nil, fn(tx) })
	return err
}
