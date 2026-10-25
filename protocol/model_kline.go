package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"time"

	"github.com/injoyai/base/types"
	"github.com/injoyai/conv"
)

type KlineReq struct {
	Exchange Exchange
	Code     string
	Start    uint16
	Count    uint16
}

func (r *KlineReq) Bytes(category uint8) (types.Bytes, error) {
	if r.Count > 800 {
		return nil, errors.New("单次数量不能超过800")
	}
	if len(r.Code) != 6 {
		return nil, errors.New("股票代码长度错误")
	}
	return klineData(r.Exchange, r.Code, r.Start, r.Count, category), nil
}

func klineData(exchange Exchange, code string, start, count uint16, category uint8) []byte {
	data := make([]byte, 20+len(code))
	data[0] = exchange.Uint8()
	copy(data[2:], code)
	pos := 2 + len(code)
	data[pos] = category
	data[pos+2] = 1
	binary.LittleEndian.PutUint16(data[pos+4:], start)
	binary.LittleEndian.PutUint16(data[pos+6:], count)
	return data
}

type KlineResp struct {
	Count uint16
	List  []*Kline
}

// Kline 的价格使用 float64 元值（指数为点），成交额为元；成交量保留标准协议口径。
type Kline struct {
	Last      float64   // 昨收价，元（指数为点）
	Open      float64   // 开盘价，元（指数为点）
	High      float64   // 最高价，元（指数为点）
	Low       float64   // 最低价，元（指数为点）
	Close     float64   // 收盘价/该周期最新价，元（指数为点）
	Order     int       //成交单数,不一定有值
	Volume    int64     // 原始量：股票通常为手、标准指数为股；指标/本地文件须先统一为手
	Amount    float64   // 成交额，元
	Time      time.Time //时间
	UpCount   int       //上涨数量,指数有效
	DownCount int       //下跌数量,指数有效
}

func (k *Kline) String() string {
	return fmt.Sprintf("%s 昨收盘：%.3f 开盘价：%.3f 最高价：%.3f 最低价：%.3f 收盘价：%.3f 涨跌：%.3f 涨跌幅：%0.2f 成交量：%s 成交额：%s 涨跌数: %d/%d",
		k.Time.Format("2006-01-02 15:04:05"),
		k.Last, k.Open, k.High, k.Low, k.Close,
		k.RisePrice(), k.RiseRate(),
		Int64UnitString(k.Volume), FloatUnitString(k.Amount),
		k.UpCount, k.DownCount,
	)
}

// Amplitude 返回最高价与最低价之差，单位与价格相同（元/指数点），不是百分比。
func (k *Kline) Amplitude() float64 {
	return k.High - k.Low
}

// MaxDifference 返回最高价与最低价之差，单位与价格相同（元/指数点）。
func (k *Kline) MaxDifference() float64 {
	return k.High - k.Low
}

// RisePrice 返回价格涨跌额，单位与价格相同（元/指数点）；无昨收时相对开盘价计算。
func (k *Kline) RisePrice() float64 {
	if k.Last == 0 {
		//稍微数据准确点，没减去0这么夸张，还是不准的
		return k.Close - k.Open
	}
	return k.Close - k.Last

}

// RiseRate 返回涨跌幅百分数（1 表示 1%）；无昨收时相对开盘价计算。
func (k *Kline) RiseRate() float64 {
	if k.Last == 0 {
		return (k.Close - k.Open) / k.Open * 100
	}
	return (k.Close - k.Last) / k.Last * 100
}

type kline struct{}

/*
Frame
Prefix: 0c
MsgID: 0208d301
Control: 01
Length: 1c00
Length: 1c00
Type: 2d05
Data: 000030303030303104000100a401a40100000000000000000000

Data:
Exchange: 00
Unknown: 00
Code: 303030303031
Type: 04
Unknown: 00
Unknown: 0100
Start: a401
Count: a401
Append: 00000000000000000000
*/
func (kline) Frame(category uint8, code string, start, count uint16) (*Frame, error) {
	if count > 800 {
		return nil, errors.New("单次数量不能超过800")
	}

	exchange, number, err := DecodeCode(code)
	if err != nil {
		return nil, err
	}

	return &Frame{
		Control: Control01,
		Type:    TypeKline,
		Data:    klineData(exchange, number, start, count, category),
	}, nil
}

func (kline) Decode(bs []byte, c KlineCache) (*KlineResp, error) {

	if len(bs) < 2 {
		return nil, errors.New("数据长度不足")
	}
	resp := &KlineResp{
		Count: Uint16(bs[:2]),
	}
	d := wireDecoder{bs: bs[2:]}
	minRecord := 16 // 日期、四个变长价格、量和额。
	if c.Kind == KindIndex {
		minRecord += 4
	}
	if int(resp.Count) > len(d.bs)/minRecord {
		return nil, io.ErrUnexpectedEOF
	}
	if resp.Count > 0 {
		resp.List = make([]*Kline, 0, resp.Count)
	}

	var last int64 // 线上的整数厘差值先累加，再转换为元，避免累计浮点误差。
	for i := uint16(0); i < resp.Count; i++ {
		date := d.take(4)
		if d.err != nil {
			return nil, fmt.Errorf("K线记录 %d: %w", i, d.err)
		}
		k := &Kline{Time: GetTime([4]byte(date), c.Type)}
		open := int64(d.number())
		closePrice := int64(d.number())
		high := int64(d.number())
		low := int64(d.number())

		k.Last = float64(last) / 1000
		k.Open = float64(open+last) / 1000
		k.Close = float64(last+open+closePrice) / 1000
		k.High = float64(open+last+high) / 1000
		k.Low = float64(open+last+low) / 1000
		last = last + open + closePrice

		/*
			发现不同的K线数据处理不一致,测试如下:
			1分: 需要除以100
			5分: 需要除以100
			15分: 需要除以100
			30分: 需要除以100
			60分: 需要除以100
			日: 不需要操作
			周: 不需要操作
			月: 不需要操作
			季: 不需要操作
			年: 不需要操作

		*/
		k.Volume = int64(float32Value(d.uint32()))
		k.Volume = normalizeKlineVolume(k.Volume, c)
		k.Amount = float32Value(d.uint32())

		switch c.Kind {
		case KindIndex:
			//指数和股票的差别,指数多解析4字节,并处理成交量*100
			k.Volume = ToShares(k.Volume)
			k.UpCount = int(d.uint16())
			k.DownCount = int(d.uint16())
		}

		if d.err != nil {
			return nil, fmt.Errorf("K线记录 %d: %w", i, d.err)
		}
		resp.List = append(resp.List, k)
	}
	resp.List = FixKlineTime(resp.List)
	return resp, nil
}

type KlineCache struct {
	Type uint8  //1分钟,5分钟,日线等
	Kind string //指数,个股等
	Code string //证券代码,用于区分不同品种的成交量单位
}

func normalizeKlineVolume(volume int64, c KlineCache) int64 {
	switch c.Type {
	case TypeKlineMinute, TypeKline5Minute, TypeKlineMinute2, TypeKline15Minute, TypeKline30Minute, TypeKline60Minute, TypeKlineDay2:
		if !IsConvertibleBond(c.Code) {
			return FromShares(volume)
		}
	}
	return volume
}

// FixKlineTime 修复盘内下午(13~15点)拉取数据的时候,11.30的时间变成13.00
func FixKlineTime(ks []*Kline) []*Kline {
	if len(ks) == 0 {
		return ks
	}
	now := time.Now().In(Location())
	//只有当天下午13~15点之间才会出现的时间问题
	node1 := time.Date(now.Year(), now.Month(), now.Day(), 13, 0, 0, 0, now.Location())
	node2 := time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, now.Location())
	if ks[len(ks)-1].Time.Unix() < node1.Unix() || ks[len(ks)-1].Time.Unix() > node2.Unix() {
		return ks
	}
	ls := ks
	if len(ls) >= 120 {
		ls = ls[len(ls)-120:]
	}
	for i, v := range ls {
		if v.Time.Unix() == node1.Unix() {
			ls[i].Time = time.Date(now.Year(), now.Month(), now.Day(), 11, 30, 0, 0, now.Location())
		}
	}
	return ks
}

type Klines []*Kline

// MA 返回最近 n 根收盘价的均值，单位与输入价格相同，不截断到厘。
func (ks Klines) MA(n int) float64 {
	if n <= 0 || len(ks) < n {
		return 0
	}
	sum := 0.0
	// 取最后n个
	for _, k := range ks[len(ks)-n:] {
		sum += k.Close
	}
	return sum / float64(n)
}

// EMA 返回指数移动均价，单位与输入价格相同，不截断到厘。
func (ks Klines) EMA(n int) float64 {
	if len(ks) == 0 || n <= 0 {
		return 0
	}

	ema := ks[0].Close
	den := float64(n) + 1
	num := 2.0

	for i := 1; i < len(ks); i++ {
		ema = (ks[i].Close*num + ema*(den-num)) / den
	}
	return ema
}

// MACD 返回 DIF、DEA 和柱值，均与输入价格同单位，不是百分比。
func (ks Klines) MACD() (dif, dea, hist float64) {
	if len(ks) == 0 {
		return 0, 0, 0
	}

	ema12 := ks[0].Close
	ema26 := ks[0].Close
	den12 := 13.0
	den26 := 27.0
	denDea := 10.0
	num := 2.0

	for i := 1; i < len(ks); i++ {
		ema12 = (ks[i].Close*num + ema12*(den12-num)) / den12
		ema26 = (ks[i].Close*num + ema26*(den26-num)) / den26
		dif = ema12 - ema26
		dea = (dif*num + dea*(denDea-num)) / denDea
		hist = (dif - dea) * 2
	}
	return dif, dea, hist
}

// RSI 常用于超买超卖
func (ks Klines) RSI(n int) int64 {
	if len(ks) == 0 || n <= 0 {
		return 0
	}
	var gain, loss float64
	var rsi int64

	for i := 1; i < len(ks); i++ {
		diff := ks[i].Close - ks[i-1].Close

		if diff > 0 {
			gain += diff
		} else {
			loss -= diff
		}

		if i >= n+1 {
			prev := ks[i-n].Close - ks[i-n-1].Close
			if prev > 0 {
				gain -= prev
			} else {
				loss += prev
			}
		}

		if i >= n {
			rsi = 0
			if gain+loss > 0 {
				rsi = int64(100 * gain / (gain + loss))
			}
		}
	}
	return rsi
}

// BOLL 返回上轨、中轨和下轨价格，单位与输入价格相同。
func (ks Klines) BOLL(n int) (upper, mid, lower float64) {
	if len(ks) < n || n <= 0 {
		return 0, 0, 0
	}

	mid = ks.MA(n)
	var sum float64
	for _, k := range ks[len(ks)-n:] {
		d := k.Close - mid
		sum += d * d
	}
	std := math.Sqrt(sum / float64(n))
	upper = mid + std*2
	lower = mid - std*2
	return upper, mid, lower
}

// ATR 返回平均真实波幅，单位与输入价格相同，不是百分比。
func (ks Klines) ATR(n int) float64 {
	if len(ks) == 0 || n <= 0 {
		return 0
	}
	var sum float64
	var atr float64

	for i := 1; i < len(ks); i++ {
		h := ks[i].High
		l := ks[i].Low
		pc := ks[i-1].Close

		tr := max(h-l, max(math.Abs(h-pc), math.Abs(l-pc)))
		sum += tr

		if i > n {
			prev := max(ks[i-n].High-ks[i-n].Low,
				max(math.Abs(ks[i-n].High-ks[i-n-1].Close), math.Abs(ks[i-n].Low-ks[i-n-1].Close)))
			sum -= prev
		}
		if i >= n {
			atr = sum / float64(n)
		}
	}
	return atr
}

// VWAP 返回成交量加权均价（元/指数点）；Volume 为手，Amount 为成交额（元）。
func (ks Klines) VWAP() float64 {
	if len(ks) == 0 {
		return 0
	}
	var volSum int64
	var amtSum float64
	var vwap float64

	for i := 0; i < len(ks); i++ {
		volSum += ks[i].Volume
		amtSum += ks[i].Amount
		if volSum > 0 {
			vwap = amtSum / float64(volSum) / SharesPerLot
		}
	}
	return vwap
}

// LastPrice 返回最后一根收盘价，单位与输入价格相同。
func (ks Klines) LastPrice() float64 {
	if len(ks) == 0 {
		return 0
	}
	return ks[len(ks)-1].Close
}

func (ks Klines) Sort() {
	sort.Slice(ks, func(i, j int) bool {
		return ks[i].Time.Before(ks[j].Time)
	})
}

// Kline 合并 K 线；last 和结果价格与输入价格同单位，成交额保持为元。
func (ks Klines) Kline(t time.Time, last float64) *Kline {
	k := &Kline{
		Time:   t,
		Last:   last,
		Open:   last,
		High:   last,
		Low:    last,
		Close:  last,
		Volume: 0,
		Amount: 0,
	}
	for i, v := range ks {
		switch i {
		case 0:
			k.Last = v.Last
			k.Open = v.Open
			k.High = v.High
			k.Low = v.Low
			k.Close = v.Close
		default:
			if k.Open == 0 {
				k.Open = v.Open
			}
			k.High = conv.Select(k.High < v.High, v.High, k.High)
			k.Low = conv.Select(k.Low > v.Low, v.Low, k.Low)
		}
		k.Close = v.Close
		k.Volume += v.Volume
		k.Amount += v.Amount
		k.Order += v.Order
		// 涨跌家数是时点快照，周期结果取末根，不能累加。
		k.UpCount = v.UpCount
		k.DownCount = v.DownCount
	}
	return k
}

// Merge 合并成其他类型的K线
func (ks Klines) Merge(n int) Klines {
	if n <= 1 {
		return ks
	}

	if len(ks) == 0 {
		return nil
	}
	res := make(Klines, 0, 1+(len(ks)-1)/n)
	for start := 0; start < len(ks); {
		end := start + min(n, len(ks)-start)
		ls := ks[start:end]
		last := ls[len(ls)-1]
		k := ls.Kline(last.Time, ls[0].Open)
		res = append(res, k)
		start = end
	}
	return res
}

// Merge241 合并成其他类型的K线；n<=0 原样返回。
func (ks Klines) Merge241(n int) Klines {
	if n <= 0 {
		return ks
	}
	mDay := make(map[string]Klines)
	for _, v := range ks {
		day := v.Time.Format(time.DateOnly)
		mDay[day] = append(mDay[day], v)
	}

	result := Klines{}

	for _, dayKs := range mDay {
		// 构建 minute → K 映射
		m := make(map[int]*Kline, len(dayKs))
		for _, k := range dayKs {
			m[minutes(k.Time)] = k
		}

		// 按 times 构建标准 241 根分钟
		std := make([]*Kline, 0, len(minutes241))
		date := dayKs[0].Time
		var lastClose float64
		var upCount, downCount int
		for _, key := range minutes241 {
			if k, ok := m[key]; ok {
				std = append(std, k)
				lastClose = k.Close
				upCount, downCount = k.UpCount, k.DownCount
			} else {
				t := time.Date(date.Year(), date.Month(), date.Day(), key/60, key%60, 0, 0, date.Location())
				std = append(std, &Kline{
					Time:      t,
					Open:      lastClose,
					High:      lastClose,
					Low:       lastClose,
					Close:     lastClose,
					Volume:    0,
					Amount:    0,
					Order:     0,
					UpCount:   upCount,
					DownCount: downCount,
				})
			}
		}

		lenStd := len(std)
		if lenStd == 0 {
			continue
		}

		// 先把第一根独立 K 线直接加入结果
		result = append(result, std[0])

		// 从第二根开始 N 分钟合并
		for i := 1; i < lenStd; {

			k2 := &Kline{
				Last:      std[i].Last,
				Open:      std[i].Open,
				High:      std[i].High,
				Low:       std[i].Low,
				Close:     std[i].Close,
				Order:     std[i].Order,
				Volume:    std[i].Volume,
				Amount:    std[i].Amount,
				Time:      std[i].Time,
				UpCount:   std[i].UpCount,
				DownCount: std[i].DownCount,
			}

			end := i + min(n, lenStd-i)

			for j := i + 1; j < end; j++ {
				k := std[j]
				if k.High > k2.High {
					k2.High = k.High
				}
				if k.Low < k2.Low {
					k2.Low = k.Low
				}
				k2.Time = k.Time
				k2.Close = k.Close
				k2.Volume += k.Volume
				k2.Amount += k.Amount
				k2.Order += k.Order
				k2.UpCount = k.UpCount
				k2.DownCount = k.DownCount
			}

			result = append(result, k2)

			i = end
		}
	}

	result.Sort()

	return result
}

// 沪深分钟网格：09:30竞价、09:31～11:30、13:01～15:00。
var minutes241 = func() []int {
	out := make([]int, 0, 241)
	for minute := 9*60 + 30; minute <= 11*60+30; minute++ {
		out = append(out, minute)
	}
	for minute := 13*60 + 1; minute <= 15*60; minute++ {
		out = append(out, minute)
	}
	return out
}()
