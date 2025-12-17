package protocol

import (
	"encoding/binary"
	"errors"
	"math"
	"sort"
	"time"
)

/*
根据官网的名称来,gbbq股本变迁

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

func (this *Gbbq) TableName() string {
	return "gbbq"
}

func (this *Gbbq) IsEquity() bool {
	switch this.Category {
	case 2, 3, 5, 7, 8, 9, 10:
		return true
	}
	return false
}

func (this *Gbbq) IsXRXD() bool {
	switch this.Category {
	case 1:
		return true
	}
	return false
}

func (this *Gbbq) Equity() *Equity {
	return &Equity{
		Category: this.Category,
		Code:     this.Code,
		Time:     this.Time,
		Float:    this.C3,
		Total:    this.C4,
	}
}

func (this *Gbbq) XRXD() *XRXD {
	base := 100. //保留2位小数
	return &XRXD{
		Code:        this.Code,
		Time:        this.Time,
		Fenhong:     float64(int64(this.C1*base+0.5)) / base,
		Peigujia:    float64(int64(this.C2*base+0.5)) / base,
		Songzhuangu: float64(int64(this.C3*base+0.5)) / base,
		Peigu:       float64(int64(this.C4*base+0.5)) / base,
	}
}

type Gbbqs map[string][]*Gbbq

func (this Gbbqs) GetEquities() map[string][]*Equity {
	m := map[string][]*Equity{}
	for k, v := range this {
		for _, vv := range v {
			if vv.IsEquity() {
				m[k] = append(m[k], vv.Equity())
			}
		}

	}
	return m
}

func (this Gbbqs) GetXRXDs() map[string][]*XRXD {
	m := map[string][]*XRXD{}
	for k, v := range this {
		for _, vv := range v {
			if vv.IsXRXD() {
				m[k] = append(m[k], vv.XRXD())
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

// Turnover 换手率,传入股,通达信获取的一般是手,注意
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
	Fenhong     float64   //分红,10股分n元
	Peigujia    float64   //配股价
	Songzhuangu float64   //送转股
	Peigu       float64   //配股
}

// Pre 计算除权除息之后的价格,10元,10股分5元->9.5元
func (this *XRXD) Pre(p Price) Price {
	if this == nil {
		return p
	}
	numerator := (p.Float64()*10 - this.Fenhong) + (this.Peigu * this.Peigujia)
	denominator := 10 + this.Songzhuangu + this.Peigu
	if denominator == 0 {
		return p
	}
	return Price((numerator / denominator) * 1000)
}

type XRXDs []*XRXD

func (this XRXDs) Pre(ks []*Kline) PreKlines {
	return this.Pre2(ks)

	m := make(map[string]*XRXD)
	for _, v := range this {
		m[v.Time.Format(time.DateOnly)] = v
	}
	ls := make(PreKlines, len(ks))
	for i, k := range ks {
		x := m[k.Time.Format(time.DateOnly)]
		ls[i] = &PreKline{
			Kline:   k,
			PreLast: x.Pre(k.Last),
		}
	}
	return ls
}

func (this XRXDs) Pre2(ks []*Kline) PreKlines {
	if len(ks) == 0 {
		return PreKlines{}
	}
	//ks[0].Last = ks[0].Open

	m := make(map[string]*Kline)
	for _, v := range ks {
		m[v.Time.Format(time.DateOnly)] = v
	}
	ls := make(PreKlines, len(this))
	last := ks[0]
	for i, v := range this {
		k := m[v.Time.Format(time.DateOnly)]
		if k == nil {
			k = last
		}
		ls[i] = &PreKline{
			Kline:   k,
			PreLast: v.Pre(k.Last),
		}
		last = k
	}
	return ls
}

type PreKline struct {
	*Kline
	PreLast Price
}

func (this *PreKline) QFQFactor() float64 {
	if this.Last == this.PreLast || this.Last == 0 || this.PreLast == 0 {
		return 1
	}
	return this.PreLast.Float64() / this.Last.Float64()
}

func (this *PreKline) QFQ() *Kline {
	f := this.QFQFactor()
	return this.FQ(f)
}

func (this *PreKline) HFQFactor() float64 {
	if this.Last == this.PreLast || this.Last == 0 || this.PreLast == 0 {
		return 1
	}
	return this.Last.Float64() / this.PreLast.Float64()
}

func (this *PreKline) HFQ() *Kline {
	f := this.HFQFactor()
	return this.FQ(f)
}

func (this *PreKline) FQ(f float64) *Kline {
	base := Price(100)
	return &Kline{
		Last:      (this.Last * Price(f*float64(base))) / base,
		Open:      (this.Open * Price(f*float64(base))) / base,
		High:      (this.High * Price(f*float64(base))) / base,
		Low:       (this.Low * Price(f*float64(base))) / base,
		Close:     (this.Close * Price(f*float64(base))) / base,
		Order:     this.Order,
		Volume:    this.Volume,
		Amount:    this.Amount,
		Time:      this.Time,
		UpCount:   this.UpCount,
		DownCount: this.DownCount,
	}
}

type PreKlines []*PreKline

func (this PreKlines) FactorMap() map[string]*Factor {
	m := make(map[string]*Factor)
	for _, v := range this.Factor() {
		m[v.Time.Format(time.DateOnly)] = v
	}
	return m
}

func (this PreKlines) Factor() []*Factor {
	ls := make([]*Factor, len(this))
	sort.Slice(this, func(i, j int) bool {
		return this[i].Time.Before(this[i].Time)
	})
	lastHFQ := 1.0
	for i, v := range this {
		lastHFQ *= v.HFQFactor()
		ls[i] = &Factor{
			Time:    v.Time,
			Last:    v.Last,
			PreLast: v.PreLast,
			HFQ:     lastHFQ,
		}
	}

	sort.Slice(this, func(i, j int) bool {
		return this[i].Time.After(this[i].Time)
	})

	lastQFQ := 1.0
	for i := len(this) - 1; i >= 0; i-- {
		v := this[i]
		lastQFQ *= v.QFQFactor()
		ls[i].QFQ = lastQFQ
	}

	return ls
}

//func (this PreKlines) Factor(startTime time.Time) []*Factor {
//	ls := make([]*Factor, 0, len(this))
//	sort.Slice(this, func(i, j int) bool {
//		return this[i].Time.Before(this[i].Time)
//	})
//	lastHFQ := 1.0
//	for _, v := range this {
//		if v.Time.Before(startTime) {
//			//continue
//		}
//		lastHFQ *= v.HFQFactor()
//		ls = append(ls, &Factor{
//			Time:    v.Time,
//			Last:    v.Last,
//			PreLast: v.PreLast,
//			HFQ:     lastHFQ,
//		})
//	}
//
//	sort.Slice(this, func(i, j int) bool {
//		return this[i].Time.After(this[i].Time)
//	})
//
//	lastQFQ := 1.0
//	for i := len(this) - 1; i >= 0; i-- {
//		v := this[i]
//		if v.Time.Before(startTime) {
//			//continue
//		}
//		lastHFQ *= v.QFQFactor()
//		ls[i].QFQ = lastQFQ
//	}
//
//	return ls
//}

type Factor struct {
	Time    time.Time
	Last    Price
	PreLast Price
	QFQ     float64
	HFQ     float64
}
