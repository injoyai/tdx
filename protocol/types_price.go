package protocol

import (
	"fmt"
)

type PriceLevel struct {
	Buy    bool    //买卖
	Price  float64 // 档位报价，元（指数为点）
	Number int     //量 想买卖的数量
}

type PriceLevels [5]PriceLevel

func (ps PriceLevels) String() string {
	s := ""
	if ps[0].Buy {
		for i, v := range ps {
			s += fmt.Sprintf("买%d  %g元  %s\n", i+1, v.Price, IntUnitString(v.Number))
		}

	} else {
		for i := 4; i >= 0; i-- {
			s += fmt.Sprintf("卖%d  %g元  %s\n", i+1, ps[i].Price, IntUnitString(ps[i].Number))
		}
	}
	return s
}

// GetPrice 将变长整数厘解码为元；无价格尺度的整数使用 CutInt64。
func GetPrice(bs []byte) ([]byte, float64) {
	bs, v := CutInt64(bs)
	return bs, float64(v) / 1000
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
