package pull

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/injoyai/conv"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/export"
	"github.com/injoyai/tdx/v2/protocol"
)

// NewTrade 创建历史分笔下载器，分笔及合成分钟线 CSV 写入 dir。
func NewTrade(dir string) *Trade {
	return &Trade{
		Dir: dir,
	}
}

// Trade 按交易日下载历史分笔并导出 CSV；注入组件由调用方维护和关闭。
type Trade struct {
	Dir     string
	Pool    tdx.ClientPool
	Workday interface {
		RangeYear(int, func(time.Time) bool)
	}
	CodeName func(string) string // 可选，仅用于导出证券名称
}

func (p *Trade) Pull(ctx context.Context, code string) error {
	for i := 2000; i <= time.Now().Year(); i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := p.PullYear(ctx, i, code); err != nil {
			return err
		}
	}
	return nil
}

func (p *Trade) PullYear(ctx context.Context, year int, code string) (err error) {
	if isNil(p.Pool) {
		return fmt.Errorf("pull: 历史分笔下载需要配置 Pool")
	}
	if p.Workday == nil {
		return fmt.Errorf("pull: 历史分笔下载需要配置 Workday")
	}

	tss := protocol.Trades{}
	kss1 := protocol.Klines(nil)

	p.Workday.RangeYear(year, func(t time.Time) bool {

		select {
		case <-ctx.Done():
			err = ctx.Err()
			return false
		default:
		}

		date := t.Format("20060102")

		var resp *protocol.TradeResp
		err = p.Pool.Do(func(c tdx.Client) error {
			r, err := c.GetHistoryTradeDay(date, code)
			if err != nil {
				return err
			}
			resp, err = r.StandardResponse()
			return err
		})
		if err != nil {
			slog.Error("拉取历史分笔失败", "code", code, "date", date, "err", err)
			return false
		}

		tss = append(tss, resp.List...)

		//转成分时K线
		ks := resp.List.Klines()

		kss1 = append(kss1, ks...)

		return true
	})

	if err != nil {
		return
	}

	filename := code + "-" + conv.String(year) + ".csv"
	if err := export.TradesToCSV(filepath.Join(p.Dir, "分时成交", filename), tss); err != nil {
		return err
	}
	name := ""
	if p.CodeName != nil {
		name = p.CodeName(code)
	}
	for _, period := range []int{1, 5, 15, 30, 60} {
		if err := ctx.Err(); err != nil {
			return err
		}
		ks := kss1
		if period > 1 {
			ks = kss1.Merge241(period)
		}
		if err := export.KlinesToCSV(filepath.Join(p.Dir, conv.String(period)+"分钟", filename), code, name, ks); err != nil {
			return err
		}
	}
	return nil
}
