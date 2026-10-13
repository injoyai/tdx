package tdx

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/ios/module/tcp"
	"github.com/injoyai/logs"
)

func NewTCPDial(addr string) ios.DialFunc {
	return tcp.NewDial(withPort(addr, "7709"))
}

func withPort(addr, port string) string {
	if !strings.Contains(addr, ":") {
		return net.JoinHostPort(addr, port)
	}
	return addr
}

func hostAddrs(hosts []string, port string) []string {
	addrs := make([]string, len(hosts))
	for i, host := range hosts {
		addrs[i] = withPort(host, port)
	}
	return addrs
}

func NewHostDial(hosts []string) ios.DialFunc {
	if len(hosts) == 0 {
		hosts = Hosts
	}
	index := 0

	return func(ctx context.Context) (ios.ReadWriteCloser, string, error) {
		if len(hosts) == 0 {
			return nil, "", errors.New("服务器列表为空")
		}
		defer func() { index++ }()
		if index >= len(hosts) {
			index = 0
		}
		addr := withPort(hosts[index], "7709")
		c, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", addr)
		return c, addr, err
	}
}

func NewRandomDial(hosts []string) ios.DialFunc {
	if len(hosts) == 0 {
		hosts = Hosts
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return func(ctx context.Context) (ios.ReadWriteCloser, string, error) {
		if len(hosts) == 0 {
			return nil, "", errors.New("服务器列表为空")
		}
		addr := withPort(hosts[r.Intn(len(hosts))], "7709")
		c, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", addr)
		return c, addr, err
	}
}

func NewRangeDial(hosts []string) ios.DialFunc {
	if len(hosts) == 0 {
		hosts = Hosts
	}
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
			addr = withPort(addr, "7709")
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
