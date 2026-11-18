package market

import (
	"fmt"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

// exUnit pulls markets using the extended quote protocol.
type exUnit struct{ name protocol.Market }

func (u *exUnit) Name() string { return u.name.String() }

// Ex 取扩展行情连接池；未配置返回错误。
func (u *exUnit) Ex(s *pull.Service) (tdx.ClientPool, error) {
	p := s.ExPool()
	if p == nil {
		return nil, fmt.Errorf("pull: 市场 %s 需要配置 ExPool（扩展行情7727连接池）", u.name)
	}
	return p, nil
}

// Codes filters the shared instrument inventory to this market's wire exchange.
func (u *exUnit) Codes(s *pull.Service) ([]model.Code, error) {
	ex, ok := u.name.Exchange()
	if !ok {
		return nil, fmt.Errorf("pull: 未定义的扩展市场 %s", u.name)
	}
	ins, err := s.GetInstruments()
	if err != nil {
		return nil, err
	}
	out := []model.Code{}
	for _, item := range ins {
		if protocol.Exchange(item.Market) != ex {
			continue
		}
		code := item.Code
		if i, err := u.name.ParseInstrument(code); err == nil {
			code = i.StorageCode()
		}
		// Per-security fetch validates the code, so one malformed row cannot prevent discovery of the rest.
		out = append(out, model.Code{Market: u.name, Code: code, Name: item.Name})
	}
	return out, nil
}

// bars 复用客户端分页；过滤 Until 包含的界外停止记录。
func (u *exUnit) bars(s *pull.Service, code model.Code, category uint8, from string) ([]tdx.Kline, error) {
	if code.Market != u.name {
		return nil, fmt.Errorf("pull: 证券 %s 不属于市场 %s", code.Key(), u.name)
	}
	p, err := u.Ex(s)
	if err != nil {
		return nil, err
	}
	i, err := code.Instrument()
	if err != nil {
		return nil, err
	}
	var out []tdx.Kline
	err = p.DoContext(s.Context(), func(c tdx.Client) error {
		c = tdx.WithContext(s.Context(), c)
		var parseErr error
		r, err := c.GetKlineUntil(category, i.QueryCode(), func(k tdx.Kline) bool {
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
func (u *exUnit) FetchDay(s *pull.Service, code model.Code) error {
	_, err := u.FetchDayResult(s, code)
	return err
}

func (u *exUnit) FetchDayResult(s *pull.Service, code model.Code) (pull.FetchResult, error) {
	info, ok := u.name.Info()
	if !ok {
		return pull.FetchResult{Known: true}, fmt.Errorf("pull: 市场 %s 没有内置 K 线配置", u.name)
	}
	last, err := s.LastDayTime(code)
	if err != nil {
		return pull.FetchResult{Known: true}, err
	}
	from := max(last, s.Start().Format(time.DateOnly))
	bars, err := u.bars(s, code, info.Day.Category, from)
	if err != nil {
		return pull.FetchResult{Known: true}, err
	}
	out := make([]*model.KlineDay, 0, len(bars))
	for _, k := range bars {
		row := &model.KlineDay{
			Time: k.Time, Open: k.Open, High: k.High,
			Low: k.Low, Close: k.Close, Volume: k.Volume,
		}
		row.Amount = k.Amount
		out = append(out, row)
	}
	return saveDaysResult(s, code, from, out)
}

// FetchMinute 多年份共用一次分页，成功扫描后才提交年度完成标记。
func (u *exUnit) FetchMinute(s *pull.Service, code model.Code) error {
	_, err := u.FetchMinuteResult(s, code)
	return err
}

func (u *exUnit) FetchMinuteResult(s *pull.Service, code model.Code) (pull.FetchResult, error) {
	info, ok := u.name.Info()
	if !ok {
		return pull.FetchResult{Known: true}, fmt.Errorf("pull: 市场 %s 没有内置 K 线配置", u.name)
	}
	return pullMinutesResult(s, code, time.Now(), func(from string) ([]*model.KlineMinute, error) {
		bars, err := u.bars(s, code, info.Minute.Category, from)
		if err != nil {
			return nil, err
		}
		out := make([]*model.KlineMinute, 0, len(bars))
		for _, k := range bars {
			row := &model.KlineMinute{
				Time: k.Time, Open: k.Open, High: k.High,
				Low: k.Low, Close: k.Close, Volume: k.Volume,
			}
			row.Amount = k.Amount
			out = append(out, row)
		}
		return out, nil
	}, nil)
}
