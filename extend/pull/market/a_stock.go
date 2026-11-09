// Package market 实现各市场的拉取 Unit（可插拔）。
// 标准行情(7709)：沪深股票/ETF/LOF/指数/板块；扩展行情(7727)：期货/港股/美股。
package market

import (
	"fmt"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

// stdUnit 标准行情(7709)市场的基础实现：直接使用连接池和元数据缓存。
// 负责原始行情适配与增量边界，分页结果通过 Service 存储。
type stdUnit struct {
	name protocol.Market // 市场标识（protocol.Market 枚举）
}

func (u *stdUnit) Name() string { return u.name.String() }

// Codes 从同一份标准代码元数据筛选新增的国内品种。
func (u *stdUnit) Codes(s *pull.Service) ([]model.Code, error) {
	list, err := s.DiscoverCodes()
	if err != nil {
		return nil, err
	}
	out := []model.Code{}
	for _, code := range list {
		if code.Market == u.name {
			out = append(out, code)
		}
	}
	return out, nil
}

func (u *stdUnit) pool(s *pull.Service) (tdx.ClientPool, error) {
	p := s.Pool()
	if p == nil {
		return nil, fmt.Errorf("pull: 市场 %s 需要配置 Pool（标准行情7709连接池）", u.name)
	}
	return p, nil
}

func (u *stdUnit) dayUntil(c tdx.Client, code, from string) ([]tdx.Kline, error) {
	info, ok := u.name.Info()
	if !ok {
		return nil, fmt.Errorf("pull: 市场 %s 没有内置 K 线配置", u.name)
	}
	resp, err := c.GetKlineUntil(info.Day.Category, code, func(k tdx.Kline) bool { return k.Time <= from })
	if err != nil {
		return nil, err
	}
	return resp.List, nil
}

// minUntil 仅股票/ETF 拆分竞价；结果直接使用统一 Client 的股数，不再转换原始协议量。
func (u *stdUnit) minUntil(c tdx.Client, code string, from string) ([]tdx.Kline, error) {
	info, ok := u.name.Info()
	if !ok {
		return nil, fmt.Errorf("pull: 市场 %s 没有内置 K 线配置", u.name)
	}
	stop := func(k tdx.Kline) bool { return k.Time <= from }
	var resp *tdx.KlineResp
	var err error
	if u.name == protocol.MarketAStock || u.name == protocol.MarketEtfLof {
		resp, err = c.GetKlineMinute241Until(code, stop)
	} else {
		resp, err = c.GetKlineUntil(info.Minute.Category, code, stop)
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

// Bond 可转债市场，成交量暂按国内默认100换算。
type Bond struct{ stdUnit }

// Other 其他国内标准品种，不冒用股票分类或股本数据。
type Other struct{ stdUnit }

var (
	_ pull.Unit = (*AStock)(nil)
	_ pull.Unit = (*Index)(nil)
	_ pull.Unit = (*EtfLof)(nil)
	_ pull.Unit = (*Bond)(nil)
	_ pull.Unit = (*Other)(nil)
)

func init() {
	pull.Register(&AStock{stdUnit{name: protocol.MarketAStock}})
	pull.Register(&Index{stdUnit{name: protocol.MarketIndex}})
	pull.Register(&EtfLof{stdUnit{name: protocol.MarketEtfLof}})
	pull.Register(&Bond{stdUnit{name: protocol.MarketBond}})
	pull.Register(&Other{stdUnit{name: protocol.MarketOther}})
}

// ---- FetchDay 通用实现：仅保存原始行情 ----

func (u *stdUnit) FetchDay(s *pull.Service, code model.Code) error {
	pool, err := u.pool(s)
	if err != nil {
		return err
	}
	last, err := s.LastDayTime(code)
	if err != nil {
		return err
	}
	from := max(last, s.Start().Format(time.DateOnly))

	var ks []tdx.Kline
	err = pool.DoContext(s.Context(), func(c tdx.Client) error {
		c = tdx.WithContext(s.Context(), c)
		var err error
		ks, err = u.dayUntil(c, code.Code, from)
		return err
	})
	if err != nil {
		return err
	}

	out := make([]*model.KlineDay, 0, len(ks))
	for _, k := range ks {
		d := &model.KlineDay{
			Time:   k.Time,
			Open:   k.Open,
			High:   k.High,
			Low:    k.Low,
			Close:  k.Close,
			Volume: k.Volume,
		}
		d.Amount = k.Amount

		out = append(out, d)
	}
	return saveDays(s, code, from, out)
}

// FetchMinute 将全部待补年份合并为一次分页，股票/ETF 集合竞价也只拆分一次。
func (u *stdUnit) FetchMinute(s *pull.Service, code model.Code) error {
	pool, err := u.pool(s)
	if err != nil {
		return err
	}
	var fallback *minuteFallback
	if s.TradeFallbackEnabled() && (u.name == protocol.MarketAStock || u.name == protocol.MarketEtfLof) {
		fallback = &minuteFallback{
			days: func(from time.Time) ([]tdx.Kline, error) {
				var days []tdx.Kline
				err := pool.DoContext(s.Context(), func(c tdx.Client) error {
					c = tdx.WithContext(s.Context(), c)
					var err error
					days, err = u.dayUntil(c, code.Code, from.Format(time.DateOnly))
					return err
				})
				return days, err
			},
			trades: func(day time.Time) (protocol.Trades, error) {
				var trades protocol.Trades
				err := pool.DoContext(s.Context(), func(c tdx.Client) error {
					c = tdx.WithContext(s.Context(), c)
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
	return pullMinutes(s, code, time.Now(), func(from string) ([]*model.KlineMinute, error) {
		var ks []tdx.Kline
		err := pool.DoContext(s.Context(), func(c tdx.Client) error {
			c = tdx.WithContext(s.Context(), c)
			var err error
			ks, err = u.minUntil(c, code.Code, from)
			return err
		})
		if err != nil {
			return nil, err
		}
		out := make([]*model.KlineMinute, 0, len(ks))
		for _, k := range ks {
			row := &model.KlineMinute{
				Time: k.Time, Open: k.Open, High: k.High,
				Low: k.Low, Close: k.Close,
				Volume: k.Volume,
			}
			row.Amount = k.Amount
			out = append(out, row)
		}
		return out, nil
	}, fallback)
}
