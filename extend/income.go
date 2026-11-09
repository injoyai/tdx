package extend

import (
	"fmt"
	"math"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

// DoIncomes 从 startAt 所在的东八区交易日期起计算收盘价涨跌；兼容零点与 15:00 日线时间。
// ks 须按时间升序排列，记录非 nil 且收盘价有限。offsets 按 K 线条数偏移，0 为基准记录；负数及越界项忽略。
func DoIncomes(ks protocol.Klines, startAt time.Time, offsets ...int) (Incomes, error) {
	year, month, day := startAt.In(protocol.Location()).Date()
	start := time.Date(year, month, day, 0, 0, 0, 0, protocol.Location()).Unix()
	index := len(ks)
	for i, v := range ks {
		if v == nil {
			return nil, fmt.Errorf("income: 第 %d 条 K 线为空", i)
		}
		if math.IsNaN(v.Close) || math.IsInf(v.Close, 0) {
			return nil, fmt.Errorf("income: 第 %d 条 K 线收盘价无效", i)
		}
		if i > 0 && v.Time.Before(ks[i-1].Time) {
			return nil, fmt.Errorf("income: K 线未按时间升序排列，第 %d 条早于前一条", i)
		}
		if index == len(ks) && v.Time.Unix() >= start {
			index = i
		}
	}
	ks = ks[index:]

	ls := Incomes{}

	for _, v := range offsets {
		if v >= 0 && v < len(ks) {
			x := ks[v]
			ls = append(ls, &Income{
				Offset:  v,
				Time:    x.Time,
				Source:  *ks[0],
				Current: *x,
			})
		}
	}

	return ls, nil
}

type Incomes []*Income

type Income struct {
	Offset  int            // K 线条数偏移，0 为基准记录
	Time    time.Time      //时间
	Source  protocol.Kline //源
	Current protocol.Kline //当前
}

func (i *Income) String() string {
	if i == nil {
		return "<nil>"
	}
	rise := "不可计算"
	if rate, available := i.RiseRate(); available {
		if percent := rate * 100; !math.IsInf(percent, 0) {
			rise = fmt.Sprintf("%.1f%%", percent)
		} else {
			rise = "超出显示范围"
		}
	}
	return fmt.Sprintf("偏移: %d, 时间: %s, 涨幅: %s", i.Offset, i.Time.Format(time.DateOnly), rise)
}

// Rise 返回收盘价涨跌额，单位与源价格相同（元/指数点）。
func (i *Income) Rise() float64 {
	return i.Current.Close - i.Source.Close
}

// RiseRate 返回收盘价涨跌比例及可用性，0.01 表示 1%，不是百分数。
// 基准价为零、价格或计算结果非有限值时不可用；真实零涨跌返回 (0, true)。
func (i *Income) RiseRate() (float64, bool) {
	if i == nil || i.Source.Close == 0 {
		return 0, false
	}
	rate := i.Rise() / i.Source.Close
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0, false
	}
	return rate, true
}
