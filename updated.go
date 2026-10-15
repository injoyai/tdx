package tdx

import (
	"context"
	"sync"
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/robfig/cron/v3"
)

type Updater interface {
	Update() error
}

// NewTimer 启动进程级定时更新。需要释放任务时使用 StartTimer。
func NewTimer(spec string, retry int, up Updater) error {
	_, err := StartTimer(spec, retry, up)
	return err
}

// StartTimer 立即更新后启动定时任务；stop 可重复或并发调用，并等待在途任务退出。
// 同一任务不重叠执行，停止时取消重试等待；up.Update 自身应有有限超时。
func StartTimer(spec string, retry int, up Updater) (stop func(), err error) {
	ctx, cancel := context.WithCancel(context.Background())
	cr := cron.New(cron.WithSeconds(), cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))
	// 需要每天早上9点更新数据,8点多获取不到今天的数据
	_, err = cr.AddFunc(spec, func() {
		for i := 0; i < max(1, retry); i++ {
			if ctx.Err() != nil {
				return
			}
			if err := up.Update(); err != nil {
				logs.Err(err)
				if i+1 < max(1, retry) {
					timer := time.NewTimer(5 * time.Minute)
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case <-timer.C:
					}
				}
			} else {
				break
			}
		}
	})
	if err != nil {
		cancel()
		return nil, err
	}
	if err := up.Update(); err != nil {
		cancel()
		return nil, err
	}
	cr.Start()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-cr.Stop().Done()
		})
	}, nil
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
