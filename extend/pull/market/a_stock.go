// Package market 实现各市场的拉取 Unit（可插拔）。
// 标准行情(7709)：沪深股票/ETF/LOF/指数/板块；扩展行情(7727)：期货/港股/美股。
package market

import (
	"fmt"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/gbbq"
	"github.com/injoyai/tdx/v2/extend/manage"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

// stdUnit 标准行情(7709)市场的基础实现：通过 manage.Manage 连接源取连接。
// 负责原始行情适配与增量边界，分页结果通过 Service 存储。
type stdUnit struct {
	name pull.Market // 市场标识（pull.Market 枚举）
}

func (u *stdUnit) Name() string { return u.name.String() }

// Codes 获取代码列表（由具体市场决定来源）。
func (u *stdUnit) Codes(s *pull.Service) ([]pull.Code, error) {
	return nil, fmt.Errorf("market: %s 未实现 Codes", u.name)
}

// Manage 只获取标准行情连接源；不因查询代码、指数或分钟线触发股本初始化。
func (u *stdUnit) Manage(s *pull.Service) (*manage.Manage, error) {
	m := s.Manage()
	if m == nil {
		return nil, fmt.Errorf("pull: 市场 %s 需要配置 Manage（标准行情7709连接源）", u.name)
	}
	return m, nil
}

// fetchDayAll 一次性拉取全部日线（从 Start 起，直到没有更早数据）。
func (u *stdUnit) fetchDayAll(c tdx.Client, code string, startAt time.Time) (protocol.Klines, error) {
	return u.dayUntil(c, code, func(k *protocol.Kline) bool {
		if !startAt.IsZero() && k.Time.Before(startAt) {
			return true
		}
		return false
	})
}

// fetchDayFrom 拉取日线，直到遇到 last 之前的日期（增量）。
func (u *stdUnit) fetchDayFrom(c tdx.Client, code string, last int64, startAt time.Time) (protocol.Klines, error) {
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

// dayUntil 统一查询自动选择指数或股票解码，从新到旧拉取直到 f 返回 true。
func (u *stdUnit) dayUntil(c tdx.Client, code string, f func(k *protocol.Kline) bool) (protocol.Klines, error) {
	info, ok := u.name.Info()
	if !ok {
		return nil, fmt.Errorf("pull: 市场 %s 没有内置 K 线配置", u.name)
	}
	var resp *tdx.KlineResp
	var err error
	resp, err = c.GetKlineUntil(info.Day.Category, code, func(k tdx.Kline) bool { return f(k.Standard) })
	if err != nil {
		return nil, err
	}
	raw, err := resp.StandardResponse()
	if err != nil {
		return nil, err
	}
	return raw.List, nil
}

// fetchMinAll 一次性拉取全部1分钟线（从 Start 起）。
func (u *stdUnit) fetchMinAll(c tdx.Client, code string, startAt time.Time) (protocol.Klines, error) {
	return u.minUntil(c, code, func(k *protocol.Kline) bool {
		if !startAt.IsZero() && k.Time.Before(startAt) {
			return true
		}
		return false
	})
}

// minUntil 指数走统一分钟查询、其余走 GetKlineMinute241（含集合竞价 241 根），从新到旧拉取直到 f 返回 true。
func (u *stdUnit) minUntil(c tdx.Client, code string, f func(k *protocol.Kline) bool) (protocol.Klines, error) {
	info, ok := u.name.Info()
	if !ok {
		return nil, fmt.Errorf("pull: 市场 %s 没有内置 K 线配置", u.name)
	}
	var resp *tdx.KlineResp
	var err error
	if info.Kind == protocol.KindIndex {
		resp, err = c.GetKlineUntil(info.Minute.Category, code, func(k tdx.Kline) bool { return f(k.Standard) })
	} else {
		resp, err = c.GetKlineMinute241Until(code, func(k tdx.Kline) bool { return f(k.Standard) })
	}
	if err != nil {
		return nil, err
	}
	raw, err := resp.StandardResponse()
	if err != nil {
		return nil, err
	}
	return raw.List, nil
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
	pull.Register(&AStock{stdUnit{name: pull.MarketAStock}})
	pull.Register(&Index{stdUnit{name: pull.MarketIndex}})
	pull.Register(&EtfLof{stdUnit{name: pull.MarketEtfLof}})
}

// ---- Codes 实现 ----

func (a *AStock) Codes(s *pull.Service) ([]pull.Code, error) {
	m, err := a.Manage(s)
	if err != nil {
		return nil, err
	}
	out := []pull.Code{}
	for _, v := range m.Codes.GetStocks() {
		out = append(out, pull.Code{Market: a.name, Code: v.FullCode(), Name: v.Name})
	}
	return out, nil
}

func (i *Index) Codes(s *pull.Service) ([]pull.Code, error) {
	m, err := i.Manage(s)
	if err != nil {
		return nil, err
	}
	out := []pull.Code{}
	for _, v := range m.Codes.GetIndexes() {
		out = append(out, pull.Code{Market: i.name, Code: v.FullCode(), Name: v.Name})
	}
	return out, nil
}

func (e *EtfLof) Codes(s *pull.Service) ([]pull.Code, error) {
	m, err := e.Manage(s)
	if err != nil {
		return nil, err
	}
	out := []pull.Code{}
	for _, v := range m.Codes.GetETFs() {
		out = append(out, pull.Code{Market: e.name, Code: v.FullCode(), Name: v.Name})
	}
	return out, nil
}

// ---- FetchDay 通用实现（含换手率/股本回填，仅股票有） ----

func (u *stdUnit) FetchDay(s *pull.Service, code pull.Code) error {
	info, ok := u.name.Info()
	if !ok {
		return fmt.Errorf("pull: 市场 %s 没有内置 K 线配置", u.name)
	}
	m, err := u.Manage(s)
	if err != nil {
		return err
	}
	var equity gbbq.Interface
	if u.name == pull.MarketAStock {
		equity, err = m.InitGbbq()
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
	err = m.ClientPool.Do(func(c tdx.Client) error {
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
			Volume: info.Day.ToStorageVolume(k.Volume),
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
	return saveDays(s, code, max(last, start.Unix()), out)
}

// FetchMinute 将全部待补年份合并为一次分页，股票/ETF 集合竞价也只拆分一次。
func (u *stdUnit) FetchMinute(s *pull.Service, code pull.Code) error {
	info, ok := u.name.Info()
	if !ok {
		return fmt.Errorf("pull: 市场 %s 没有内置 K 线配置", u.name)
	}
	m, err := u.Manage(s)
	if err != nil {
		return err
	}
	var fallback *minuteFallback
	if s.TradeFallbackEnabled() && (u.name == pull.MarketAStock || u.name == pull.MarketEtfLof) {
		fallback = &minuteFallback{
			days: func(from time.Time) (protocol.Klines, error) {
				var days protocol.Klines
				err := m.ClientPool.Do(func(c tdx.Client) error {
					var err error
					days, err = u.fetchDayAll(c, code.Code, from)
					return err
				})
				return days, err
			},
			trades: func(day time.Time) (protocol.Trades, error) {
				var trades protocol.Trades
				err := m.ClientPool.Do(func(c tdx.Client) error {
					resp, err := c.GetHistoryTradeDay(day.Format("20060102"), code.Code)
					if err != nil {
						return err
					}
					raw, err := resp.StandardResponse()
					if err != nil {
						return err
					}
					trades = raw.List
					return nil
				})
				return trades, err
			},
		}
	}
	return pullMinutes(s, code, time.Now(), func(from time.Time) ([]*pull.KlineMinute, error) {
		var ks protocol.Klines
		err := m.ClientPool.Do(func(c tdx.Client) error {
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
				Volume: info.Minute.ToStorageVolume(k.Volume), Amount: k.Amount.Float64(),
			})
		}
		return out, nil
	}, fallback)
}
