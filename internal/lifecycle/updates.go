package lifecycle

import (
	"net"
	"sync"
)

// Updates 串行化元数据刷新；关闭先停止任务，再释放本对象创建的资源。
type Updates struct {
	mu     sync.Mutex
	closed bool
	once   sync.Once
	err    error
}

func (u *Updates) Run(fn func() error) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed {
		return net.ErrClosed
	}
	return fn()
}

func (u *Updates) Close(stop func(), cleanup func() error) error {
	u.once.Do(func() {
		u.mu.Lock()
		u.closed = true
		u.mu.Unlock()
		if stop != nil {
			stop()
		}
		if cleanup != nil {
			u.err = cleanup()
		}
	})
	return u.err
}
