// Package update 提供可停止的定时更新工具。
package update

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
	"github.com/robfig/cron/v3"
)

// StartTimer 立即更新后启动定时任务；stop 可重复或并发调用，并等待在途任务退出。
// 同一任务不重叠执行，停止时取消重试等待；更新函数自身应有有限超时。
func StartTimer(spec string, retry int, update func() error) (stop func(), err error) {
	if update == nil {
		return nil, errors.New("update: missing update function")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cr := cron.New(cron.WithSeconds(), cron.WithLocation(protocol.Location()), cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))
	// 需要每天早上9点更新数据,8点多获取不到今天的数据
	_, err = cr.AddFunc(spec, func() {
		for i := 0; i < max(1, retry); i++ {
			if ctx.Err() != nil {
				return
			}
			if err := update(); err != nil {
				slog.Error("定时更新失败", "spec", spec, "attempt", i+1, "err", err)
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
	if err := update(); err != nil {
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
