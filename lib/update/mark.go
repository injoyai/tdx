// Package update 提供持久化更新标记和可停止的定时更新工具。
package update

import (
	"errors"
	"reflect"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

// Store 持久化更新标记。缺少记录时 LoadUpdate 返回 has=false；方法须支持并发调用。
// 标记使用 Unix 秒，独立于行情数据的源时间字符串。
type Store interface {
	LoadUpdate(key string) (unix int64, has bool, err error)
	SaveUpdate(key string, unix int64) error
}

// NewWithStore 创建使用自定义存储的完成标记，不打开数据库、不接管存储生命周期。
func NewWithStore(store Store, hour, minute int) (*Tracker, error) {
	if store == nil || (reflect.ValueOf(store).Kind() == reflect.Pointer && reflect.ValueOf(store).IsNil()) {
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
