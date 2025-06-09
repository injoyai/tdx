package protocol

import (
	"errors"
	"fmt"
	"github.com/injoyai/conv"
	"time"
)

type TradeResp struct {
	Count uint16
	List  Trades
}

// Trade 分时成交，todo 时间没有到秒，客户端上也没有,东方客户端能显示秒
type Trade struct {
	Time   time.Time //时间, 09:30
	Price  Price     //价格
	Volume int       //成交量,手
	Status int       //0是买，1是卖，2中性/汇总 中途也可能出现2,例20241115(sz000001)的14:56
	Number int       //单数,历史数据该字段无效
}

func (this *Trade) String() string {
	return fmt.Sprintf("%s \t%-6s \t%-6s \t%-6d(手) \t%-4d(单) \t%-4s",
		this.Time, this.Price, this.Amount(), this.Volume, this.Number, this.StatusString())
}

// Amount 成交额
func (this *Trade) Amount() Price {
	return this.Price * Price(this.Volume*100)
}

func (this *Trade) StatusString() string {
	switch this.Status {
	case 0:
		return "买入"
	case 1:
		return "卖出"
	default:
		return ""
	}
}

// AvgVolume 平均每单成交量
func (this *Trade) AvgVolume() float64 {
	return float64(this.Volume) / float64(this.Number)
}

// AvgPrice 平均每单成交金额
func (this *Trade) AvgPrice() Price {
	return Price(this.AvgVolume() * float64(this.Price) * 100)
}

// IsBuy 是否是买单
func (this *Trade) IsBuy() bool {
	return this.Status == 0
}

// IsSell 是否是卖单
func (this *Trade) IsSell() bool {
	return this.Status == 1
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
		Type:    TypeMinuteTrade,
		Data:    append([]byte{exchange.Uint8(), 0x0}, codeBs...),
	}, nil
}

func (trade) Decode(bs []byte, c TradeCache) (*TradeResp, error) {

	_, code, err := DecodeCode(c.Code)
	if err != nil {
		return nil, err
	}

	if len(bs) < 2 {
		return nil, errors.New("数据长度不足")
	}

	resp := &TradeResp{
		Count: Uint16(bs[:2]),
	}

	bs = bs[2:]

	lastPrice := Price(0)
	for i := uint16(0); i < resp.Count; i++ {
		timeStr := GetHourMinute([2]byte(bs[:2]))
		t, err := time.Parse("2006010215:04", c.Date+timeStr)
		if err != nil {
			return nil, err
		}
		mt := &Trade{Time: t}
		var sub Price
		bs, sub = GetPrice(bs[2:])
		lastPrice += sub * 10 //把分转换成厘
		mt.Price = lastPrice / basePrice(code)
		bs, mt.Volume = CutInt(bs)
		bs, mt.Number = CutInt(bs)
		bs, mt.Status = CutInt(bs)
		bs, _ = CutInt(bs) //这个得到的是0，不知道是啥
		resp.List = append(resp.List, mt)
	}

	return resp, nil
}

type Trades []*Trade

func (this Trades) Kline() (k *Kline, err error) {
	k = &Kline{}
	for i, v := range this {
		switch i {
		case 0:
			k.Time = v.Time
			k.Open = v.Price
			k.High = v.Price
			k.Low = v.Price
			k.Close = v.Price
		case len(this) - 1:
			k.Close = v.Price
		}
		k.High = conv.Select(v.Price > k.High, v.Price, k.High)
		k.Low = conv.Select(v.Price < k.Low, v.Price, k.Low)
		k.Volume += int64(v.Volume)
		k.Amount += v.Amount()
	}
	return
}

// Klines1 1分K线
func (this Trades) Klines1() (Klines, error) {
	m := make(map[int64]Trades)
	for _, v := range this {
		//小于9点30的数据归类到9点30
		if v.Time.Hour() == 9 && v.Time.Minute() < 30 {
			v.Time = time.Date(v.Time.Year(), v.Time.Month(), v.Time.Day(), 9, 30, 0, 0, v.Time.Location())
		}
		m[v.Time.Unix()] = append(m[v.Time.Unix()], v)
	}

	ls := Klines(nil)
	for _, v := range m {
		k, err := v.Kline()
		if err != nil {
			return nil, err
		}
		ls = append(ls, k)
	}
	ls.Sort()
	return ls, nil
}

type TradeCache struct {
	Date string //日期
	Code string //计算倍数
}
