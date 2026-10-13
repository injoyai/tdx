package common

import "github.com/injoyai/tdx/v2"

// Run 在示例查询结束或 panic 时关闭客户端。
func Run(f func(c tdx.Client)) {
	c := tdx.NewClient(tdx.WithRedial(false))
	defer c.Close()
	f(c)
}
