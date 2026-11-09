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

// Pull 下载明确年份范围（含两端）。
func (p *Trade) Pull(ctx context.Context, startYear, endYear int, code string) error {
	if ctx == nil {
		return fmt.Errorf("pull: nil context")
	}
	if startYear < 1 || endYear > 9999 || startYear > endYear {
		return fmt.Errorf("pull: 无效年份范围 %d..%d", startYear, endYear)
	}
	for i := startYear; i <= endYear; i++ {
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
	if ctx == nil {
		return fmt.Errorf("pull: nil context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if year < 1 || year > 9999 {
		return fmt.Errorf("pull: 无效年份 %d", year)
	}
	if isNil(p.Pool) {
		return fmt.Errorf("pull: 历史分笔下载需要配置 Pool")
	}
	if isNil(p.Workday) {
		return fmt.Errorf("pull: 历史分笔下载需要配置 Workday")
	}

	tss := []tdx.Trade{}
	kss1 := protocol.Klines(nil)

	p.Workday.RangeYear(year, func(t time.Time) bool {

		select {
		case <-ctx.Done():
			err = ctx.Err()
			return false
		default:
		}

		date := t.Format("20060102")

		var resp *tdx.TradeResp
		err = p.Pool.DoContext(ctx, func(c tdx.Client) error {
			c = tdx.WithContext(ctx, c)
			r, err := c.GetHistoryTradeDay(date, code)
			if err != nil {
				return err
			}
			resp = r
			return nil
		})
		if err == nil {
			err = ctx.Err()
		}
		if err != nil {
			slog.Error("拉取历史分笔失败", "code", code, "date", date, "err", err)
			return false
		}

		tss = append(tss, resp.List...)

		// 竞价／午休合成继续复用标准算法，CSV 使用统一业务字段。
		raw, rawErr := resp.StandardResponse()
		if rawErr != nil {
			err = rawErr
			return false
		}
		ks := raw.List.Klines()

		kss1 = append(kss1, ks...)

		return true
	})

	if err != nil {
		return
	}
	if err := ctx.Err(); err != nil {
		return err
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
		rows := make([]tdx.Kline, 0, len(ks))
		for _, k := range ks {
			amount := k.Amount
			rows = append(rows, tdx.Kline{Time: k.Time.Format(protocol.TimeLayout), Open: k.Open, High: k.High,
				Low: k.Low, Close: k.Close, Volume: protocol.ToShares(k.Volume), Amount: &amount})
		}
		if err := export.KlinesToCSV(filepath.Join(p.Dir, conv.String(period)+"分钟", filename), code, name, rows); err != nil {
			return err
		}
	}
	return nil
}
