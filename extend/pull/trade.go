package pull

import (
	"context"
	"path/filepath"
	"time"

	"github.com/injoyai/conv"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/export"
	"github.com/injoyai/tdx/v2/extend/manage"
	"github.com/injoyai/tdx/v2/protocol"
)

// NewTrade 创建历史分笔下载器，分笔及合成分钟线 CSV 写入 dir。
func NewTrade(dir string) *Trade {
	return &Trade{
		Dir: dir,
	}
}

// Trade 按交易日下载历史分笔并导出 CSV；管理器资源由调用方持有。
type Trade struct {
	Dir string
}

func (p *Trade) Pull(ctx context.Context, m *manage.Manage, code string) error {
	for i := 2000; i <= time.Now().Year(); i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := p.PullYear(ctx, m, i, code); err != nil {
			return err
		}
	}
	return nil
}

func (p *Trade) PullYear(ctx context.Context, m *manage.Manage, year int, code string) (err error) {

	tss := protocol.Trades{}
	kss1 := protocol.Klines(nil)

	m.Workday.RangeYear(year, func(t time.Time) bool {

		select {
		case <-ctx.Done():
			err = ctx.Err()
			return false
		default:
		}

		date := t.Format("20060102")

		var resp *protocol.TradeResp
		err = m.Do(func(c tdx.Client) error {
			r, err := c.GetHistoryTradeDay(date, code)
			if err != nil {
				return err
			}
			resp, err = r.StandardResponse()
			return err
		})
		if err != nil {
			logs.Err(err)
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
	name := m.Codes.GetName(code)
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
