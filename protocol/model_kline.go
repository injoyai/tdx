package protocol

import (
	"errors"
	"fmt"
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

func (this *KlineReq) Bytes(Type uint8) (types.Bytes, error) {
	if this.Count > 800 {
		return nil, errors.New("单次数量不能超过800")
	}
	if len(this.Code) != 6 {
		return nil, errors.New("股票代码长度错误")
	}
	data := []byte{this.Exchange.Uint8(), 0x0}
	data = append(data, []byte(this.Code)...) //这里怎么是正序了？
	data = append(data, Type, 0x0)
	data = append(data, 0x01, 0x0)
	data = append(data, Bytes(this.Start)...)
	data = append(data, Bytes(this.Count)...)
	data = append(data, make([]byte, 10)...) //未知啥含义
	return data, nil
}

type KlineResp struct {
	Count uint16
	List  []*Kline
}

type Kline struct {
	Last      Price     //昨日收盘价,这个是列表的上一条数据的收盘价，如果没有上条数据，那么这个值为0
	Open      Price     //开盘价
	High      Price     //最高价
	Low       Price     //最低价
	Close     Price     //收盘价,如果是当天,则是最新价/实时价
	Order     int       //成交单数,不一定有值
	Volume    int64     //成交量
	Amount    Price     //成交额
	Time      time.Time //时间
	UpCount   int       //上涨数量,指数有效
	DownCount int       //下跌数量,指数有效
}

func (this *Kline) String() string {
	return fmt.Sprintf("%s 昨收盘：%.3f 开盘价：%.3f 最高价：%.3f 最低价：%.3f 收盘价：%.3f 涨跌：%s 涨跌幅：%0.2f 成交量：%s 成交额：%s 涨跌数: %d/%d",
		this.Time.Format("2006-01-02 15:04:05"),
		this.Last.Float64(), this.Open.Float64(), this.High.Float64(), this.Low.Float64(), this.Close.Float64(),
		this.RisePrice(), this.RiseRate(),
		Int64UnitString(this.Volume), FloatUnitString(this.Amount.Float64()),
		this.UpCount, this.DownCount,
	)
}

// MaxDifference 最大差值，最高-最低
func (this *Kline) MaxDifference() Price {
	return this.High - this.Low
}

// RisePrice 涨跌金额,第一个数据不准，仅做参考
func (this *Kline) RisePrice() Price {
	if this.Last == 0 {
		//稍微数据准确点，没减去0这么夸张，还是不准的
		return this.Close - this.Open
	}
	return this.Close - this.Last

}

// RiseRate 涨跌比例/涨跌幅,第一个数据不准，仅做参考
func (this *Kline) RiseRate() float64 {
	if this.Last == 0 {
		return float64(this.Close-this.Open) / float64(this.Open) * 100
	}
	return float64(this.Close-this.Last) / float64(this.Last) * 100
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
func (kline) Frame(Type uint8, code string, start, count uint16) (*Frame, error) {
	if count > 800 {
		return nil, errors.New("单次数量不能超过800")
	}

	exchange, number, err := DecodeCode(code)
	if err != nil {
		return nil, err
	}

	data := []byte{exchange.Uint8(), 0x0}
	data = append(data, []byte(number)...) //这里怎么是正序了？
	data = append(data, Type, 0x0)
	data = append(data, 0x01, 0x0)
	data = append(data, Bytes(start)...)
	data = append(data, Bytes(count)...)
	data = append(data, make([]byte, 10)...) //未知啥含义

	return &Frame{
		Control: Control01,
		Type:    TypeKline,
		Data:    data,
	}, nil
}

func (kline) Decode(bs []byte, c KlineCache) (*KlineResp, error) {

	if len(bs) < 2 {
		return nil, errors.New("数据长度不足")
	}
	resp := &KlineResp{
		Count: Uint16(bs[:2]),
	}
	bs = bs[2:]

	var last Price //上条数据(昨天)的收盘价
	for i := uint16(0); i < resp.Count; i++ {
		k := &Kline{
			Time: GetTime([4]byte(bs[:4]), c.Type),
		}

		var open Price
		bs, open = GetPrice(bs[4:])
		var _close Price
		bs, _close = GetPrice(bs)
		var high Price
		bs, high = GetPrice(bs)
		var low Price
		bs, low = GetPrice(bs)

		k.Last = last
		k.Open = open + last
		k.Close = last + open + _close
		k.High = open + last + high
		k.Low = open + last + low
		last = last + open + _close

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
		k.Volume = int64(getVolume(Uint32(bs[:4])))
		bs = bs[4:]
		switch c.Type {
		case TypeKlineMinute, TypeKline5Minute, TypeKlineMinute2, TypeKline15Minute, TypeKline30Minute, TypeKline60Minute, TypeKlineDay2:
			k.Volume /= 100
		}
		k.Amount = Price(getVolume(Uint32(bs[:4])) * 1000) //从元转为厘,并去除多余的小数
		bs = bs[4:]

		switch c.Kind {
		case KindIndex:
			//指数和股票的差别,指数多解析4字节,并处理成交量*100
			k.Volume *= 100
			k.UpCount = conv.Int([]byte{bs[1], bs[0]})
			k.DownCount = conv.Int([]byte{bs[3], bs[2]})
			bs = bs[4:]
		}

		resp.List = append(resp.List, k)
	}
	resp.List = FixKlineTime(resp.List)
	return resp, nil
}

type KlineCache struct {
	Type uint8  //1分钟,5分钟,日线等
	Kind string //指数,个股等
}

// FixKlineTime 修复盘内下午(13~15点)拉取数据的时候,11.30的时间变成13.00
func FixKlineTime(ks []*Kline) []*Kline {
	if len(ks) == 0 {
		return ks
	}
	now := time.Now()
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

// LastPrice 获取最后一个K线的收盘价
func (this Klines) LastPrice() Price {
	if len(this) == 0 {
		return 0
	}
	return this[len(this)-1].Close
}

func (this Klines) Len() int {
	return len(this)
}

func (this Klines) Swap(i, j int) {
	this[i], this[j] = this[j], this[i]
}

func (this Klines) Less(i, j int) bool {
	return this[i].Time.Before(this[j].Time)
}

func (this Klines) Sort() {
	sort.Sort(this)
}

func (this Klines) Kline(t time.Time, last Price) *Kline {
	k := &Kline{
		Time:   t,
		Open:   last,
		High:   last,
		Low:    last,
		Close:  last,
		Volume: 0,
		Amount: 0,
	}
	for i, v := range this {
		switch i {
		case 0:
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
	}
	return k
}

// Merge 合并成其他类型的K线
func (this Klines) Merge(n int) Klines {
	if n <= 1 {
		return this
	}

	ks := Klines(nil)
	ls := Klines(nil)
	for i := 0; ; i++ {
		if len(this) <= i*n {
			break
		}
		if len(this) < (i+1)*n {
			ls = this[i*n:]
		} else {
			ls = this[i*n : (i+1)*n]
		}
		if len(ls) == 0 {
			break
		}
		last := ls[len(ls)-1]
		k := ls.Kline(last.Time, ls[0].Open)
		ks = append(ks, k)
	}
	return ks
}

// Merge241 合并成其他类型的K线
func (ks Klines) Merge241(n int) Klines {
	mDay := make(map[string]Klines)
	for _, v := range ks {
		day := v.Time.Format(time.DateOnly)
		mDay[day] = append(mDay[day], v)
	}

	result := Klines{}

	for _, dayKs := range mDay {
		// 构建 minute → K 映射
		m := make(map[string]*Kline, len(dayKs))
		for _, k := range dayKs {
			key := k.Time.Format(timeFormat)
			m[key] = k
		}

		// 按 times 构建标准 241 根分钟
		std := make([]*Kline, 0, len(times241))
		var lastClose Price
		for _, key := range times241 {
			if k, ok := m[key]; ok {
				std = append(std, k)
				lastClose = k.Close
			} else {
				t, _ := time.ParseInLocation(timeFormat, key, time.Local)
				std = append(std, &Kline{
					Time:      t,
					Open:      lastClose,
					High:      lastClose,
					Low:       lastClose,
					Close:     lastClose,
					Volume:    0,
					Amount:    0,
					Order:     0,
					UpCount:   0,
					DownCount: 0,
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

			end := i + n
			if end > lenStd {
				end = lenStd
			}

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
			}

			result = append(result, k2)

			i = end
		}
	}

	result.Sort()

	return result
}

var (
	times241 = []string{
		// 上午 09:30 – 11:29
		"09:30", "09:31", "09:32", "09:33", "09:34", "09:35", "09:36", "09:37", "09:38", "09:39",
		"09:40", "09:41", "09:42", "09:43", "09:44", "09:45", "09:46", "09:47", "09:48", "09:49",
		"09:50", "09:51", "09:52", "09:53", "09:54", "09:55", "09:56", "09:57", "09:58", "09:59",
		"10:00", "10:01", "10:02", "10:03", "10:04", "10:05", "10:06", "10:07", "10:08", "10:09",
		"10:10", "10:11", "10:12", "10:13", "10:14", "10:15", "10:16", "10:17", "10:18", "10:19",
		"10:20", "10:21", "10:22", "10:23", "10:24", "10:25", "10:26", "10:27", "10:28", "10:29",
		"10:30", "10:31", "10:32", "10:33", "10:34", "10:35", "10:36", "10:37", "10:38", "10:39",
		"10:40", "10:41", "10:42", "10:43", "10:44", "10:45", "10:46", "10:47", "10:48", "10:49",
		"10:50", "10:51", "10:52", "10:53", "10:54", "10:55", "10:56", "10:57", "10:58", "10:59",
		"11:00", "11:01", "11:02", "11:03", "11:04", "11:05", "11:06", "11:07", "11:08", "11:09",
		"11:10", "11:11", "11:12", "11:13", "11:14", "11:15", "11:16", "11:17", "11:18", "11:19",
		"11:20", "11:21", "11:22", "11:23", "11:24", "11:25", "11:26", "11:27", "11:28", "11:29",
		"11:30",

		// 下午 13:01 – 15:00
		"13:01", "13:02", "13:03", "13:04", "13:05", "13:06", "13:07", "13:08", "13:09", "13:10",
		"13:11", "13:12", "13:13", "13:14", "13:15", "13:16", "13:17", "13:18", "13:19", "13:20",
		"13:21", "13:22", "13:23", "13:24", "13:25", "13:26", "13:27", "13:28", "13:29", "13:30",
		"13:31", "13:32", "13:33", "13:34", "13:35", "13:36", "13:37", "13:38", "13:39", "13:40",
		"13:41", "13:42", "13:43", "13:44", "13:45", "13:46", "13:47", "13:48", "13:49", "13:50",
		"13:51", "13:52", "13:53", "13:54", "13:55", "13:56", "13:57", "13:58", "13:59", "14:00",
		"14:01", "14:02", "14:03", "14:04", "14:05", "14:06", "14:07", "14:08", "14:09", "14:10",
		"14:11", "14:12", "14:13", "14:14", "14:15", "14:16", "14:17", "14:18", "14:19", "14:20",
		"14:21", "14:22", "14:23", "14:24", "14:25", "14:26", "14:27", "14:28", "14:29", "14:30",
		"14:31", "14:32", "14:33", "14:34", "14:35", "14:36", "14:37", "14:38", "14:39", "14:40",
		"14:41", "14:42", "14:43", "14:44", "14:45", "14:46", "14:47", "14:48", "14:49", "14:50",
		"14:51", "14:52", "14:53", "14:54", "14:55", "14:56", "14:57", "14:58", "14:59", "15:00",
	}

	timeFormat = "15:04"
)
