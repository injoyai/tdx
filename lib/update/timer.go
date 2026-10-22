package update

import (
	"context"
	"sync"
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2/protocol"
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
	cr := cron.New(cron.WithSeconds(), cron.WithLocation(protocol.Location()), cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))
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
