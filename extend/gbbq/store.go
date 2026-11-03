package gbbq

import (
	"errors"

	"github.com/injoyai/tdx/v2/lib/update"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
	"xorm.io/xorm"
)

// Store 保存完整股本/除权除息快照及更新标记。Save 须原子替换，失败保留旧数据。
// 存储由调用方管理生命周期。
type Store interface {
	update.Store
	Load() ([]*protocol.Gbbq, error)
	Save(rows []*protocol.Gbbq) error
}

type SQLStore struct {
	*update.SQLStore
	db *xorms.Engine
}

// NewSQLStore 使用已有表结构，不接管数据库生命周期。
func NewSQLStore(db *xorms.Engine) (*SQLStore, error) {
	if db == nil || db.Engine == nil {
		return nil, errors.New("gbbq: 数据库引擎为空")
	}
	if err := db.Sync2(new(protocol.Gbbq)); err != nil {
		return nil, err
	}
	marks, err := update.NewSQLStore(db)
	if err != nil {
		return nil, err
	}
	return &SQLStore{SQLStore: marks, db: db}, nil
}

func (s *SQLStore) Load() ([]*protocol.Gbbq, error) {
	var rows []*protocol.Gbbq
	err := s.db.Asc("Time").Find(&rows)
	return rows, err
}

func (s *SQLStore) Save(rows []*protocol.Gbbq) error {
	for _, row := range rows {
		if row == nil {
			return errors.New("gbbq: nil 股本记录")
		}
	}
	err := s.db.SessionFunc(func(session *xorm.Session) error {
		if _, err := session.Where("1=1").Delete(new(protocol.Gbbq)); err != nil {
			return err
		}
		for _, v := range rows {
			if _, err := session.Insert(v); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
