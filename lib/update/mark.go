// Package update 提供持久化更新标记和可停止的定时更新工具。
package update

import (
	"time"

	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
	"xorm.io/xorm"
)

// New 创建以固定东八区每日 hour:minute 为更新边界的完成标记。
func New(db *xorms.Engine, hour, minute int) (*Tracker, error) {
	err := db.Sync2(new(Record))
	return &Tracker{db: db, hour: hour, minute: minute}, err
}

type Tracker struct {
	db     *xorms.Engine
	hour   int
	minute int
}

func (u *Tracker) Mark(key string) error {
	return u.db.SessionFunc(func(session *xorm.Session) error {
		mark := &Record{Key: key, Time: time.Now().Unix()}
		n, err := session.Where("`Key`=?", key).Cols("Time").Update(mark)
		if err != nil || n > 0 {
			return err
		}
		_, err = session.Insert(mark)
		return err
	})
}

func (u *Tracker) IsUpdated(key string) (bool, error) {
	mark := new(Record)
	{ // 查询不创建占位记录，标记由 Mark 原子保存。
		has, err := u.db.Where("`Key`=?", key).Get(mark)
		if err != nil {
			return true, err
		} else if !has {
			return false, nil
		}
	}
	{ //判断是否更新过,更新过则不更新
		now := time.Now().In(protocol.Location())
		node := time.Date(now.Year(), now.Month(), now.Day(), u.hour, u.minute, 0, 0, protocol.Location())
		updateTime := time.Unix(mark.Time, 0)
		if now.Sub(node) > 0 {
			//当前时间在9点之后,且更新时间在9点之前,需要更新
			if updateTime.Sub(node) < 0 {
				return false, nil
			}
		} else {
			//当前时间在9点之前,且更新时间在上个节点之前
			if updateTime.Sub(node.Add(-time.Hour*24)) < 0 {
				return false, nil
			}
		}
	}
	return true, nil
}

type Record struct {
	Key  string
	Time int64 //更新时间
}

func (*Record) TableName() string {
	return "update"
}
