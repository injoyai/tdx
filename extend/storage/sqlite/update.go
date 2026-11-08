package sqlite

import (
	"errors"
	"sync"

	"github.com/injoyai/tdx/v2/lib/update"
	"xorm.io/xorm"
)

// UpdateStore 在调用方的 SQLite 引擎上保存更新标记，不接管连接生命周期。
type UpdateStore struct {
	db     *xorm.Engine
	mu     sync.Mutex
	prefix string
}

// Record 保持 SQLite 历史表结构。
type Record struct {
	Key  string
	Time int64
}

func (*Record) TableName() string { return "update" }

// NewUpdate 使用已有 SQLite 引擎并创建缺失的标记表；可指定键前缀隔离同库中的不同组件。
func NewUpdate(db *xorm.Engine, prefix ...string) (*UpdateStore, error) {
	if db == nil {
		return nil, errors.New("update: 数据库引擎为空")
	}
	s := &UpdateStore{db: db}
	if len(prefix) > 0 {
		s.prefix = prefix[0]
	}
	if err := ensureTable(db, new(Record)); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *UpdateStore) LoadUpdate(key string) (int64, bool, error) {
	key = s.prefix + key
	var r Record
	has, err := s.db.Where("`"+s.db.GetColumnMapper().Obj2Table("Key")+"`=?", key).Get(&r)
	return r.Time, has, err
}

func (s *UpdateStore) SaveUpdate(key string, unix int64) error {
	key = s.prefix + key
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Transaction(func(tx *xorm.Session) (any, error) {
		mark := &Record{Key: key, Time: unix}
		n, err := tx.Where("`"+s.db.GetColumnMapper().Obj2Table("Key")+"`=?", key).
			Cols(s.db.GetColumnMapper().Obj2Table("Time")).Update(mark)
		if err != nil || n > 0 {
			return nil, err
		}
		_, err = tx.Insert(mark)
		return nil, err
	})
	return err
}

var _ update.Store = (*UpdateStore)(nil)
