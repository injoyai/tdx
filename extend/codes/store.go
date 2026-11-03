package codes

import (
	"errors"

	"github.com/injoyai/tdx/v2/lib/update"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"xorm.io/xorm"
)

// Store 保存完整代码快照和更新标记。Save 必须原子替换快照，保留既有证券身份，
// 并返回提交后的记录（含主键）；失败时不得修改已提交数据。调用方管理生命周期。
type Store interface {
	update.Store
	Load() ([]*CodeModel, error)
	Save(rows []*CodeModel) ([]*CodeModel, error)
}

// SQLStore 使用现有代码表结构；数据库由调用方关闭。
type SQLStore struct {
	*update.SQLStore
	db *xorms.Engine
}

func NewSQLStore(db *xorms.Engine) (*SQLStore, error) {
	if db == nil || db.Engine == nil {
		return nil, errors.New("codes: 数据库引擎为空")
	}
	if err := db.Sync2(new(CodeModel)); err != nil {
		return nil, err
	}
	marks, err := update.NewSQLStore(db)
	if err != nil {
		return nil, err
	}
	return &SQLStore{SQLStore: marks, db: db}, nil
}

func (s *SQLStore) Load() ([]*CodeModel, error) {
	var rows []*CodeModel
	err := s.db.Find(&rows)
	return rows, err
}

func (s *SQLStore) Save(rows []*CodeModel) ([]*CodeModel, error) {
	list, err := s.Load()
	if err != nil {
		return nil, err
	}
	mCode := make(map[string]*CodeModel, len(list))
	for _, v := range list {
		mCode[v.FullCode()] = v
	}
	var inserts, updates []*CodeModel
	for _, code := range rows {
		if code == nil {
			return nil, errors.New("codes: nil 代码记录")
		}
		if val, ok := mCode[code.FullCode()]; ok {
			if val.Name != code.Name || val.Multiple != code.Multiple || val.Decimal != code.Decimal || val.LastPrice != code.LastPrice {
				updates = append(updates, code)
			}
			delete(mCode, code.FullCode())
		} else {
			inserts = append(inserts, code)
		}
	}
	// 增量操作统一放在事务中，保留未变化记录和已有主键。
	err = s.db.SessionFunc(func(session *xorm.Session) error {
		for _, v := range mCode {
			if _, err := session.Where("Exchange=? and Code=?", v.Exchange, v.Code).Delete(v); err != nil {
				return err
			}
		}
		batchSize := 100 // SQLite 每批不超过 999 个参数。
		for start := 0; start < len(inserts); start += batchSize {
			end := min(start+batchSize, len(inserts))
			if _, err := session.Insert(inserts[start:end]); err != nil {
				return err
			}
		}
		for _, v := range updates {
			if _, err := session.Where("Exchange=? and Code=?", v.Exchange, v.Code).
				Cols("Name", "Multiple", "Decimal", "LastPrice").Update(v); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	list = nil
	if err := s.db.Find(&list); err != nil {
		return nil, err
	}

	return list, nil
}
