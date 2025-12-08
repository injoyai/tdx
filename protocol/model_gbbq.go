package protocol

import (
	"encoding/binary"
	"errors"
	"math"
	"time"
)

/*
根据官网的名称来,gb应该是股本,bq不知道啥意思

XDXR_CATEGORY_MAPPING = {
    1 : "除权除息",
    2 : "送配股上市",
    3 : "非流通股上市",
    4 : "未知股本变动",
    5 : "股本变化",
    6 : "增发新股",
    7 : "股份回购",
    8 : "增发新股上市",
    9 : "转配股上市",
    10 : "可转债上市",
    11 : "扩缩股",
    12 : "非流通股缩股",
    13 : "送认购权证",
    14 : "送认沽权证"
}


*/

type gbbq struct{}

func (gbbq) Frame(code string) (*Frame, error) {
	exchange, number, err := DecodeCode(code)
	if err != nil {
		return nil, err
	}

	data := []byte{0x01, 0x00}
	data = append(data, exchange.Uint8())
	data = append(data, number...)
	return &Frame{
		Control: Control01,
		Type:    TypeGbbq,
		Data:    data,
	}, nil
}

func (gbbq) Decode(bs []byte) (*GbbqResp, error) {

	if len(bs) < 11 {
		return nil, errors.New("数据长度不足")
	}

	_count := Uint16(bs[9:11])
	resp := &GbbqResp{
		Count: _count,
		List:  make([]*Gbbq, 0, _count),
	}
	bs = bs[11:]

	for i := uint16(0); i < _count; i++ {
		g := &Gbbq{
			Exchange: Exchange(bs[0]),
			Code:     string(bs[1:7]),
			Time:     GetTime([4]byte(bs[8:12]), 100),
			Category: int(bs[12]),
		}
		bs = bs[13:]
		switch g.Category {
		case 1:
			//fenhong, peigujia, songzhuangu, peigu  = struct.unpack("<ffff", body_buf[pos: pos + 16])
			g.C1 = float64(math.Float32frombits(binary.LittleEndian.Uint32(bs[0:4])))
			g.C2 = float64(math.Float32frombits(binary.LittleEndian.Uint32(bs[4:8])))
			g.C3 = float64(math.Float32frombits(binary.LittleEndian.Uint32(bs[8:12])))
			g.C4 = float64(math.Float32frombits(binary.LittleEndian.Uint32(bs[12:16])))

		case 11, 12:
			// (_, _, suogu, _) = struct.unpack("<IIfI", body_buf[pos: pos + 16])
			g.C3 = float64(math.Float32frombits(binary.LittleEndian.Uint32(bs[8:12])))

		case 13, 14:
			//  xingquanjia, _, fenshu, _ = struct.unpack("<fIfI", body_buf[pos: pos + 16])
			g.C1 = float64(math.Float32frombits(binary.LittleEndian.Uint32(bs[0:4])))
			g.C3 = float64(math.Float32frombits(binary.LittleEndian.Uint32(bs[8:12])))

		default:
			//panqianliutong_raw, qianzongguben_raw, panhouliutong_raw, houzongguben_raw = struct.unpack("<IIII", body_buf[pos: pos + 16])
			//panqianliutong = _get_v(panqianliutong_raw)
			//panhouliutong = _get_v(panhouliutong_raw)
			//qianzongguben = _get_v(qianzongguben_raw)
			//houzongguben = _get_v(houzongguben_raw)
			g.C1 = getVolume(Uint32(bs[0:4]))
			g.C2 = getVolume(Uint32(bs[4:8]))
			g.C3 = getVolume(Uint32(bs[8:12]))
			g.C4 = getVolume(Uint32(bs[12:16]))

		}
		bs = bs[16:]
		resp.List = append(resp.List, g)
	}

	return resp, nil
}

type GbbqResp struct {
	Count uint16
	List  []*Gbbq
}

type Gbbq struct {
	Exchange Exchange
	Code     string
	Time     time.Time
	Category int //2, 3, 5, 7, 8, 9, 10
	C1       float64
	C2       float64
	C3       float64
	C4       float64
}

func (this *Gbbq) FullCode() string {
	return this.Exchange.String() + this.Code
}
