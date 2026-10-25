// Package market 实现各市场的拉取 Unit（可插拔）。
// 期货市场：走扩展行情(7727) GetKline 拉取日线/分钟线。
package market

import (
	"fmt"
	"strings"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

// exUnit 扩展行情(7727)市场的基础实现：通过 pull.Service.ExPool 取连接。
// 品种代码列表中，同一 Unit 可能覆盖多个扩展市场（如期货多个交易所），
// 此时交易所简称作为 Code.Code 前缀（"cff/IF2609"），由 marketOf 解析。
type exUnit struct {
	name protocol.Market // 市场标识（protocol.Market 枚举）
	// markets 该 Unit 覆盖的扩展市场编码（用于从 GetInstruments 过滤）
	markets []protocol.Exchange
}

func (u *exUnit) Name() string { return u.name.String() }

// Ex 取扩展行情连接池；未配置返回错误。
func (u *exUnit) Ex(s *pull.Service) (tdx.ClientPool, error) {
	p := s.ExPool()
	if p == nil {
		return nil, fmt.Errorf("pull: 市场 %s 需要配置 ExPool（扩展行情7727连接池）", u.name)
	}
	return p, nil
}

// Codes 从 GetInstruments 分页拉取品种列表，过滤出本 Unit 覆盖的市场编码。
// 多交易所市场（期货）时 Code.Code 带交易所前缀，如 "cff/IF2609"。
func (u *exUnit) Codes(s *pull.Service) ([]pull.Code, error) {
	ins, err := s.GetInstruments()
	if err != nil {
		return nil, err
	}
	out := []pull.Code{}
	for _, it := range ins {
		exchange := protocol.Exchange(it.Market)
		if !u.isMarket(exchange) {
			continue
		}
		code := it.Code
		if u.multiMarket() {
			code = exchange.String() + "/" + it.Code
		}
		out = append(out, pull.Code{Market: u.name, Code: code, Name: it.Name})
	}

	return out, nil
}

// isMarket 判断市场编码是否属于本 Unit。
func (u *exUnit) isMarket(m protocol.Exchange) bool {
	for _, x := range u.markets {
		if x == m {
			return true
		}
	}
	return false
}

// multiMarket 是否覆盖多个扩展市场（需要交易所前缀区分）。
func (u *exUnit) multiMarket() bool { return len(u.markets) > 1 }

// marketOf 解析 Code 的市场编码与裸代码（去掉交易所前缀）。
func (u *exUnit) marketOf(code pull.Code) (protocol.Exchange, string, error) {
	codeStr := code.Code
	if i := strings.IndexByte(codeStr, '/'); i > 0 {
		prefix := codeStr[:i]
		exchange, err := protocol.ParseExchange(prefix)
		// Code.Code 已由 pull 归一化，只接受本 Unit 的规范交易所简称。
		if err == nil && prefix == exchange.String() && u.isMarket(exchange) {
			return exchange, codeStr[i+1:], nil
		}
		return 0, "", fmt.Errorf("pull: 未知的扩展市场前缀 %q", codeStr[:i])
	}
	if len(u.markets) == 0 {
		return 0, "", fmt.Errorf("pull: 市场 %s 未配置扩展市场编码", u.name)
	}
	return u.markets[0], codeStr, nil
}

// bars 复用客户端分页；过滤 Until 包含的界外停止记录。
func (u *exUnit) bars(s *pull.Service, code pull.Code, category uint8, from string) ([]tdx.Kline, error) {
	p, err := u.Ex(s)
	if err != nil {
		return nil, err
	}
	market, bare, err := u.marketOf(code)
	if err != nil {
		return nil, err
	}
	var out []tdx.Kline
	err = p.Do(func(c tdx.Client) error {
		var parseErr error
		r, err := c.GetKlineUntil(category, (protocol.Instrument{Exchange: market, Code: bare}).String(), func(k tdx.Kline) bool {
			layout := protocol.TimeLayout
			if !protocol.IsIntraday(category) {
				layout = time.DateOnly
			}
			at, err := time.Parse(layout, k.Time)
			parseErr = err
			if err == nil && at.Format(layout) != k.Time {
				parseErr = fmt.Errorf("无效源时间 %q", k.Time)
			}
			return parseErr != nil || k.Time <= from
		})
		if err != nil {
			return err
		}
		if parseErr != nil {
			return parseErr
		}
		for _, k := range r.List {
			layout := protocol.TimeLayout
			if !protocol.IsIntraday(category) {
				layout = time.DateOnly
			}
			at, err := time.Parse(layout, k.Time)
			if err != nil {
				return err
			}
			if at.Format(layout) != k.Time {
				return fmt.Errorf("无效源时间 %q", k.Time)
			}
			if k.Time >= from {
				out = append(out, k)
			}
		}
		return nil
	})
	return out, err
}

// FetchDay 增量拉日线，含最后一根以便重插修复。
func (u *exUnit) FetchDay(s *pull.Service, code pull.Code) error {
	info, ok := u.name.Info()
	if !ok {
		return fmt.Errorf("pull: 市场 %s 没有内置 K 线配置", u.name)
	}
	last, err := s.LastDayTime(code)
	if err != nil {
		return err
	}
	from := max(last, s.Start().Format(time.DateOnly))
	bars, err := u.bars(s, code, info.Day.Category, from)
	if err != nil {
		return err
	}
	out := make([]*pull.KlineDay, 0, len(bars))
	for _, k := range bars {
		row := &pull.KlineDay{
			Time: k.Time, Open: k.Open, High: k.High,
			Low: k.Low, Close: k.Close, Volume: k.Volume,
		}
		if k.Amount != nil {
			row.Amount = *k.Amount
		}
		out = append(out, row)
	}
	return saveDays(s, code, from, out)
}

// FetchMinute 多年份共用一次分页，成功扫描后才提交年度完成标记。
func (u *exUnit) FetchMinute(s *pull.Service, code pull.Code) error {
	info, ok := u.name.Info()
	if !ok {
		return fmt.Errorf("pull: 市场 %s 没有内置 K 线配置", u.name)
	}
	return pullMinutes(s, code, time.Now(), func(from string) ([]*pull.KlineMinute, error) {
		bars, err := u.bars(s, code, info.Minute.Category, from)
		if err != nil {
			return nil, err
		}
		out := make([]*pull.KlineMinute, 0, len(bars))
		for _, k := range bars {
			row := &pull.KlineMinute{
				Time: k.Time, Open: k.Open, High: k.High,
				Low: k.Low, Close: k.Close, Volume: k.Volume,
			}
			if k.Amount != nil {
				row.Amount = *k.Amount
			}
			out = append(out, row)
		}
		return out, nil
	}, nil)
}

// Future 期货市场（中金/郑商/大商/上期/广期/期货指数）。
type Future struct{ exUnit }

var _ pull.Unit = (*Future)(nil)

func init() {
	pull.Register(&Future{exUnit{
		name:    protocol.MarketFuture,
		markets: []protocol.Exchange{protocol.ExchangeCFF, protocol.ExchangeCZC, protocol.ExchangeDCE, protocol.ExchangeSHF, protocol.ExchangeGFE, protocol.ExchangeQHZ},
	}})
}
