package protocol

import (
	"fmt"
	"time"
)

// Price 价格，单位厘
type Price int64

func (p Price) Float64() float64 {
	return float64(p) / 1000
}

func (p Price) Int64() int64 {
	return int64(p)
}

func (p Price) String() string {
	return fmt.Sprintf("%s元", FloatUnitString(p.Float64()))
}

func (p Price) Abs() Price {
	if p < 0 {
		return -p
	}
	return p
}

func Yuan(f float64) Price {
	return Price(f * 1000)
}

type PriceLevel struct {
	Buy    bool  //买卖
	Price  Price //价 想买卖的价格
	Number int   //量 想买卖的数量
}

type PriceLevels [5]PriceLevel

func (ps PriceLevels) String() string {
	s := ""
	if ps[0].Buy {
		for i, v := range ps {
			s += fmt.Sprintf("买%d  %s  %s\n", i+1, v.Price, IntUnitString(v.Number))
		}

	} else {
		for i := 4; i >= 0; i-- {
			s += fmt.Sprintf("卖%d  %s  %s\n", i+1, ps[i].Price, IntUnitString(ps[i].Number))
		}
	}
	return s
}

// DecodeKline 解码旧格式的五个变长价格字段（收盘及相对收盘的昨收/开/高/低），
// 将分乘 10 转为厘；时间取调用时刻，不包含日期、成交量或金额，也不报告截断错误。
//
// Deprecated: 标准 K 线响应请使用 MKline.Decode；两者字段布局不同，不能互换。
func DecodeKline(bs []byte) ([]byte, *Kline) {
	k := &Kline{
		Time: time.Now(),
	}

	//当日收盘价，一般2字节
	bs, k.Close = GetPrice(bs)

	//前日收盘价，一般1字节
	bs, k.Last = GetPrice(bs)
	k.Last += k.Close

	//当日开盘价，一般1字节
	bs, k.Open = GetPrice(bs)
	k.Open += k.Close

	//当日最高价，一般1字节
	bs, k.High = GetPrice(bs)
	k.High += k.Close

	//当日最低价，一般1字节
	bs, k.Low = GetPrice(bs)
	k.Low += k.Close

	//默认按股票展示
	k.Last *= 10
	k.Open *= 10
	k.Close *= 10
	k.High *= 10
	k.Low *= 10

	return bs, k
}

func GetPrice(bs []byte) ([]byte, Price) {
	bs, v := CutInt64(bs)
	return bs, Price(v)
}

func CutInt(bs []byte) ([]byte, int) {
	bs, v := CutInt64(bs)
	return bs, int(v)
}

func CutInt64(bs []byte) ([]byte, int64) {
	for i := range bs {
		if bs[i]&0x80 == 0 {
			return bs[i+1:], int64(getData(bs[:i+1]))
		}
	}
	return bs, 0
}

/*
字节的第一位表示后续是否有数据（字节）
第一字节 的第二位表示正负 1负0正 有效数据为后6位
后续字节 的有效数据为后7位
最大长度未知
0x20说明有后续数据
*/
func getData(bs []byte) (data int32) {

	for i := range bs {
		switch i {
		case 0:
			//取后6位
			data += int32(bs[0] & 0x3F)

		default:
			//取后7位
			data += int32(bs[i]&0x7F) << uint8(6+(i-1)*7)

		}

		//判断是否有后续数据
		if bs[i]&0x80 == 0 {
			break
		}
	}

	//第一字节的第二位为1表示为负数
	if len(bs) > 0 && bs[0]&0x40 > 0 {
		data = -data
	}

	return
}

///*
//字节的第一位表示后续是否有数据（字节）
//第一字节 的第二位表示正负 1负0正 有效数据为后6位
//后续字节 的有效数据为后7位
//最大长度未知
//0x20说明有后续数据
//*/
//func getPrice(bs []byte) (data Price) {
//
//	for i := range bs {
//		switch i {
//		case 0:
//			//取后6位
//			data += Price(int32(bs[0] & 0x3F))
//
//		default:
//			//取后7位
//			data += Price(int32(bs[i]&0x7F) << uint8(6+(i-1)*7))
//
//		}
//
//		//判断是否有后续数据
//		if bs[i]&0x80 == 0 {
//			break
//		}
//	}
//
//	//第一字节的第二位为1表示为负数
//	if len(bs) > 0 && bs[0]&0x40 > 0 {
//		data = -data
//	}
//
//	return
//}

//func getData(bs []byte) (data int) {
//
//	for i := range bs {
//		switch i {
//		case 0:
//			//取后6位
//			data += int(bs[0] & 0x3F)
//
//		default:
//			//取后7位
//			data += int(bs[i]&0x7F) << uint8(6+(i-1)*7)
//
//		}
//
//		//判断是否有后续数据
//		if bs[i]&0x80 == 0 {
//			break
//		}
//	}
//
//	//第一字节的第二位为1表示为负数
//	if len(bs) > 0 && bs[0]&0x40 > 0 {
//		data = -data
//	}
//
//	return
//}
