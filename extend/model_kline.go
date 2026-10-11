package extend

import "github.com/injoyai/tdx/v2/protocol"

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

// REF 前n天
func (this Klines) REF(n int) *Kline {
	return this[len(this)-n-1]
}

// HHV 近n天的最高价,同tdx公式命名
func (this Klines) HHV(n int) protocol.Price {
	p := protocol.Price(0)
	for i := len(this) - n; i < len(this); i++ {
		if p < this[i].High {
			p = this[i].High
		}
	}
	return p
}

// LLV 近n天的最低价,同tdx公式命名
func (this Klines) LLV(n int) protocol.Price {
	p := protocol.Price(0)
	for i := len(this) - n; i < len(this); i++ {
		if p == 0 || p > this[i].Low {
			p = this[i].Low
		}
	}
	return p
}

// MA 均线
func (ks Klines) MA(n int) protocol.Price {
	if len(ks) < n {
		return 0
	}
	sum := protocol.Price(0)
	// 取最后n个
	for _, k := range ks[len(ks)-n:] {
		sum += k.Close
	}
	return sum / protocol.Price(n)
}

// EMA MACD的基础
func (ks Klines) EMA(n int) protocol.Price {
	if len(ks) == 0 || n <= 0 {
		return 0
	}

	ema := ks[0].Close
	den := int64(n + 1)
	num := int64(2)

	for i := 1; i < len(ks); i++ {
		ema = protocol.Price(
			(int64(ks[i].Close)*num + int64(ema)*(den-num)) / den,
		)
	}
	return ema
}

// MACD 常用于短线核心
func (ks Klines) MACD() (dif, dea, hist protocol.Price) {
	if len(ks) == 0 {
		return 0, 0, 0
	}

	ema12 := ks[0].Close
	ema26 := ks[0].Close
	den12 := int64(13)
	den26 := int64(27)
	denDea := int64(10)
	num := int64(2)

	for i := 1; i < len(ks); i++ {
		ema12 = protocol.Price((int64(ks[i].Close)*num + int64(ema12)*(den12-num)) / den12)
		ema26 = protocol.Price((int64(ks[i].Close)*num + int64(ema26)*(den26-num)) / den26)
		dif = ema12 - ema26
		dea = protocol.Price((int64(dif)*num + int64(dea)*(denDea-num)) / denDea)
		hist = (dif - dea) * 2
	}
	return dif, dea, hist
}

// RSI 常用于超买超卖
func (ks Klines) RSI(n int) int64 {
	if len(ks) == 0 || n <= 0 {
		return 0
	}
	var gain, loss int64
	var rsi int64

	for i := 1; i < len(ks); i++ {
		diff := int64(ks[i].Close - ks[i-1].Close)

		if diff > 0 {
			gain += diff
		} else {
			loss -= diff
		}

		if i >= n+1 {
			prev := int64(ks[i-n].Close - ks[i-n-1].Close)
			if prev > 0 {
				gain -= prev
			} else {
				loss += prev
			}
		}

		if i >= n {
			rsi = 0
			if gain+loss > 0 {
				rsi = 100 * gain / (gain + loss)
			}
		}
	}
	return rsi
}

// BOLL 布林带（洗盘神器）
func (ks Klines) BOLL(n int) (upper, mid, lower protocol.Price) {
	if len(ks) < n || n <= 0 {
		return 0, 0, 0
	}

	mid = ks.MA(n)
	var sum int64
	for _, k := range ks[len(ks)-n:] {
		d := int64(k.Close - mid)
		sum += d * d
	}
	std := protocol.I64Sqrt(sum / int64(n))
	upper = mid + protocol.Price(std*2)
	lower = mid - protocol.Price(std*2)
	return upper, mid, lower
}

// ATR 常用于判断是否该止损
func (ks Klines) ATR(n int) protocol.Price {
	if len(ks) == 0 || n <= 0 {
		return 0
	}
	var sum int64
	var atr protocol.Price

	for i := 1; i < len(ks); i++ {
		h := ks[i].High
		l := ks[i].Low
		pc := ks[i-1].Close

		tr := max(h-l, max((h-pc).Abs(), (l-pc).Abs()))
		sum += int64(tr)

		if i > n {
			prev := max(ks[i-n].High-ks[i-n].Low,
				max((ks[i-n].High-ks[i-n-1].Close).Abs(), (ks[i-n].Low-ks[i-n-1].Close).Abs()))
			sum -= int64(prev)
		}
		if i >= n {
			atr = protocol.Price(sum / int64(n))
		}
	}
	return atr
}

// VWAP 成交量加权均价，Volume 需为手，Amount 为实际成交额。
func (ks Klines) VWAP() protocol.Price {
	if len(ks) == 0 {
		return 0
	}
	var volSum, amtSum int64
	var vwap protocol.Price

	for i := 0; i < len(ks); i++ {
		volSum += ks[i].Volume
		amtSum += int64(ks[i].Amount)
		if volSum > 0 {
			vwap = protocol.Price(amtSum / volSum / 100)
		}
	}
	return vwap
}
