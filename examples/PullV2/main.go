// PullV2 演示指定证券或全市场的日线/分钟线拉取与落库查询。
package main

import (
	"flag"
	"github.com/injoyai/tdx/v2/protocol"
	"time"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/manage"
	"github.com/injoyai/tdx/v2/extend/pull"
	_ "github.com/injoyai/tdx/v2/extend/pull/market"
)

func main() {
	dir := flag.String("dir", "./output/pullv2", "数据目录，应放在 ./output/ 下")
	since := flag.String("since", time.Now().In(protocol.Location()).AddDate(0, -1, 0).Format("20060102"), "首次拉取的起始日期 YYYYMMDD")
	day := flag.Bool("day", true, "拉取日线")
	minute := flag.Bool("minute", true, "拉取 1 分钟线")
	all := flag.Bool("all", false, "全市场拉取；不能同时指定证券")
	force := flag.Bool("force", false, "忽略当日完成标记")
	flag.Parse()
	if !*day && !*minute {
		logs.Panic("日线和分钟线至少启用一种")
	}
	if *all && flag.NArg() > 0 {
		logs.Panic("-all 不能同时指定证券")
	}
	_, err := time.Parse("20060102", *since)
	logs.PanicErr(err)
	codes := flag.Args()
	if !*all && len(codes) == 0 {
		codes = []string{"sz000001"}
	}
	needsEx := *all
	for _, raw := range codes {
		code, err := pull.ParseCode(raw)
		logs.PanicErr(err)
		switch code.Market {
		case protocol.MarketFuture, protocol.MarketHK, protocol.MarketHKIndex, protocol.MarketUS:
			needsEx = true
		}
	}
	m, err := manage.New()
	logs.PanicErr(err)
	defer m.Close()
	cfg := &pull.Config{
		Dir: *dir, Codes: codes, Day: *day, Minute: *minute, StartAt: *since,
		Goroutines: 1, Manage: m, Workday: m.Workday,
	}
	if needsEx {
		ex, err := tdx.NewPool(func() (tdx.Client, error) {
			return tdx.DialExDefault(tdx.WithRedial(false))
		}, 1)
		logs.PanicErr(err)
		defer ex.Close()
		cfg.ExPool = ex
	}
	s, err := pull.New(cfg)
	logs.PanicErr(err)
	defer s.Close()
	logs.PanicErr(s.Update(*force))
	if *day {
		for _, raw := range codes {
			code, err := pull.ParseCode(raw)
			logs.PanicErr(err)
			ks, err := s.QueryDay(code, "", "")
			logs.PanicErr(err)
			logs.Debug(code.Key(), "已落库日线:", len(ks))
			for _, k := range ks {
				logs.Debug(k)
			}
		}
	}
}
