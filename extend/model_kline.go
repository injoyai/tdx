package extend

import "github.com/injoyai/tdx/protocol"

type Info struct {
	Code       string         `json:"code"`       //代码
	Name       string         `json:"name"`       //名称
	Price      protocol.Price `json:"price"`      //最新价
	Turnover   float64        `json:"turnover"`   //换手率
	FloatStock int64          `json:"floatStock"` //流通股本
	TotalStock int64          `json:"totalStock"` //总股本
	FloatValue protocol.Price `json:"floatValue"` //流通市值
	TotalValue protocol.Price `json:"totalValue"` //总市值
}

type Kline struct {
	Unix            int64 `xorm:"pk"`
	*protocol.Kline `xorm:"extends"`
	Turnover        float64
	FloatStock      int64
	TotalStock      int64
	//InsideDish      int64
	//OuterDisc       int64
}

func (this *Kline) FloatValue() protocol.Price {
	return this.Close * protocol.Price(this.FloatStock)
}

func (this *Kline) TotalValue() protocol.Price {
	return this.Close * protocol.Price(this.TotalStock)
}

/*



 */

type Klines []*Kline

// MA 均线
func (ks Klines) MA(n int) []protocol.Price {
	out := make([]protocol.Price, len(ks))
	var sum int64

	for i := 0; i < len(ks); i++ {
		sum += int64(ks[i].Close)

		if i >= n {
			sum -= int64(ks[i-n].Close)
		}

		if i >= n-1 {
			out[i] = protocol.Price(sum / int64(n))
		}
	}
	return out
}

// EMA MACD的基础
func (ks Klines) EMA(n int) []protocol.Price {
	out := make([]protocol.Price, len(ks))
	if len(ks) == 0 {
		return out
	}

	out[0] = ks[0].Close
	den := int64(n + 1)
	num := int64(2)

	for i := 1; i < len(ks); i++ {
		out[i] = protocol.Price(
			(int64(ks[i].Close)*num + int64(out[i-1])*(den-num)) / den,
		)
	}
	return out
}

// MACD 常用于短线核心
func (ks Klines) MACD() (dif, dea, hist []protocol.Price) {
	ema12 := ks.EMA(12)
	ema26 := ks.EMA(26)

	n := len(ks)
	dif = make([]protocol.Price, n)
	for i := 0; i < n; i++ {
		dif[i] = ema12[i] - ema26[i]
	}

	dea = make([]protocol.Price, n)
	dea[0] = dif[0]

	// DEA = EMA(dif, 9)
	den := int64(10)
	num := int64(2)

	for i := 1; i < n; i++ {
		dea[i] = protocol.Price((int64(dif[i])*num + int64(dea[i-1])*(den-num)) / den)
	}

	hist = make([]protocol.Price, n)
	for i := 0; i < n; i++ {
		hist[i] = (dif[i] - dea[i]) * 2
	}
	return
}

// RSI 常用于超买超卖
func (ks Klines) RSI(n int) []int64 {
	out := make([]int64, len(ks))
	var gain, loss int64

	for i := 1; i < len(ks); i++ {
		diff := int64(ks[i].Close - ks[i-1].Close)

		if diff > 0 {
			gain += diff
		} else {
			loss -= diff
		}

		if i >= n {
			prev := int64(ks[i-n].Close - ks[i-n-1].Close)
			if prev > 0 {
				gain -= prev
			} else {
				loss += prev
			}
		}

		if i >= n && loss > 0 {
			out[i] = 100 * gain / (gain + loss)
		}
	}
	return out
}

// BOLL 布林带（洗盘神器）
func (ks Klines) BOLL(n int) (upper, mid, lower []protocol.Price) {
	mid = ks.MA(n)
	upper = make([]protocol.Price, len(ks))
	lower = make([]protocol.Price, len(ks))

	for i := n - 1; i < len(ks); i++ {
		var sum int64
		for j := i - n + 1; j <= i; j++ {
			d := int64(ks[j].Close - mid[i])
			sum += d * d
		}

		std := protocol.I64Sqrt(sum / int64(n))
		upper[i] = mid[i] + protocol.Price(std*2)
		lower[i] = mid[i] - protocol.Price(std*2)
	}
	return
}

// ATR 常用于判断是否该止损
func (ks Klines) ATR(n int) []protocol.Price {
	out := make([]protocol.Price, len(ks))
	var sum int64

	for i := 1; i < len(ks); i++ {
		h := ks[i].High
		l := ks[i].Low
		pc := ks[i-1].Close

		tr := max(h-l, max((h-pc).Abs(), (l-pc).Abs()))
		sum += int64(tr)

		if i >= n {
			prev := max(ks[i-n+1].High-ks[i-n+1].Low,
				max((ks[i-n+1].High-ks[i-n].Close).Abs(), (ks[i-n+1].Low-ks[i-n].Close).Abs()))
			sum -= int64(prev)
			out[i] = protocol.Price(sum / int64(n))
		}
	}
	return out
}

func (ks Klines) VWAP() []protocol.Price {
	out := make([]protocol.Price, len(ks))
	var volSum, amtSum int64

	for i := 0; i < len(ks); i++ {
		volSum += ks[i].Volume
		amtSum += int64(ks[i].Amount)
		if volSum > 0 {
			out[i] = protocol.Price(amtSum / volSum)
		}
	}
	return out
}
