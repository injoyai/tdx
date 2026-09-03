// Package market 实现各市场的拉取 Unit（可插拔）。
// 标准行情(7709)：沪深股票/ETF/LOF/指数/板块；扩展行情(7727)：期货/港股/美股。
package market

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/extend/pull"
	"github.com/injoyai/tdx/protocol"
)

// stdUnit 标准行情(7709)市场的基础实现：通过 tdx.Manage 连接源取连接。
// 日线/分钟线增量逻辑由框架（store.go）统一处理，这里只负责"拉取原始K线并转股"。
type stdUnit struct {
	name pull.Market // 市场标识（pull.Market 枚举）
	kind string      // 分类：stock / etf / index（决定用 GetKline 还是 GetIndex）
}

func (u *stdUnit) Name() string { return u.name.String() }

// Codes 获取代码列表（由具体市场决定来源）。
func (u *stdUnit) Codes(ctx context.Context, s *pull.Service) ([]pull.Code, error) {
	return nil, fmt.Errorf("market: %s 未实现 Codes", u.name)
}

// gbbqMu 保护下述懒加载（多个 stdUnit / goroutine 并发触发时只初始化一次）。
var gbbqMu sync.Mutex

// Manage 取标准行情连接源；未配置返回错误。
// 懒加载 Gbbq：NewManage 默认塞空实现（&Gbbq{}，GetEquity 恒 nil），导致
// 股票日线的流通股/总股本/换手率静默为 0。检测到空实现时自动初始化真实现
// （独立连接，首跑全量拉取一次 gbbq 入库，之后每日 05:09 定时更新）。
func (u *stdUnit) Manage(s *pull.Service) (*tdx.Manage, error) {
	m := s.Manage()
	if m == nil {
		return nil, fmt.Errorf("pull: 市场 %s 需要配置 Manage（标准行情7709连接源）", u.name)
	}
	if _, ok := m.Gbbq.(*tdx.Gbbq); !ok {
		return m, nil // 用户自定义实现或 nil，不动
	}
	gbbqMu.Lock()
	defer gbbqMu.Unlock()
	if g, ok := m.Gbbq.(*tdx.Gbbq); ok && g.IsEmpty() {
		g, err := tdx.NewGbbq() // 独立连接，避免与池内连接并发冲突
		if err != nil {
			return nil, fmt.Errorf("pull: 初始化股本变迁数据失败: %w", err)
		}
		m.Gbbq = g
	}
	return m, nil
}

// fetchDayAll 一次性拉取全部日线（从 Start 起，直到没有更早数据）。
func (u *stdUnit) fetchDayAll(ctx context.Context, c *tdx.Client, code string, startAt time.Time) (protocol.Klines, error) {
	return u.dayUntil(ctx, c, code, func(k *protocol.Kline) bool {
		if !startAt.IsZero() && k.Time.Before(startAt) {
			return true
		}
		return false
	})
}

// fetchDayFrom 拉取日线，直到遇到 last 之前的日期（增量）。
func (u *stdUnit) fetchDayFrom(ctx context.Context, c *tdx.Client, code string, last int64, startAt time.Time) (protocol.Klines, error) {
	stop := time.Unix(last, 0)
	return u.dayUntil(ctx, c, code, func(k *protocol.Kline) bool {
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
func (u *stdUnit) dayUntil(ctx context.Context, c *tdx.Client, code string, f func(k *protocol.Kline) bool) (protocol.Klines, error) {
	var resp *protocol.KlineResp
	var err error
	switch u.kind {
	case "index":
		resp, err = c.GetIndexDayUntil(code, f)
	default:
		resp, err = c.GetKlineDayUntil(code, f)
	}
	if err != nil {
		return nil, err
	}
	return resp.List, nil
}

// fetchMinAll 一次性拉取全部1分钟线（从 Start 起）。
func (u *stdUnit) fetchMinAll(ctx context.Context, c *tdx.Client, code string, startAt time.Time) (protocol.Klines, error) {
	return u.minUntil(ctx, c, code, func(k *protocol.Kline) bool {
		if !startAt.IsZero() && k.Time.Before(startAt) {
			return true
		}
		return false
	})
}

// fetchMinFrom 拉取1分钟线，直到遇到 last 之前的记录（增量）。
func (u *stdUnit) fetchMinFrom(ctx context.Context, c *tdx.Client, code string, last int64, startAt time.Time) (protocol.Klines, error) {
	stop := time.Unix(last, 0)
	return u.minUntil(ctx, c, code, func(k *protocol.Kline) bool {
		if !k.Time.After(stop) { // <= last 已入库，停止
			return true
		}
		if !startAt.IsZero() && k.Time.Before(startAt) {
			return true
		}
		return false
	})
}

// minUntil 指数走 GetIndex、其余走 GetKlineMinute241（含集合竞价 241 根），从新到旧拉取直到 f 返回 true。
func (u *stdUnit) minUntil(ctx context.Context, c *tdx.Client, code string, f func(k *protocol.Kline) bool) (protocol.Klines, error) {
	var resp *protocol.KlineResp
	var err error
	switch u.kind {
	case "index":
		resp, err = c.GetIndexUntil(protocol.TypeKlineMinute, code, f)
	default:
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

func (u *AStock) Codes(ctx context.Context, s *pull.Service) ([]pull.Code, error) {
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

func (u *Index) Codes(ctx context.Context, s *pull.Service) ([]pull.Code, error) {
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

func (u *EtfLof) Codes(ctx context.Context, s *pull.Service) ([]pull.Code, error) {
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

func (u *stdUnit) FetchDay(ctx context.Context, s *pull.Service, code pull.Code) error {
	m, err := u.Manage(s)
	if err != nil {
		return err
	}
	last, err := s.LastDayUnix(code)
	if err != nil {
		return err
	}
	start := s.Start()

	var ks protocol.Klines
	err = m.Do(func(c *tdx.Client) error {
		var err error
		if last > 0 {
			ks, err = u.fetchDayFrom(ctx, c, code.Code, last, start)
		} else {
			ks, err = u.fetchDayAll(ctx, c, code.Code, start)
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
		if u.kind == "stock" && m.Gbbq != nil {
			if eq := m.Gbbq.GetEquity(code.Code, k.Time); eq != nil {
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

// ---- FetchMin 通用实现 ----

func (u *stdUnit) FetchMin(ctx context.Context, s *pull.Service, code pull.Code) error {
	m, err := u.Manage(s)
	if err != nil {
		return err
	}
	start := s.Start()
	now := time.Now()

	// 历史年份：文件不存在的逐个补全（历史不可变，已存在的跳过；
	// 若某年文件中途失败留下半库，可删除该文件后重跑触发全量重拉）
	for y := start.Year(); y < now.Year(); y++ {
		if s.MinExists(code, y) {
			continue
		}
		if err := u.fetchMinYear(ctx, m, s, code, y, start); err != nil {
			return err
		}
	}
	// 今年：增量更新
	return u.fetchMinYear(ctx, m, s, code, now.Year(), start)
}

// fetchMinYear 拉取并写入指定年份的1分钟线。
func (u *stdUnit) fetchMinYear(ctx context.Context, m *tdx.Manage, s *pull.Service, code pull.Code, year int, start time.Time) error {
	last, err := s.LastMinUnix(code, year)
	if err != nil {
		return err
	}
	var ks protocol.Klines
	err = m.Do(func(c *tdx.Client) error {
		var err error
		if last > 0 {
			ks, err = u.fetchMinFrom(ctx, c, code.Code, last, start)
		} else {
			ks, err = u.fetchMinAll(ctx, c, code.Code, start)
		}
		return err
	})
	if err != nil {
		return err
	}

	out := make([]*pull.KlineMinute, 0, len(ks))
	for _, k := range ks {
		// 只保留本年的数据（补去年时同理）
		if k.Time.Year() != year {
			continue
		}
		d := &pull.KlineMinute{
			Unix:   k.Time.Unix(),
			Open:   k.Open.Float64(),
			High:   k.High.Float64(),
			Low:    k.Low.Float64(),
			Close:  k.Close.Float64(),
			Amount: k.Amount.Float64(),
		}
		// 成交量统一转股：股票分钟 Decode 后=手×100；指数分钟经 normalize÷100 后=手，再×100=股。
		d.Volume = pull.ToShares(k.Volume)
		out = append(out, d)
	}
	// 空结果保护：同 FetchDay，避免误删库内最后一根。
	if last > 0 && len(out) == 0 {
		return nil
	}
	return s.SaveMin(code, year, last, out)
}
