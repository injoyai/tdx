package protocol

import (
	"errors"
	"fmt"
	"github.com/injoyai/base/g"
	"github.com/injoyai/conv"
	"time"
)

type KlineReq struct {
	Exchange Exchange
	Code     string
	Start    uint16
	Count    uint16
}

func (this *KlineReq) Bytes(Type uint8) (g.Bytes, error) {
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
		case TypeKlineMinute, TypeKline5Minute, TypeKlineMinute2, TypeKline15Minute, TypeKline30Minute, TypeKlineHour, TypeKlineDay2:
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

	return resp, nil
}

type KlineCache struct {
	Type uint8  //1分钟,5分钟,日线等
	Kind string //指数,个股等
}
