package tdx

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/logs"
)

// NewTCPDial 连接指定的完整 TCP 地址，不补端口。
func NewTCPDial(addr string) ios.DialFunc {
	return func(ctx context.Context) (ios.ReadWriteCloser, string, error) {
		c, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", addr)
		return c, addr, err
	}
}

// NewHostDial 每次连接下一个完整地址；空列表返回错误。
func NewHostDial(hosts []string) ios.DialFunc {
	index := 0
	var mu sync.Mutex

	return func(ctx context.Context) (ios.ReadWriteCloser, string, error) {
		if len(hosts) == 0 {
			return nil, "", errors.New("服务器列表为空")
		}
		mu.Lock()
		addr := hosts[index]
		index = (index + 1) % len(hosts)
		mu.Unlock()
		c, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", addr)
		return c, addr, err
	}
}

// NewRandomDial 随机连接列表中的完整地址；空列表返回错误。
func NewRandomDial(hosts []string) ios.DialFunc {
	return func(ctx context.Context) (ios.ReadWriteCloser, string, error) {
		if len(hosts) == 0 {
			return nil, "", errors.New("服务器列表为空")
		}
		addr := hosts[rand.Intn(len(hosts))]
		c, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", addr)
		return c, addr, err
	}
}

// NewRangeDial 遍历完整地址，首次连接成功即返回；空列表返回错误。
func NewRangeDial(hosts []string) ios.DialFunc {
	return func(ctx context.Context) (c ios.ReadWriteCloser, _ string, err error) {
		if len(hosts) == 0 {
			return nil, "", errors.New("服务器列表为空")
		}
		for i, addr := range hosts {
			select {
			case <-ctx.Done():
				return nil, "", ctx.Err()
			default:
			}
			c, err = (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", addr)
			if err == nil {
				return c, addr, nil
			}
			if i < len(hosts)-1 {
				//最后一个错误返回出去
				logs.Err(err, "等待2秒后尝试下一个服务地址...")
				select {
				case <-ctx.Done():
					return nil, "", ctx.Err()
				case <-time.After(2 * time.Second):
				}
			}
		}
		return
	}
}
