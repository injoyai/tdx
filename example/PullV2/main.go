// PullV2 演示 v2 拉取服务（extend/pull）的多种用法。
// 配置全部通过代码参数传入（本库作为第三方引用，不写配置文件）。
//
// 说明：
//   - 数据落库到输出目录 ./output/pullv2（已 gitignore）。
//   - 需联网连接通达信服务器；日线+1分钟线，成交量统一为"股"。
//   - 运行一次即拉取（每天增量去重，重复运行会自动跳过当日）。
package main

import (
	"context"
	"fmt"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/extend/pull"
	_ "github.com/injoyai/tdx/extend/pull/market" // 注册全部内置市场 Unit
)

func main() {
	// 例1：全市场拉取（股票/指数/ETF/LOF/板块/期货/港股/美股）
	// 数据根目录 ./output/pullv2，起始日期 20260101，日线+分钟线
	_ = pullAll()

	// 例2：只拉指定的沪深股票 + 港股，自定义代码列表，并发 4
	// _ = pullCustom()

	// 例3：只拉日线，指定单个市场
	// _ = pullDayOnly()
}

// pullAll 全市场拉取示例。
func pullAll() error {
	m, err := tdx.NewManage()
	if err != nil {
		return err
	}
	if closer, ok := m.IPool.(interface{ Close() error }); ok {
		defer closer.Close()
	}

	// 扩展行情连接池（期货/港股/美股必需）
	ex, err := tdx.NewPool(func() (*tdx.Client, error) {
		return tdx.DialExHqDefault()
	}, 4)
	if err != nil {
		return err
	}
	defer ex.Close()

	cfg := &pull.PullConfig{
		Dir:        "./output/pullv2",
		Goroutines: 8,
		StartAt:    "20260101",
		Manage:     m,
		ExPool:     ex,
		Workday:    m.Workday,
	}
	s, err := pull.NewService(cfg)
	if err != nil {
		return err
	}
	logs.Debug("开始全市场拉取，日线+1分钟线，成交量单位=股")
	err = s.Update(context.Background())
	logs.Debug("全市场拉取完成, err=", err)
	return err
}

// pullCustom 自定义范围拉取示例。
func pullCustom() error {
	m, err := tdx.NewManage()
	if err != nil {
		return err
	}
	if closer, ok := m.IPool.(interface{ Close() error }); ok {
		defer closer.Close()
	}

	ex, err := tdx.NewPool(func() (*tdx.Client, error) {
		return tdx.DialExHqDefault()
	}, 2)
	if err != nil {
		return err
	}
	defer ex.Close()

	cfg := &pull.PullConfig{
		Dir:        "./output/pullv2/custom",
		Goroutines: 4,
		// 只更新两个市场
		Units: []pull.Unit{
			mustUnit("a_stock"),
			mustUnit("hk"),
		},
		// 自定义代码列表（覆盖默认的自动发现）
		Codes: map[string][]string{
			"a_stock": {"sh600000", "sz000001"},
			"hk":      {"HK00700", "HK09988"},
		},
		StartAt: "20260601",
		Manage:  m,
		ExPool:  ex,
	}
	s, err := pull.NewService(cfg)
	if err != nil {
		return err
	}
	return s.Update(context.Background())
}

// pullDayOnly 只拉日线示例。
func pullDayOnly() error {
	m, err := tdx.NewManage()
	if err != nil {
		return err
	}
	if closer, ok := m.IPool.(interface{ Close() error }); ok {
		defer closer.Close()
	}

	cfg := &pull.PullConfig{
		Dir:     "./output/pullv2/dayonly",
		Day:     true, // 只拉日线
		Minute:  false,
		StartAt: "20260101",
		// 只更新指数市场
		Units:  []pull.Unit{mustUnit("index")},
		Manage: m,
	}
	s, err := pull.NewService(cfg)
	if err != nil {
		return err
	}
	return s.Update(context.Background())
}

// mustUnit 按名称取已注册的市场 Unit。
func mustUnit(name string) pull.Unit {
	u, ok := pull.Get(name)
	if !ok {
		panic(fmt.Sprintf("未注册的市场: %s", name))
	}
	return u
}
