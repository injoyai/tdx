package protocol

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/injoyai/base/types"
	"github.com/injoyai/conv"
)

type TradeResp struct {
	Count uint16
	List  Trades
}

// Trade 分时成交, 时间没有到秒，客户端上也没有,东方客户端能显示秒
type Trade struct {
	Time   time.Time //时间, 09:30
	Price  float64   // 成交价，元（指数为点）
	Volume int       //成交量,手
	Status int       //0是买，1是卖，2中性/汇总 中途也可能出现2,例20241115(sz000001)的14:56
	Number int       //单数,历史数据该字段无效
}

func (t *Trade) String() string {
	return fmt.Sprintf("%s \t%-6g \t%-6g \t%-6d(手) \t%-4d(单) \t%-4s",
		t.Time, t.Price, t.Amount(), t.Volume, t.Number, t.StatusString())
}

// Amount 返回成交额（元），按成交价乘以手数×100 计算。
func (t *Trade) Amount() float64 {
	return t.Price * float64(ToShares(int64(t.Volume)))
}

func (t *Trade) StatusString() string {
	switch t.Status {
	case 0:
		return "买入"
	case 1:
		return "卖出"
	default:
		return ""
	}
}

// AvgVolume 平均每单成交量
func (t *Trade) AvgVolume() float64 {
	return float64(t.Volume) / float64(t.Number)
}

// AvgAmount 返回平均每单成交额，单位为元。
func (t *Trade) AvgAmount() float64 {
	return t.AvgVolume() * t.Price * SharesPerLot
}

// IsBuy 是否是买单
func (t *Trade) IsBuy() bool {
	return t.Status == 0
}

// IsSell 是否是卖单
func (t *Trade) IsSell() bool {
	return t.Status == 1
}

type trade struct{}

func (trade) Frame(code string, start, count uint16) (*Frame, error) {
	exchange, number, err := DecodeCode(code)
	if err != nil {
		return nil, err
	}

	codeBs := []byte(number)
	codeBs = append(codeBs, Bytes(start)...)
	codeBs = append(codeBs, Bytes(count)...)
	return &Frame{
		Control: Control01,
		Type:    TypeTrade,
		Data:    append([]byte{exchange.Uint8(), 0x0}, codeBs...),
	}, nil
}

func (trade) Decode(bs []byte, c TradeCache) (*TradeResp, error) {

	if len(bs) < 2 {
		return nil, errors.New("数据长度不足")
	}

	return decodeTrades(bs[2:], Uint16(bs[:2]), c, false)
}

func decodeTrades(bs []byte, count uint16, c TradeCache, historical bool) (*TradeResp, error) {
	resp := &TradeResp{Count: count}
	fields := 5
	if historical {
		fields = 4
	}
	if int(count) > len(bs)/(2+fields) {
		return nil, io.ErrUnexpectedEOF
	}
	if count == 0 {
		return resp, nil
	}
	date, err := time.ParseInLocation("20060102", c.Date, Location())
	if err != nil {
		return nil, err
	}
	resp.List = make(Trades, 0, count)
	multiple := basePrice(c.Code)
	var lastPrice int64
	for i := uint16(0); i < resp.Count; i++ {
		if len(bs) < 2 {
			return nil, fmt.Errorf("成交记录 %d: 时间数据不足", i)
		}
		clock := int(Uint16(bs[:2]))
		if clock >= 24*60 {
			return nil, fmt.Errorf("成交记录 %d: 无效时间 %d", i, clock)
		}
		t := time.Date(date.Year(), date.Month(), date.Day(), clock/60, clock%60, 0, 0, Location())
		mt := &Trade{Time: t}
		bs = bs[2:]
		var values [5]int
		for field := 0; field < fields; field++ {
			bs, values[field], err = cutIntChecked(bs)
			if err != nil {
				return nil, fmt.Errorf("成交记录 %d 字段 %d: %w", i, field, err)
			}
		}
		lastPrice += int64(values[0]) // 先累计线上整数分，再换算为元。
		mt.Price = float64(lastPrice) / 100 / multiple
		mt.Volume = values[1]
		mt.Status = values[2]
		if !historical {
			mt.Number = values[2]
			mt.Status = values[3]
		}
		resp.List = append(resp.List, mt)
	}

	return resp, nil
}

type Trades []*Trade

// SideVolumes 返回主动卖出和主动买入的股数，沿用原 Status 分类口径。
func (ts Trades) SideVolumes() (sell int64, buy int64) {
	for _, v := range ts {
		switch v.Status {
		case 0: //买
			buy += ToShares(int64(v.Volume))
		case 1: //卖
			sell += ToShares(int64(v.Volume))
		}
	}
	return
}

// Klines 按分笔自带的交易日期与时区合并为沪深 241 根分钟线，保留 09:30 竞价。
func (ts Trades) Klines() Klines {
	//按天分割
	m := make(types.SortMap[int64, Trades])
	for _, v := range ts {
		//获取当天零点的时间戳
		unix := time.Date(v.Time.Year(), v.Time.Month(), v.Time.Day(), 0, 0, 0, 0, v.Time.Location()).Unix()
		m[unix] = append(m[unix], v)
	}

	//按天排序
	mKline := types.SortMap[int64, Klines]{}
	for date, v := range m {
		//生成一分钟k线
		t := time.Unix(date, 0).In(v[0].Time.Location())
		mKline[date] = v.klinesForDay(t)
	}
	//按时间排序
	lss := mKline.Sort()
	ls := Klines{}
	for _, v := range lss {
		ls = append(ls, v...)
	}
	return ls
}

// Kline 合并分时成交为一根 K 线；last 与分笔价格同单位（元/指数点），成交额为元。
func (ts Trades) Kline(t time.Time, last float64) *Kline {
	k := &Kline{
		Time:  t,
		Last:  last,
		Open:  last,
		High:  last,
		Low:   last,
		Close: last,
	}
	first := 0
	for _, v := range ts {
		if v.Price <= 0 {
			continue
		}
		switch first {
		case 0:
			k.Open = v.Price
			k.High = v.Price
			k.Low = v.Price
			k.Close = v.Price
		default:
			k.High = conv.Select(k.High < v.Price, v.Price, k.High)
			k.Low = conv.Select(k.Low > v.Price, v.Price, k.Low)
		}
		k.Close = v.Price
		k.Volume += int64(v.Volume)
		k.Order += v.Number
		k.Amount += v.Amount()
		first++
	}
	return k
}

// kline1 生成一分钟k线,一天
func (ts Trades) klinesForDay(date time.Time) Klines {
	_929 := 569  //9:29 的分钟
	_1130 := 690 //11:30 的分钟
	_1300 := 780 //13:00 的分钟
	_1500 := 900 //15:00 的分钟
	m := map[int]Trades{}
	//获取开盘价,有可能前几分钟没有数据,先遍历一遍
	var open float64
	for _, v := range ts {
		if v.Price > 0 {
			open = v.Price
			break
		}
	}
	//分组,按
	for _, v := range ts {
		ms := minutes(v.Time)
		t := conv.Select(ms < _929, _929, ms)
		t++
		t = conv.Select(t > _1130 && t <= _1300, _1130, t)
		t = conv.Select(t > _1500, _1500, t)
		m[t] = append(m[t], v)
	}
	//合并
	ls := make(Klines, 0, len(minutes241))
	for _, v := range minutes241 {
		k := m[v].Kline(time.Date(date.Year(), date.Month(), date.Day(), v/60, v%60, 0, 0, date.Location()), open)
		open = k.Close
		ls = append(ls, k)
	}
	return ls
}

/*



 */

type TradeCache struct {
	Date string //日期
	Code string //计算倍数
}
