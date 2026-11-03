// Package update 提供持久化更新标记和可停止的定时更新工具。
package update

import (
	"errors"
	"time"

	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
	"xorm.io/xorm"
)

// Store 持久化更新标记。缺少记录时 LoadUpdate 返回 has=false；方法须支持并发调用。
// 标记使用 Unix 秒，独立于行情数据的源时间字符串。
type Store interface {
	LoadUpdate(key string) (unix int64, has bool, err error)
	SaveUpdate(key string, unix int64) error
}

// New 创建使用 SQL 存储、以固定东八区每日 hour:minute 为边界的完成标记。
// 数据库仍由调用方关闭。
func New(db *xorms.Engine, hour, minute int) (*Tracker, error) {
	store, err := NewSQLStore(db)
	if err != nil {
		return nil, err
	}
	return NewWithStore(store, hour, minute)
}

// NewWithStore 创建使用自定义存储的完成标记，不打开数据库、不接管存储生命周期。
func NewWithStore(store Store, hour, minute int) (*Tracker, error) {
	if store == nil {
		return nil, errors.New("update: Store 不能为 nil")
	}
	return &Tracker{store: store, hour: hour, minute: minute}, nil
}

type Tracker struct {
	store  Store
	hour   int
	minute int
}

func (u *Tracker) Mark(key string) error {
	return u.store.SaveUpdate(key, time.Now().Unix())
}

// IsUpdatedSince 判断指定边界（含）之后是否已成功更新。
func (u *Tracker) IsUpdatedSince(key string, since time.Time) (bool, error) {
	unix, has, err := u.store.LoadUpdate(key)
	return has && unix >= since.Unix(), err
}

func (u *Tracker) IsUpdated(key string) (bool, error) {
	now := time.Now().In(protocol.Location())
	node := time.Date(now.Year(), now.Month(), now.Day(), u.hour, u.minute, 0, 0, protocol.Location())
	if !now.After(node) {
		node = node.Add(-24 * time.Hour)
	}
	return u.IsUpdatedSince(key, node)
}

// SQLStore 使用现有 update 表保存标记，不接管数据库生命周期。
type SQLStore struct{ db *xorms.Engine }

func NewSQLStore(db *xorms.Engine) (*SQLStore, error) {
	if db == nil || db.Engine == nil {
		return nil, errors.New("update: 数据库引擎为空")
	}
	if err := db.Sync2(new(Record)); err != nil {
		return nil, err
	}
	return &SQLStore{db: db}, nil
}

func (s *SQLStore) LoadUpdate(key string) (int64, bool, error) {
	var record Record
	has, err := s.db.Where("`Key`=?", key).Get(&record)
	return record.Time, has, err
}

func (s *SQLStore) SaveUpdate(key string, unix int64) error {
	return s.db.SessionFunc(func(session *xorm.Session) error {
		mark := &Record{Key: key, Time: unix}
		n, err := session.Where("`Key`=?", key).Cols("Time").Update(mark)
		if err != nil || n > 0 {
			return err
		}
		_, err = session.Insert(mark)
		return err
	})
}

type Record struct {
	Key  string
	Time int64 //更新时间
}

func (*Record) TableName() string {
	return "update"
}
