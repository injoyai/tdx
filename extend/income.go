package extend

import (
	"fmt"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func DoIncomes(ks protocol.Klines, startAt time.Time, days ...int) Incomes {
	year, month, day := startAt.Date()
	start := time.Date(year, month, day, 15, 0, 0, 0, startAt.Location()).Unix()
	index := len(ks)
	for i, v := range ks {
		if v.Time.Unix() >= start {
			index = i
			break
		}
	}
	ks = ks[index:]

	ls := Incomes{}

	for _, v := range days {
		if v >= 0 && v < len(ks) {
			x := ks[v]
			ls = append(ls, &Income{
				Offset: v,
				Time:   x.Time,
				Source: protocol.Kline{
					Open:  ks[0].Open,
					High:  ks[0].High,
					Low:   ks[0].Low,
					Close: ks[0].Close,
				},
				Current: protocol.Kline{
					Open:  x.Open,
					High:  x.High,
					Low:   x.Low,
					Close: x.Close,
				},
			})
		}
	}

	return ls
}

type Incomes []*Income

type Income struct {
	Offset  int            //偏移量
	Time    time.Time      //时间
	Source  protocol.Kline //源
	Current protocol.Kline //当前
}

func (i *Income) String() string {
	return fmt.Sprintf("偏移: %d, 时间: %s, 涨幅: %.1f%%", i.Offset, i.Time.Format(time.DateOnly), i.RiseRate()*100)
}

func (i *Income) Rise() protocol.Price {
	return i.Current.Close - i.Source.Close
}

func (i *Income) RiseRate() float64 {
	return i.Rise().Float64() / i.Source.Close.Float64()
}
