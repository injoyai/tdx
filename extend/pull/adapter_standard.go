package pull

import (
	"fmt"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

// stdUnit 标准行情(7709)市场的基础实现：直接使用连接池和元数据缓存。
// 负责原始行情适配与增量边界，分页结果通过 Service 存储。
type stdUnit struct {
	name protocol.MarketID // 市场标识（protocol.MarketID 枚举）
}

func (u *stdUnit) Name() string { return u.name.String() }

func (u *stdUnit) Identity() string { return u.name.Identity() }

// Codes 从同一份标准代码元数据筛选新增的国内品种。
func (u *stdUnit) Codes(s *Service) ([]model.Code, error) {
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
	if u.name.DiscoversBlocks() {
		blocks, err := u.blockCodes(s)
		if err != nil {
			return nil, err
		}
		out = append(out, blocks...)
	}
	return out, nil
}

func (u *stdUnit) pool(s *Service) (tdx.ClientPool, error) {
	p := s.Pool()
	if p == nil {
		return nil, fmt.Errorf("pull: 市场 %s 需要配置 Pool（标准行情7709连接池）", u.name)
	}
	return p, nil
}

func (u *stdUnit) dayUntil(c tdx.Client, code, from string) ([]tdx.Kline, error) {
	i, err := protocol.ParseInstrument(code)
	if err != nil {
		return nil, err
	}
	info, ok := i.Rules()
	if !ok {
		return nil, fmt.Errorf("pull: 市场 %s 没有 K 线配置", u.name)
	}
	resp, err := c.GetKlineUntil(info.Day.Category, code, func(k tdx.Kline) bool { return k.Time <= from })
	if err != nil {
		return nil, err
	}
	return resp.List, nil
}

// minUntil 仅股票/ETF 拆分竞价；结果直接使用统一 Client 的股数，不再转换原始协议量。
func (u *stdUnit) minUntil(c tdx.Client, code string, from string) ([]tdx.Kline, error) {
	i, err := protocol.ParseInstrument(code)
	if err != nil {
		return nil, err
	}
	info, ok := i.Rules()
	if !ok {
		return nil, fmt.Errorf("pull: 市场 %s 没有 K 线配置", u.name)
	}
	stop := func(k tdx.Kline) bool { return k.Time <= from }
	var resp *tdx.KlineResp
	if info.SplitAuction {
		resp, err = c.GetKlineMinute241Until(code, stop)
	} else {
		resp, err = c.GetKlineUntil(info.Minute.Category, code, stop)
	}
	if err != nil {
		return nil, err
	}
	return resp.List, nil
}

// ---- FetchDay 通用实现：仅保存原始行情 ----

func (u *stdUnit) FetchDay(s *Service, code model.Code) error {
	_, err := u.FetchDayResult(s, code)
	return err
}

func (u *stdUnit) FetchDayResult(s *Service, code model.Code) (FetchResult, error) {
	if code.Market != u.name {
		return FetchResult{Known: true}, fmt.Errorf("pull: 证券 %s 不属于市场 %s", code.Key(), u.name)
	}
	i, err := code.Instrument()
	if err != nil {
		return FetchResult{Known: true}, err
	}
	pool, err := u.pool(s)
	if err != nil {
		return FetchResult{Known: true}, err
	}
	last, err := s.LastDayTime(code)
	if err != nil {
		return FetchResult{Known: true}, err
	}
	from := max(last, s.Start().Format(time.DateOnly))

	query := standardQuery(i, code)
	var ks []tdx.Kline
	err = pool.DoContext(s.Context(), func(c tdx.Client) error {
		c = tdx.WithContext(s.Context(), c)
		var err error
		ks, err = u.dayUntil(c, query, from)
		return err
	})
	if err != nil {
		return FetchResult{Known: true}, err
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
	return saveDaysResult(s, code, from, out)
}

// FetchMinute 将全部待补年份合并为一次分页，股票/ETF 集合竞价也只拆分一次。
func (u *stdUnit) FetchMinute(s *Service, code model.Code) error {
	_, err := u.FetchMinuteResult(s, code)
	return err
}

func (u *stdUnit) FetchMinuteResult(s *Service, code model.Code) (FetchResult, error) {
	if code.Market != u.name {
		return FetchResult{Known: true}, fmt.Errorf("pull: 证券 %s 不属于市场 %s", code.Key(), u.name)
	}
	pool, err := u.pool(s)
	if err != nil {
		return FetchResult{Known: true}, err
	}
	var fallback *minuteFallback
	i, err := code.Instrument()
	if err != nil {
		return FetchResult{Known: true}, err
	}
	info, _ := i.Rules()
	query := standardQuery(i, code)
	if s.TradeFallbackEnabled() && info.TradeFallback {
		fallback = &minuteFallback{
			days: func(from time.Time) ([]tdx.Kline, error) {
				var days []tdx.Kline
				err := pool.DoContext(s.Context(), func(c tdx.Client) error {
					c = tdx.WithContext(s.Context(), c)
					var err error
					days, err = u.dayUntil(c, query, from.Format(time.DateOnly))
					return err
				})
				return days, err
			},
			trades: func(day time.Time) (protocol.Trades, error) {
				var trades protocol.Trades
				err := pool.DoContext(s.Context(), func(c tdx.Client) error {
					c = tdx.WithContext(s.Context(), c)
					resp, err := c.GetHistoryTradeDay(day.Format("20060102"), query)
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
	return pullMinutesResult(s, code, time.Now(), func(from string) ([]*model.KlineMinute, error) {
		var ks []tdx.Kline
		err := pool.DoContext(s.Context(), func(c tdx.Client) error {
			c = tdx.WithContext(s.Context(), c)
			var err error
			ks, err = u.minUntil(c, query, from)
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

func standardQuery(i protocol.Instrument, code model.Code) string {
	info, _ := code.Market.Info()
	if len(info.Routes) > 1 {
		return i.QueryCode()
	}
	return code.Code
}
