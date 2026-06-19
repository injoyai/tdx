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
	if _count == 0 {
		return &GbbqResp{}, nil
	}
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
		Float:    int64(this.C3),
		Total:    int64(this.C4),
	}
}

func (this *Gbbq) XRXD() *XRXD {
	base := 100. //保留2位小数
	return &XRXD{
		Code:        this.Code,
		Time:        this.Time,
		Fenhong:     math.Round(this.C1*base) / base,
		Peigujia:    math.Round(this.C2*base) / base,
		Songzhuangu: math.Round(this.C3*base) / base,
		Peigu:       math.Round(this.C4*base) / base,
	}
}

type Equity struct {
	Category int       //2, 3, 5, 7, 8, 9, 10
	Code     string    //例sh600000
	Time     time.Time //时间
	Float    int64     //流通股本,单位股
	Total    int64     //总股本,单位股
}

// Turnover 换手率,传入股,通达信获取的一般是手,注意
func (this *Equity) Turnover(volume int64) float64 {
	return (float64(volume) / float64(this.Float)) * 100
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

// mc 单个除权除息事件(category=1)的仿射系数:
//
//	m = (10 + 送转 + 配股) / 10   乘法因子(送股/转增/配股)
//	c = (分红 − 配股*配股价) / 10  每股净现金流出(分红减配股注入)
//
// 标准除权参考价(减法口径)与 XRXD.Pre 等价: P_adj = (P − c) / m。
func (this *XRXD) mc() (m, c float64) {
	m = (10 + this.Songzhuangu + this.Peigu) / 10
	c = (this.Fenhong - this.Peigu*this.Peigujia) / 10
	if m == 0 {
		m = 1
	}
	return
}

type XRXDs []*XRXD

// Pre ks需要按时间从小到大
//
// 复权模型为【仿射变换】 price_adj = A*price_raw + B (不是纯比例缩放):
//   - A 由送股/转增/配股累积, B 由现金分红累积并随后续送配缩放。
//   - 纯比例口径(旧实现 PreLast/Last 逐日累乘)在有现金分红时会系统性偏离通达信,
//     因此这里改为仿射, 与通达信桌面端逐分对齐。
//
// 关键: 事件按【日期位置】施加, 而不是按"ex-day 是否命中交易日"。
// 当 ex-day 落在停牌区间(股改/大比例转增连续停牌)时, 该日不在 ks 里,
// 旧实现会整体跳过该事件(或只施加一个), 污染该 ex-day 及之前所有历史的复权因子。
// 正确做法: 自今向过去遍历交易日, 把所有 exday > 当前交易日 且尚未施加的事件【依次复合】,
// 同一停牌缺口内的多个事件自动合成 m=m1*m2、加法项链式缩放。
//
// PreLast 仍按 XRXD.Pre 填充(向后兼容), 但复权因子改由 Factors() 用仿射系数给出。
func (this XRXDs) Pre(ks []*Kline) PreKlines {
	if len(ks) == 0 {
		return PreKlines{}
	}

	//排序
	sort.Slice(this, func(i, j int) bool {
		return this[i].Time.Before(this[j].Time)
	})
	sort.Slice(ks, func(i, j int) bool {
		return ks[i].Time.Before(ks[j].Time)
	})

	ls := make(PreKlines, len(ks))
	for i, k := range ks {
		// 把全部 ex-day 事件挂到每个 PreKline 上(共享同一切片),
		// 供 Factors() 按"日期位置"复合; 不依赖 ex-day 是否命中交易日, 故停牌缺口内的
		// 多个事件都不会丢失。
		ls[i] = &PreKline{Kline: k, PreLast: k.Last, events: this}
	}

	// 把每个 ex-day 落到"第一个 >= ex-day 的交易日"上, 用于填充 PreLast(向后兼容展示用)。
	for _, x := range this {
		for _, k := range ls {
			if !k.Time.Before(x.Time) { // k.Time >= x.Time
				k.PreLast = x.Pre(k.Last)
				break
			}
		}
	}

	return ls
}

type PreKline struct {
	*Kline
	PreLast Price
	events  XRXDs // 该股全部除权除息事件(按时间升序), 各 PreKline 共享同一引用
}

func (this *PreKline) QFQFactor() float64 {
	if this.Last == this.PreLast || this.Last == 0 || this.PreLast == 0 {
		return 1
	}
	return this.PreLast.Float64() / this.Last.Float64()
}

func (this *PreKline) HFQFactor() float64 {
	if this.Last == this.PreLast || this.Last == 0 || this.PreLast == 0 {
		return 1
	}
	return this.Last.Float64() / this.PreLast.Float64()
}

type PreKlines []*PreKline

// Factors 计算每个交易日的仿射复权系数(前复权 QFQMul/QFQAdd, 后复权 HFQMul/HFQAdd)。
//
// QFQ(前复权): 锚定最新交易日 A=1,B=0, 自今向过去回溯, 遇 ex-day:
//
//	A <- A / m ; B <- B - A_new * c
//
// HFQ(后复权): 锚定最早交易日 (A0,B0)=最早日的 QFQ 系数, hfq = (qfq - B0) / A0,
// 即对任意 raw 价: hfq_price = (A/A0)*raw + (B-B0)/A0。
//
// 复权价 = round_half_up(QFQMul*raw + QFQAdd) , 由 Factor.QFQPrice / HFQPrice 完成。
//
// 兼容字段 QFQ/HFQ(纯比例因子)仍按"当日仿射相对最新/最早日的比例"近似填充,
// 仅在纯送转(无分红)时与仿射一致; 有分红时请改用 QFQPrice/HFQPrice。
//
// ───────── 已知无法对齐的特例(诚实记录) ─────────
// 600519 茅台 2006-05-26 开盘: 本算法 qfq=-260.97, 通达信桌面端=-260.98(差 0.005)。
// 该日 raw 开盘两端均=39.49; 用 50 位高精度重算 B 仍差 0.00505, 是真实差异而非浮点漂移;
// fenhong 量化偏差均 <1e-4。任何"逐 ex-day 把因子 round 到 N 位"的模型虽能凑中此点,
// 却会打坏已核对正确的 2006-04-25 收盘(-260.48 → -260.42)。
// 故这是通达信桌面端对该股改停牌簇边界日的【孤立内部处理差异】, 统一规则无法复现,
// 仅记录、不做硬编码补丁。其余样本(含 000651 股改、002626 多次送转)均逐分对齐。
func (this PreKlines) Factors() []*Factor {
	if len(this) == 0 {
		return []*Factor(nil)
	}

	sort.Slice(this, func(i, j int) bool { return this[i].Time.Before(this[j].Time) })

	// 收集全部 ex-day 事件。来源为挂在 PreKline 上的完整事件切片(各 PreKline 共享),
	// 不依赖 ex-day 是否命中交易日 —— 这样停牌缺口内的多个事件都会被复合, 不丢失。
	//
	// 关键: 必须丢弃 ex-day 严格晚于【最新交易日】的事件。gbbq 里常含【已公告但尚未除权】
	// 的分红记录(ex-day 在未来), 这类事件还没生效。前复权锚定最新交易日 A=1,B=0,
	// 若把未来分红也施加进去, 会让最新日 B != 0, 整条历史被平移一个未发生的分红额。
	// 实例: 600887 gbbq 含 20260605 派9元(今日之后), 旧逻辑使全历史前复权偏低 0.90 元,
	// 与通达信/同花顺(均在除权日才生效)不符。
	type ev struct {
		t    time.Time
		m, c float64
	}
	var src XRXDs
	if len(this) > 0 {
		src = this[0].events
	}
	latest := this[len(this)-1].Time // 最新交易日
	evs := make([]ev, 0, len(src))
	for _, x := range src {
		if x.Time.After(latest) {
			continue // 未来 ex-day(已公告未生效), 不施加
		}
		m, c := x.mc()
		evs = append(evs, ev{t: x.Time, m: m, c: c})
	}
	sort.Slice(evs, func(i, j int) bool { return evs[i].t.Before(evs[j].t) }) //按 ex-day 升序

	ls := make([]*Factor, len(this))

	// QFQ: 自今向过去, 写入每个交易日系数前, 复合所有 exday 严格晚于该交易日的未施加事件。
	a, b := 1.0, 0.0
	ei := len(evs) - 1
	for i := len(this) - 1; i >= 0; i-- {
		k := this[i]
		for ei >= 0 && evs[ei].t.After(k.Time) { // ex-day 严格晚于当前交易日
			m, c := evs[ei].m, evs[ei].c
			a = a / m
			b = b - a*c
			ei--
		}
		ls[i] = &Factor{
			Time:    k.Time,
			Last:    k.Last,
			PreLast: k.PreLast,
			QFQMul:  a,
			QFQAdd:  b,
		}
	}

	// HFQ: 由最早交易日的 QFQ 系数 (a0,b0) 锚定。 hfq = (a/a0)*raw + (b-b0)/a0。
	a0, b0 := ls[0].QFQMul, ls[0].QFQAdd
	for _, f := range ls {
		if a0 != 0 {
			f.HFQMul = f.QFQMul / a0
			f.HFQAdd = (f.QFQAdd - b0) / a0
		} else {
			f.HFQMul, f.HFQAdd = 1, 0
		}
		// 兼容字段: 纯比例因子(仅纯送转无分红时等于仿射, 有分红时为近似)。
		f.QFQ = f.QFQMul
		f.HFQ = f.HFQMul
	}

	return ls
}

type Factor struct {
	Time    time.Time //日期
	Last    Price     //昨收价
	PreLast Price     //除权除息后昨收价

	// 仿射复权系数: price_adj = round_half_up(Mul*price_raw + Add)
	QFQMul float64 //前复权乘法因子 A
	QFQAdd float64 //前复权加法偏移 B(单位:元)
	HFQMul float64 //后复权乘法因子
	HFQAdd float64 //后复权加法偏移(单位:元)

	// 兼容旧字段: 纯比例因子(仅纯送转无分红时与仿射一致, 有现金分红时为近似, 勿用于精确复权)
	QFQ float64 //前复权因子(比例, 已弃用, 请用 QFQPrice)
	HFQ float64 //后复权因子(比例, 已弃用, 请用 HFQPrice)
}

// QFQPrice 前复权价格(对不复权原始价 raw 做仿射 + 四舍五入到 2 位小数)。
func (this *Factor) QFQPrice(raw Price) Price {
	return roundHalfUpYuan(this.QFQMul*raw.Float64() + this.QFQAdd)
}

// HFQPrice 后复权价格(对不复权原始价 raw 做仿射 + 四舍五入到 2 位小数)。
func (this *Factor) HFQPrice(raw Price) Price {
	return roundHalfUpYuan(this.HFQMul*raw.Float64() + this.HFQAdd)
}

// roundHalfUpYuan 把"元"值四舍五入(逢五进一)到 2 位小数后转为 Price(厘)。
// 通达信用四舍五入, 非银行家舍入; math.Round 对 .5 远离零取整, 正好是逢五进一。
func roundHalfUpYuan(yuan float64) Price {
	cents := math.Round(yuan * 100)
	return Price(cents * 10) // 1 分 = 10 厘
}

// ApplyQFQ 用前复权因子 fs 按日期对齐, 把不复权日线 ks 整段转为前复权日线。
// OHLC 与昨收均复权(四舍五入到分, 对齐通达信桌面端), 成交量/额不变。
// fs 由 (XRXDs).Pre(ks).Factors() 或 Gbbq.GetFactors 生成。返回新切片, 不改原 ks。
func ApplyQFQ(ks Klines, fs []*Factor) Klines { return applyFQ(ks, fs, true) }

// ApplyHFQ 用后复权因子把不复权日线整段转为后复权日线。见 ApplyQFQ。
func ApplyHFQ(ks Klines, fs []*Factor) Klines { return applyFQ(ks, fs, false) }

func applyFQ(ks Klines, fs []*Factor, qfq bool) Klines {
	fm := make(map[int64]*Factor, len(fs))
	for _, f := range fs {
		fm[f.Time.Unix()] = f
	}
	out := make(Klines, len(ks))
	for i, k := range ks {
		nk := *k
		if f := fm[k.Time.Unix()]; f != nil {
			price := f.QFQPrice
			if !qfq {
				price = f.HFQPrice
			}
			nk.Last = price(k.Last)
			nk.Open = price(k.Open)
			nk.High = price(k.High)
			nk.Low = price(k.Low)
			nk.Close = price(k.Close)
		}
		out[i] = &nk
	}
	return out
}
