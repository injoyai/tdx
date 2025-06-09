package extend

import (
	"github.com/injoyai/conv"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/protocol"
	"path/filepath"
	"time"
)

func NewPullTrade(dir string) *PullTrade {
	return &PullTrade{
		Dir: dir,
	}
}

type PullTrade struct {
	Dir string
}

func (this *PullTrade) Pull(m *tdx.Manage, year int, code string) (err error) {

	tss := protocol.Trades{}
	kss1 := protocol.Klines(nil)
	kss5 := protocol.Klines(nil)
	kss15 := protocol.Klines(nil)
	kss30 := protocol.Klines(nil)
	kss60 := protocol.Klines(nil)

	m.Workday.RangeYear(year, func(t time.Time) bool {
		date := t.Format("20060102")

		var resp *protocol.HistoryTradeResp
		err = m.Do(func(c *tdx.Client) error {
			resp, err = c.GetHistoryTradeAll(date, code)
			return err
		})
		if err != nil {
			logs.Err(err)
			return false
		}

		tss = append(tss, resp.List...)

		//转成分时K线
		ks, err := resp.List.Klines1()
		if err != nil {
			logs.Err(err)
			return false
		}

		kss1 = append(kss1, ks...)
		kss5 = append(kss5, ks.Merge(5)...)
		kss15 = append(kss5, ks.Merge(15)...)
		kss30 = append(kss5, ks.Merge(30)...)
		kss60 = append(kss5, ks.Merge(60)...)

		return true
	})

	_ = kss5
	_ = kss15
	_ = kss30
	_ = kss60

	filename := filepath.Join(this.Dir, conv.String(year), "分时成交", code+".csv")
	filename1 := filepath.Join(this.Dir, conv.String(year), "1分钟", code+".csv")
	filename5 := filepath.Join(this.Dir, conv.String(year), "5分钟", code+".csv")
	filename15 := filepath.Join(this.Dir, conv.String(year), "15分钟", code+".csv")
	filename30 := filepath.Join(this.Dir, conv.String(year), "30分钟", code+".csv")
	filename60 := filepath.Join(this.Dir, conv.String(year), "60分钟", code+".csv")
	name := m.Codes.GetName(code)

	err = TradeToCsv(filename, tss)
	if err != nil {
		return err
	}

	err = KlinesToCsv(filename1, code, name, kss1)
	if err != nil {
		return err
	}

	err = KlinesToCsv(filename5, code, name, kss5)
	if err != nil {
		return err
	}

	err = KlinesToCsv(filename15, code, name, kss15)
	if err != nil {
		return err
	}

	err = KlinesToCsv(filename30, code, name, kss30)
	if err != nil {
		return err
	}

	err = KlinesToCsv(filename60, code, name, kss60)
	if err != nil {
		return err
	}

	return nil
}

func KlinesToCsv(filename string, code, name string, ks protocol.Klines) error {
	data := [][]any{{"日期", "时间", "代码", "名称", "开盘", "最高", "最低", "收盘", "总手", "金额", "涨幅", "涨幅比"}}
	for _, v := range ks {
		data = append(data, []any{
			v.Time.Format("20060102"),
			v.Time.Format("15:04"),
			code,
			name,
			v.Open.Float64(),
			v.High.Float64(),
			v.Low.Float64(),
			v.Close.Float64(),
			v.Volume,
			v.Amount.Float64(),
			v.RisePrice().Float64(),
			v.RiseRate(),
		})
	}

	buf, err := toCsv(data)
	if err != nil {
		return err
	}

	return newFile(filename, buf)
}

func TradeToCsv(filename string, ts protocol.Trades) error {
	data := [][]any{{"日期", "时间", "价格", "成交量(手)", "成交额", "买卖方向"}}
	for _, v := range ts {
		data = append(data, []any{
			v.Time.Format(time.DateOnly),
			v.Time.Format("15:04"),
			v.Price.Float64(),
			v.Volume,
			v.Amount().Float64(),
			v.Status,
		})
	}
	buf, err := toCsv(data)
	if err != nil {
		return err
	}
	return newFile(filename, buf)
}
