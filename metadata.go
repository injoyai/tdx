package tdx

import (
	"net"
	"sync"
)

// updates 串行化元数据刷新；关闭先停止任务，再释放本对象创建的资源。
type updates struct {
	mu      sync.Mutex
	closed  bool
	stop    func()
	cleanup func() error
	once    sync.Once
	err     error
}

func (u *updates) run(fn func() error) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed {
		return net.ErrClosed
	}
	return fn()
}

func (u *updates) Close() error {
	u.once.Do(func() {
		u.mu.Lock()
		u.closed = true
		u.mu.Unlock()
		if u.stop != nil {
			u.stop()
		}
		if u.cleanup != nil {
			u.err = u.cleanup()
		}
	})
	return u.err
}
