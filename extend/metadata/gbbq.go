package metadata

import (
	"fmt"
	"sort"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

type Gbbq struct {
	m map[string][]*protocol.Gbbq
}

// NewGbbq validates and copies events. The resulting snapshot has no background tasks.
func NewGbbq(records []*model.Gbbq) (*Gbbq, error) {
	g := &Gbbq{m: map[string][]*protocol.Gbbq{}}
	for _, row := range records {
		if row == nil || row.Time.IsZero() {
			return nil, fmt.Errorf("gbbq: invalid event")
		}
		key, err := model.StandardCode(row.Code)
		if err != nil {
			return nil, err
		}
		copy := *row
		copy.Code = key
		g.m[key] = append(g.m[key], &copy)
	}
	for _, rows := range g.m {
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].Time.Before(rows[j].Time) })
	}
	return g, nil
}

func (g *Gbbq) All() map[string][]*protocol.Gbbq {
	m := make(map[string][]*protocol.Gbbq)
	for k, v := range g.m {
		for _, row := range v {
			copy := *row
			m[k] = append(m[k], &copy)
		}
	}
	return m
}

func (g *Gbbq) records(code string) []*protocol.Gbbq {
	key, err := model.StandardCode(code)
	if err != nil {
		return nil
	}
	return g.m[key]
}

func (g *Gbbq) GetEquity(code string, t time.Time) *protocol.Equity {
	records := g.records(code)
	for i := len(records) - 1; i >= 0; i-- {
		v := records[i]
		if !v.IsEquity() {
			continue
		}
		y, m, d := v.Time.Date()
		// 记录中的 15:00 不影响股本在所属时区当日零点生效。
		day := time.Date(y, m, d, 0, 0, 0, 0, v.Time.Location())
		if t.Unix() >= day.Unix() {
			return v.Equity()
		}
	}
	return nil
}

func (g *Gbbq) GetXRXDs(code string) protocol.XRXDs {
	res := protocol.XRXDs{}
	for _, v := range g.records(code) {
		if v.IsXRXD() {
			res = append(res, v.XRXD())
		}
	}
	return res
}

func (g *Gbbq) GetXRXDMap(code string) map[string]*protocol.XRXD {
	ls := g.records(code)
	res := map[string]*protocol.XRXD{}
	for _, v := range ls {
		if v.IsXRXD() {
			res[v.Time.Format(time.DateOnly)] = v.XRXD()
		}
	}
	return res
}

func (g *Gbbq) GetFactors(code string, ks protocol.Klines) []*protocol.Factor {
	return g.GetXRXDs(code).Pre(ks).Factors()
}

// QFQ 把已获取的不复权日线 ks 转为前复权日线(对齐通达信桌面端, 四舍五入到分)。
// 输入行情与事件均由调用方提供。
func (g *Gbbq) QFQ(code string, ks protocol.Klines) protocol.Klines {
	return protocol.ApplyQFQ(ks, g.GetFactors(code, ks))
}

// HFQ 把已获取的不复权日线转为后复权日线。见 QFQ。
func (g *Gbbq) HFQ(code string, ks protocol.Klines) protocol.Klines {
	return protocol.ApplyHFQ(ks, g.GetFactors(code, ks))
}

// Turnover distinguishes unavailable equity from an actual zero turnover.
func (g *Gbbq) Turnover(code string, at time.Time, volume int64) (float64, bool) {
	equity := g.GetEquity(code, at)
	if equity == nil || equity.Float <= 0 || volume < 0 {
		return 0, false
	}
	return equity.Turnover(volume), true
}
