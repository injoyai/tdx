package tdx

import (
	"time"

	"xorm.io/xorm"
)

func NewUpdated(key string, db *xorm.Engine) (*Updated, error) {
	err := db.Sync2(new(UpdateModel))
	return &Updated{key: key, db: db}, err
}

type Updated struct {
	key string
	db  *xorm.Engine
}

func (this *Updated) Update() error {
	_, err := this.db.Where("`Key`=?", this.key).Update(&UpdateModel{Time: time.Now().Unix()})
	return err
}

func (this *Updated) Updated() (bool, error) {
	update := new(UpdateModel)
	{ //查询或者插入一条数据
		has, err := this.db.Where("`Key`=?", this.key).Get(update)
		if err != nil {
			return true, err
		} else if !has {
			update.Key = this.key
			if _, err = this.db.Insert(update); err != nil {
				return true, err
			}
			return false, nil
		}
	}
	{ //判断是否更新过,更新过则不更新
		now := time.Now()
		node := time.Date(now.Year(), now.Month(), now.Day(), 9, 0, 0, 0, time.Local)
		updateTime := time.Unix(update.Time, 0)
		if now.Sub(node) > 0 {
			//当前时间在9点之后,且更新时间在9点之前,需要更新
			if updateTime.Sub(node) < 0 {
				return false, nil
			}
		} else {
			//当前时间在9点之前,且更新时间在上个节点之前
			if updateTime.Sub(node.Add(time.Hour*24)) < 0 {
				return false, nil
			}
		}
	}
	return true, nil
}

type UpdateModel struct {
	Key  string
	Time int64 //更新时间
}

func (*UpdateModel) TableName() string {
	return "update"
}
