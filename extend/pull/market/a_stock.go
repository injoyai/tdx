// Package market 实现各市场的拉取 Unit（可插拔）。
// 标准行情(7709)：沪深股票/ETF/LOF/指数/板块；扩展行情(7727)：期货/港股/美股。
package market

import (
	"fmt"
	"sync"
	"time"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/extend/pull"
	"github.com/injoyai/tdx/protocol"
)

// stdUnit 标准行情(7709)市场的基础实现：通过 tdx.Manage 连接源取连接。
// 负责原始行情适配与增量边界，分页结果通过 Service 存储。
type stdUnit struct {
	name pull.Market // 市场标识（pull.Market 枚举）
	kind string      // 分类：stock / etf / index（决定用 GetKline 还是 GetIndex）
}

func (u *stdUnit) Name() string { return u.name.String() }

// Codes 获取代码列表（由具体市场决定来源）。
func (u *stdUnit) Codes(s *pull.Service) ([]pull.Code, error) {
	return nil, fmt.Errorf("market: %s 未实现 Codes", u.name)
}

// gbbqMu 保护下述懒加载（多个 stdUnit / goroutine 并发触发时只初始化一次）。
var gbbqMu sync.Mutex

// Manage 只获取标准行情连接源；不因查询代码、指数或分钟线触发股本初始化。
func (u *stdUnit) Manage(s *pull.Service) (*tdx.Manage, error) {
	m := s.Manage()
	if m == nil {
		return nil, fmt.Errorf("pull: 市场 %s 需要配置 Manage（标准行情7709连接源）", u.name)
	}
	return m, nil
}

// stockEquity 仅股票日线使用；在同一锁内读取与替换默认空实现。
func stockEquity(m *tdx.Manage) (tdx.IGbbq, error) {
	gbbqMu.Lock()
	defer gbbqMu.Unlock()
	if g, ok := m.Gbbq.(*tdx.Gbbq); ok && g.IsEmpty() {
		g, err := tdx.NewGbbq()
		if err != nil {
			return nil, fmt.Errorf("pull: 初始化股本变迁数据失败: %w", err)
		}
		m.Gbbq = g
	}
	return m.Gbbq, nil
}

// fetchDayAll 一次性拉取全部日线（从 Start 起，直到没有更早数据）。
func (u *stdUnit) fetchDayAll(c *tdx.Client, code string, startAt time.Time) (protocol.Klines, error) {
	return u.dayUntil(c, code, func(k *protocol.Kline) bool {
		if !startAt.IsZero() && k.Time.Before(startAt) {
			return true
		}
		return false
	})
}

// fetchDayFrom 拉取日线，直到遇到 last 之前的日期（增量）。
func (u *stdUnit) fetchDayFrom(c *tdx.Client, code string, last int64, startAt time.Time) (protocol.Klines, error) {
	stop := time.Unix(last, 0)
	return u.dayUntil(c, code, func(k *protocol.Kline) bool {
		if !k.Time.After(stop) { // <= last 已入库，停止
			return true
		}
		if !startAt.IsZero() && k.Time.Before(startAt) {
			return true
		}
		return false
	})
}

// dayUntil 指数走 GetIndex、其余走 GetKline，从新到旧拉取直到 f 返回 true。
func (u *stdUnit) dayUntil(c *tdx.Client, code string, f func(k *protocol.Kline) bool) (protocol.Klines, error) {
	var resp *protocol.KlineResp
	var err error
	if u.kind == "index" {
		resp, err = c.GetIndexUntil(protocol.TypeKlineDay, code, f)
	} else {
		resp, err = c.GetKlineUntil(protocol.TypeKlineDay, code, f)
	}
	if err != nil {
		return nil, err
	}
	return resp.List, nil
}

// fetchMinAll 一次性拉取全部1分钟线（从 Start 起）。
func (u *stdUnit) fetchMinAll(c *tdx.Client, code string, startAt time.Time) (protocol.Klines, error) {
	return u.minUntil(c, code, func(k *protocol.Kline) bool {
		if !startAt.IsZero() && k.Time.Before(startAt) {
			return true
		}
		return false
	})
}

// minUntil 指数走 GetIndex、其余走 GetKlineMinute241（含集合竞价 241 根），从新到旧拉取直到 f 返回 true。
func (u *stdUnit) minUntil(c *tdx.Client, code string, f func(k *protocol.Kline) bool) (protocol.Klines, error) {
	var resp *protocol.KlineResp
	var err error
	if u.kind == "index" {
		resp, err = c.GetIndexUntil(protocol.TypeKlineMinute, code, f)
	} else {
		resp, err = c.GetKlineMinute241Until(code, f)
	}
	if err != nil {
		return nil, err
	}
	return resp.List, nil
}

// --- 具体市场 ---

// AStock 沪深股票市场。
type AStock struct{ stdUnit }

// Index 指数市场（含板块指数 880xxx/881xxx）。
type Index struct{ stdUnit }

// EtfLof ETF/LOF 市场。
type EtfLof struct{ stdUnit }

var (
	_ pull.Unit = (*AStock)(nil)
	_ pull.Unit = (*Index)(nil)
	_ pull.Unit = (*EtfLof)(nil)
)

func init() {
	pull.Register(&AStock{stdUnit{name: pull.MarketAStock, kind: "stock"}})
	pull.Register(&Index{stdUnit{name: pull.MarketIndex, kind: "index"}})
	pull.Register(&EtfLof{stdUnit{name: pull.MarketEtfLof, kind: "etf"}})
}

// ---- Codes 实现 ----

func (u *AStock) Codes(s *pull.Service) ([]pull.Code, error) {
	m, err := u.Manage(s)
	if err != nil {
		return nil, err
	}
	out := []pull.Code{}
	for _, v := range m.Codes.GetStocks() {
		out = append(out, pull.Code{Market: u.name, Code: v.FullCode(), Name: v.Name})
	}
	return out, nil
}

func (u *Index) Codes(s *pull.Service) ([]pull.Code, error) {
	m, err := u.Manage(s)
	if err != nil {
		return nil, err
	}
	out := []pull.Code{}
	for _, v := range m.Codes.GetIndexes() {
		out = append(out, pull.Code{Market: u.name, Code: v.FullCode(), Name: v.Name})
	}
	return out, nil
}

func (u *EtfLof) Codes(s *pull.Service) ([]pull.Code, error) {
	m, err := u.Manage(s)
	if err != nil {
		return nil, err
	}
	out := []pull.Code{}
	for _, v := range m.Codes.GetETFs() {
		out = append(out, pull.Code{Market: u.name, Code: v.FullCode(), Name: v.Name})
	}
	return out, nil
}

// ---- FetchDay 通用实现（含换手率/股本回填，仅股票有） ----

func (u *stdUnit) FetchDay(s *pull.Service, code pull.Code) error {
	m, err := u.Manage(s)
	if err != nil {
		return err
	}
	var equity tdx.IGbbq
	if u.kind == "stock" {
		equity, err = stockEquity(m)
		if err != nil {
			return err
		}
	}
	last, err := s.LastDayUnix(code)
	if err != nil {
		return err
	}
	start := s.Start()

	var ks protocol.Klines
	err = m.IPool.Do(func(c *tdx.Client) error {
		var err error
		if last > 0 {
			ks, err = u.fetchDayFrom(c, code.Code, last, start)
		} else {
			ks, err = u.fetchDayAll(c, code.Code, start)
		}
		return err
	})
	if err != nil {
		return err
	}

	out := make([]*pull.KlineDay, 0, len(ks))
	for _, k := range ks {
		d := &pull.KlineDay{
			Unix:   k.Time.Unix(),
			Open:   k.Open.Float64(),
			High:   k.High.Float64(),
			Low:    k.Low.Float64(),
			Close:  k.Close.Float64(),
			Amount: k.Amount.Float64(),
		}
		// 成交量统一转股：股票/ETF 协议为手×100；指数 Decode 已×100（即股），直接用。
		if u.kind == "index" {
			d.Volume = k.Volume
		} else {
			d.Volume = pull.ToShares(k.Volume)
		}
		// 换手率/股本回填：仅股票有（Gbbq 非股票返回 nil）
		if equity != nil {
			if eq := equity.GetEquity(code.Code, k.Time); eq != nil {
				d.FloatStock = float64(eq.Float)
				d.TotalStock = float64(eq.Total)
				d.Turnover = eq.Turnover(d.Volume)
			}
		}
		out = append(out, d)
	}
	// 空结果保护：已有数据但本次一条未拉到（退市/长期停牌/服务端异常），
	// 跳过写入，避免 upsert "先删后插" 误删库内最后一根。
	if last > 0 && len(out) == 0 {
		return nil
	}
	return s.SaveDay(code, last, out)
}

// FetchMin 将全部待补年份合并为一次分页，股票/ETF 集合竞价也只拆分一次。
func (u *stdUnit) FetchMin(s *pull.Service, code pull.Code) error {
	m, err := u.Manage(s)
	if err != nil {
		return err
	}
	var fallback *minuteFallback
	if s.TradeFallbackEnabled() && (u.kind == "stock" || u.kind == "etf") {
		fallback = &minuteFallback{
			days: func(from time.Time) (protocol.Klines, error) {
				var days protocol.Klines
				err := m.IPool.Do(func(c *tdx.Client) error {
					var err error
					days, err = u.fetchDayAll(c, code.Code, from)
					return err
				})
				return days, err
			},
			trades: func(day time.Time) (protocol.Trades, error) {
				var trades protocol.Trades
				err := m.IPool.Do(func(c *tdx.Client) error {
					resp, err := c.GetHistoryTradeDay(day.Format("20060102"), code.Code)
					if err != nil {
						return err
					}
					trades = resp.List
					return nil
				})
				return trades, err
			},
		}
	}
	return pullMinutes(s, code, time.Now(), func(from time.Time) ([]*pull.KlineMinute, error) {
		var ks protocol.Klines
		err := m.IPool.Do(func(c *tdx.Client) error {
			var err error
			ks, err = u.fetchMinAll(c, code.Code, from)
			return err
		})
		if err != nil {
			return nil, err
		}
		out := make([]*pull.KlineMinute, 0, len(ks))
		for _, k := range ks {
			out = append(out, &pull.KlineMinute{
				Unix: k.Time.Unix(), Open: k.Open.Float64(), High: k.High.Float64(),
				Low: k.Low.Float64(), Close: k.Close.Float64(),
				Volume: pull.ToShares(k.Volume), Amount: k.Amount.Float64(),
			})
		}
		return out, nil
	}, fallback)
}
