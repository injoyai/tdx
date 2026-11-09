// pull 演示指定证券或全市场的日线/分钟线拉取与落库查询。
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/pull"
	_ "github.com/injoyai/tdx/v2/extend/pull/market"
	"github.com/injoyai/tdx/v2/extend/storage/sqlite"
	"github.com/injoyai/tdx/v2/protocol"
)

func main() {
	if err := run(); err != nil {
		slog.Error("示例执行失败", "err", err)
		os.Exit(1)
	}
}

func run() error {
	dir := flag.String("dir", "./output/pull", "数据目录，应放在 ./output/ 下")
	since := flag.String("since", time.Now().In(protocol.Location()).AddDate(0, -1, 0).Format("20060102"), "首次拉取的起始日期 YYYYMMDD")
	day := flag.Bool("day", true, "拉取日线")
	minute := flag.Bool("minute", true, "拉取 1 分钟线")
	all := flag.Bool("all", false, "全市场拉取；不能同时指定证券")
	force := flag.Bool("force", false, "允许收盘前拉取，仍保留市场级去重")
	flag.Parse()
	if !*day && !*minute {
		return fmt.Errorf("日线和分钟线至少启用一种")
	}
	if *all && flag.NArg() > 0 {
		return fmt.Errorf("-all 不能同时指定证券")
	}
	_, err := time.Parse("20060102", *since)
	if err != nil {
		return err
	}
	inputs := flag.Args()
	if !*all && len(inputs) == 0 {
		inputs = []string{"sz000001"}
	}
	needsSt, needsEx := *all, *all
	for _, raw := range inputs {
		code, err := pull.ParseCode(raw)
		if err != nil {
			return err
		}
		switch code.Market {
		case protocol.MarketFuture, protocol.MarketHK, protocol.MarketHKIndex, protocol.MarketUS:
			needsEx = true
		default:
			needsSt = true
		}
	}
	cfg := &pull.Config{
		Codes: inputs, Day: *day, Minute: *minute, StartAt: *since,
		Goroutines: 1,
	}
	store, err := sqlite.NewPull(*dir, 32)
	if err != nil {
		return err
	}
	defer store.Close()
	cfg.Store = store
	if needsSt {
		pool, err := tdx.NewPool(func() (tdx.Client, error) {
			return tdx.DialDefault(tdx.WithRedial(false))
		}, 1)
		if err != nil {
			return err
		}
		defer pool.Close()
		cfg.Pool = pool
	}
	var universe []model.Code
	if *all {
		cfg.ListCodes = func(context.Context) ([]model.Code, error) { return universe, nil }
	}
	if needsEx {
		ex, err := tdx.NewPool(func() (tdx.Client, error) {
			return tdx.DialExDefault(tdx.WithRedial(false))
		}, 1)
		if err != nil {
			return err
		}
		defer ex.Close()
		cfg.ExPool = ex
	}
	s, err := pull.New(cfg)
	if err != nil {
		return err
	}
	defer s.Close()
	if *all {
		snapshot, err := s.FetchCodes(context.Background())
		if err != nil {
			return err
		}
		for _, row := range snapshot.Rows {
			code, err := pull.ParseCode(row.FullCode())
			if err != nil {
				return err
			}
			code.Name = row.Name
			universe = append(universe, code)
		}
	}
	if err := s.UpdateContext(context.Background(), *force); err != nil {
		return err
	}
	if *day {
		for _, raw := range inputs {
			code, err := pull.ParseCode(raw)
			if err != nil {
				return err
			}
			ks, err := s.QueryDay(code, "", "")
			if err != nil {
				return err
			}
			fmt.Println(code.Key(), "已落库日线:", len(ks))
			for _, k := range ks {
				fmt.Println(k)
			}
		}
	}
	return nil
}
