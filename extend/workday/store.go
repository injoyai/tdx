package workday

import (
	"errors"

	"github.com/injoyai/tdx/v2/lib/xorms"
	"xorm.io/xorm"
)

// Store 保存历史交易日；Load 按 Unix 升序返回，Save 原子追加新记录，失败保留旧数据。
// 存储由调用方管理生命周期。
type Store interface {
	Load() ([]*WorkdayModel, error)
	Save(rows []*WorkdayModel) error
}

type SQLStore struct{ db *xorms.Engine }

// NewSQLStore 使用现有 workday 表，不接管数据库生命周期。
func NewSQLStore(db *xorms.Engine) (*SQLStore, error) {
	if db == nil || db.Engine == nil {
		return nil, errors.New("workday: 数据库引擎为空")
	}
	if err := db.Sync2(new(WorkdayModel)); err != nil {
		return nil, err
	}
	return &SQLStore{db: db}, nil
}

func (s *SQLStore) Load() ([]*WorkdayModel, error) {
	var rows []*WorkdayModel
	err := s.db.Asc("Unix").Find(&rows)
	return rows, err
}

func (s *SQLStore) Save(rows []*WorkdayModel) error {
	if len(rows) == 0 {
		return nil
	}
	for _, row := range rows {
		if row == nil {
			return errors.New("workday: nil 交易日记录")
		}
	}
	return s.db.SessionFunc(func(session *xorm.Session) error {
		for start := 0; start < len(rows); start += 100 {
			if _, err := session.Insert(rows[start:min(start+100, len(rows))]); err != nil {
				return err
			}
		}
		return nil
	})
}
