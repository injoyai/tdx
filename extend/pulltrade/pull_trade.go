// Package pulltrade 按交易日下载历史分笔，并导出分笔及各分钟周期 CSV。
package pulltrade

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

func NewPullTrade(dir string) *PullTrade {
	return &PullTrade{
		Dir: dir,
	}
}

type PullTrade struct {
	Dir string
}

func (p *PullTrade) Pull(ctx context.Context, m *manage.Manage, code string) error {
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

func (p *PullTrade) PullYear(ctx context.Context, m *manage.Manage, year int, code string) (err error) {

	tss := protocol.Trades{}
	kss1 := protocol.Klines(nil)
	kss5 := protocol.Klines(nil)
	kss15 := protocol.Klines(nil)
	kss30 := protocol.Klines(nil)
	kss60 := protocol.Klines(nil)

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
		kss5 = append(kss5, ks.Merge241(5)...)
		kss15 = append(kss15, ks.Merge241(15)...)
		kss30 = append(kss30, ks.Merge241(30)...)
		kss60 = append(kss60, ks.Merge241(60)...)

		return true
	})

	if err != nil {
		return
	}

	filename := filepath.Join(p.Dir, "分时成交", code+"-"+conv.String(year)+".csv")
	filename1 := filepath.Join(p.Dir, "1分钟", code+"-"+conv.String(year)+".csv")
	filename5 := filepath.Join(p.Dir, "5分钟", code+"-"+conv.String(year)+".csv")
	filename15 := filepath.Join(p.Dir, "15分钟", code+"-"+conv.String(year)+".csv")
	filename30 := filepath.Join(p.Dir, "30分钟", code+"-"+conv.String(year)+".csv")
	filename60 := filepath.Join(p.Dir, "60分钟", code+"-"+conv.String(year)+".csv")
	name := m.Codes.GetName(code)

	err = export.TradesToCSV(filename, tss)
	if err != nil {
		return err
	}

	err = export.KlinesToCSV(filename1, code, name, kss1)
	if err != nil {
		return err
	}

	err = export.KlinesToCSV(filename5, code, name, kss5)
	if err != nil {
		return err
	}

	err = export.KlinesToCSV(filename15, code, name, kss15)
	if err != nil {
		return err
	}

	err = export.KlinesToCSV(filename30, code, name, kss30)
	if err != nil {
		return err
	}

	err = export.KlinesToCSV(filename60, code, name, kss60)
	if err != nil {
		return err
	}

	return nil
}
