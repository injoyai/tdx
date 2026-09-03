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

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/extend/pull"
	_ "github.com/injoyai/tdx/extend/pull/market" // 注册全部内置市场 Unit
)

func main() {
	m, err := tdx.NewManage()
	logs.PrintErr(err)

	// 扩展行情连接池（期货/港股/美股必需）
	ex, err := tdx.NewPool(func() (*tdx.Client, error) {
		return tdx.DialExHqDefault()
	}, 4)
	logs.PrintErr(err)

	defer ex.Close()

	s, err := pull.NewService(&pull.Config{
		Dir:        "./output/pullv2",
		Goroutines: 8,
		StartAt:    "20260101",
		Manage:     m,
		ExPool:     ex,
		Workday:    m.Workday,
		Codes: []string{
			"sz000001",
			"sh000001",
			"HK00700",
			"US.AAPL",
			"HK.HSI",
		},
	})
	logs.PrintErr(err)

	logs.Debug("开始拉取")
	err = s.Update(context.Background(), true)
	logs.Debug("拉取完成, err=", err)
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

	cfg := &pull.Config{
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

	cfg := &pull.Config{
		Dir:        "./output/pullv2/custom",
		Goroutines: 4,
		// 自定义代码列表（覆盖默认的自动发现）：
		// 直接写代码即可，市场自动路由（sz000001→沪深股票、510300→ETF、00700→港股…见 pull.ParseCode）
		Codes: []string{
			"sz000001", // 平安银行（沪深股票）
			"sh600000", // 浦发银行（沪深股票）
			"510300",   // 沪深300ETF（自动补 sh 前缀）
			"399001",   // 深证成指（指数）
			"00700",    // 腾讯（港股，5 位数字自动识别）
			"AAPL",     // 苹果（美股，纯字母自动识别）
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

	cfg := &pull.Config{
		Dir:     "./output/pullv2/dayonly",
		Day:     true, // 只拉日线
		Minute:  false,
		StartAt: "20260101",
		// 只拉几只指数（白名单模式：只拉列出的代码，不拉其他市场）
		Codes: []string{
			"sh000001", // 上证指数
			"sz399001", // 深证成指
			"sz399006", // 创业板指
		},
		Manage: m,
	}
	s, err := pull.NewService(cfg)
	if err != nil {
		return err
	}
	return s.Update(context.Background())
}
