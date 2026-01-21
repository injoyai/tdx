package tdx

import (
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/lib/xorms"
	"github.com/robfig/cron/v3"
)

type Updater interface {
	Update() error
}

func NewTimer(spec string, retry int, up Updater) error {
	//立即更新
	err := up.Update()
	if err != nil {
		return err
	}
	cr := cron.New(cron.WithSeconds())
	// 需要每天早上9点更新数据,8点多获取不到今天的数据
	_, err = cr.AddFunc(spec, func() {
		for i := 0; i == 0 || i < retry; i++ {
			if err := up.Update(); err != nil {
				logs.Err(err)
				<-time.After(time.Minute * 5)
			} else {
				break
			}
		}
	})
	if err != nil {
		return err
	}
	cr.Start()
	return nil
}

// NewUpdated 更新 hour=[9|15] minute=0
func NewUpdated(db *xorms.Engine, hour, minute int) (*Updated, error) {
	err := db.Sync2(new(UpdateModel))
	return &Updated{db: db, hour: hour, minute: minute}, err
}

type Updated struct {
	db     *xorms.Engine
	hour   int
	minute int
}

func (this *Updated) Update(key string) error {
	_, err := this.db.Where("`Key`=?", key).Update(&UpdateModel{Time: time.Now().Unix()})
	return err
}

func (this *Updated) Updated(key string) (bool, error) {
	update := new(UpdateModel)
	{ //查询或者插入一条数据
		has, err := this.db.Where("`Key`=?", key).Get(update)
		if err != nil {
			return true, err
		} else if !has {
			update.Key = key
			if _, err = this.db.Insert(update); err != nil {
				return true, err
			}
			return false, nil
		}
	}
	{ //判断是否更新过,更新过则不更新
		now := time.Now()
		node := time.Date(now.Year(), now.Month(), now.Day(), this.hour, this.minute, 0, 0, time.Local)
		updateTime := time.Unix(update.Time, 0)
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

/*



 */

type UpdateModel struct {
	Key  string
	Time int64 //更新时间
}

func (*UpdateModel) TableName() string {
	return "update"
}
