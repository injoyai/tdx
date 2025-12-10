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
			//Exchange: Exchange(bs[0]),
			Code:     Exchange(bs[0]).String() + string(bs[1:7]),
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
			g.C1 = getVolume(Uint32(bs[0:4])) * 1e4
			g.C2 = getVolume(Uint32(bs[4:8])) * 1e4
			g.C3 = getVolume(Uint32(bs[8:12])) * 1e4
			g.C4 = getVolume(Uint32(bs[12:16])) * 1e4

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
	Code     string
	Time     time.Time //15:00,注意判断逻辑
	Category int       //2, 3, 5, 7, 8, 9, 10
	C1       float64
	C2       float64
	C3       float64
	C4       float64
}

type Gbbqs map[string][]*Gbbq

func (this Gbbqs) GetEquities() map[string][]*Equity {
	m := map[string][]*Equity{}
	for k, v := range this {
		for _, vv := range v {
			switch vv.Category {
			case 2, 3, 5, 7, 8, 9, 10:
				m[k] = append(m[k], &Equity{
					Category: vv.Category,
					Code:     vv.Code,
					Time:     vv.Time,
					Float:    vv.C3,
					Total:    vv.C4,
				})
			}
		}

	}
	return m
}

func (this Gbbqs) GetXRXDs() map[string][]*XRXD {
	m := map[string][]*XRXD{}
	for k, v := range this {
		for _, vv := range v {
			switch vv.Category {
			case 1:
				m[k] = append(m[k], &XRXD{
					Code:        vv.Code,
					Time:        vv.Time,
					Fenhong:     vv.C1,
					Peigujia:    vv.C2,
					Songzhuangu: vv.C3,
					Peigu:       vv.C4,
				})
			}
		}
	}
	return m
}

type Equity struct {
	Category int       //2, 3, 5, 7, 8, 9, 10
	Code     string    //例sh600000
	Time     time.Time //时间
	Float    float64   //流通股本,单位股
	Total    float64   //总股本,单位股
}

func (this *Equity) TableName() string {
	return "equity"
}

// Turnover 换手率,传入股
func (this *Equity) Turnover(volume int64) float64 {
	return (float64(volume) / this.Float) * 100
}

/*
XRXD
除权 ex-rights
除息 ex-dividend
*/
type XRXD struct {
	Code        string    //例sh600000
	Time        time.Time //时间
	Fenhong     float64   //分红
	Peigujia    float64   //配股价
	Songzhuangu float64   //送转股
	Peigu       float64   //配股
}

func (this *XRXD) FQ(p Price) Price {
	numerator := (p.Float64()*10 - this.Fenhong) + (this.Peigu * this.Peigujia)
	denominator := 10 + this.Peigu + this.Songzhuangu
	if denominator == 0 {
		return p
	}
	return Price((numerator / denominator) * 1000)
}
