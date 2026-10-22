package main

import (
	"strings"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/manage"
)

// 管理器复用连接池、代码缓存和交易日历。
func main() {
	m, err := manage.New()
	logs.PanicErr(err)
	defer m.Close()
	logs.Debug("今天是否为交易日:", m.Workday.TodayIs())
	logs.Debug("股票数量:", len(m.Codes.GetStocks()))
	for _, code := range m.Codes.GetIndexCodes() {
		if strings.HasPrefix(code, "sz") {
			logs.Debug("深圳指数:", code)
		}
	}
	sh, sz := 0, 0
	for _, code := range m.Codes.GetETFCodes() {
		switch {
		case strings.HasPrefix(code, "sh"):
			sh++
		case strings.HasPrefix(code, "sz"):
			sz++
		}
	}
	logs.Debug("ETF 数量，上海:", sh, "深圳:", sz)
	err = m.Do(func(c tdx.Client) error {
		resp, err := c.GetKlineDay("sh000001", 0, 20)
		if err != nil {
			return err
		}
		for _, k := range resp.List {
			logs.Debug(k)
		}
		return nil
	})
	logs.PanicErr(err)
}
