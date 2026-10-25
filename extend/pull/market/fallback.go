package market

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

// minuteFallback 仅供股票/ETF 使用；协议请求仍由已有客户端接口完成。
type minuteFallback struct {
	days   func(from time.Time) (protocol.Klines, error)
	trades func(day time.Time) (protocol.Trades, error)
}

// fill 只补原生分钟最早日期之前的交易日，返回可落库数据和未完成错误。
// 本地数据参与合并，避免补旧日期时被现有删尾重插规则删除；原生数据优先。
func (f *minuteFallback) fill(s *pull.Service, code pull.Code, fromValue, endValue, boundaryValue string,
	native []*pull.KlineMinute) ([]*pull.KlineMinute, error) {
	from, err := time.ParseInLocation(protocol.TimeLayout, fromValue, protocol.Location())
	if err != nil {
		return nil, err
	}
	end, err := time.ParseInLocation(protocol.TimeLayout, endValue, protocol.Location())
	if err != nil {
		return nil, err
	}
	boundary, err := time.ParseInLocation(time.DateOnly, boundaryValue, protocol.Location())
	if err != nil {
		return nil, err
	}
	local, err := s.QueryMinute(code, fromValue, end.Add(-time.Minute).Format(protocol.TimeLayout))
	if err != nil {
		return nil, err
	}
	rows := make(map[string]*pull.KlineMinute, len(local)+len(native))
	for _, k := range local {
		rows[k.Time] = k
	}
	limit := minuteDay(end, from.Location())
	if boundary.After(limit) {
		boundary = limit
	}
	for _, k := range native {
		rows[k.Time] = k
	}
	result := func() []*pull.KlineMinute {
		out := make([]*pull.KlineMinute, 0, len(rows))
		for _, k := range rows {
			out = append(out, k)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Time < out[j].Time })
		return out
	}
	if !from.Before(boundary) {
		return result(), nil
	}
	days, err := f.days(from)
	if err != nil {
		return result(), fmt.Errorf("pull: %s 获取兜底交易日: %w", code.Key(), err)
	}
	seen := map[int64]bool{}
	var dates []time.Time
	for _, k := range days {
		day := minuteDay(k.Time, from.Location())
		if k.Volume <= 0 || day.Before(minuteDay(from, from.Location())) || !day.Before(boundary) || seen[day.Unix()] {
			continue
		}
		seen[day.Unix()] = true
		dates = append(dates, day)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	var failures []error
	for _, day := range dates {
		if hasMinuteDay(rows, day) {
			continue
		}
		trades, err := f.trades(day)
		if err != nil {
			failures = append(failures, fmt.Errorf("pull: %s %s 分笔拉取: %w", code.Key(), day.Format("20060102"), err))
			continue
		}
		ks, err := tradesToMinutes(day, trades)
		if err != nil {
			failures = append(failures, fmt.Errorf("pull: %s %s 分笔合成: %w", code.Key(), day.Format("20060102"), err))
			continue
		}
		for _, k := range ks {
			if k.Time >= fromValue && rows[k.Time] == nil {
				rows[k.Time] = k
			}
		}
	}
	return result(), errors.Join(failures...)
}

func minuteDay(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// hasMinuteDay 不把只有几根记录的旧库当作完整交易日。
func hasMinuteDay(rows map[string]*pull.KlineMinute, day time.Time) bool {
	for m := 570; m <= 900; m++ {
		if m > 690 && m <= 780 {
			continue
		}
		t := time.Date(day.Year(), day.Month(), day.Day(), m/60, m%60, 0, 0, day.Location())
		if rows[t.Format(protocol.TimeLayout)] == nil {
			return false
		}
	}
	return true
}

// tradesToMinutes 复用 Trades.Klines 的 241 根规则；合成值是近似值。
// 按请求交易日的时区校验时间，不重新标记分笔的时区。
func tradesToMinutes(day time.Time, trades protocol.Trades) ([]*pull.KlineMinute, error) {
	if len(trades) == 0 {
		return nil, errors.New("历史分笔为空，保留缺口")
	}
	normalized := make(protocol.Trades, 0, len(trades))
	var volume int64
	for _, trade := range trades {
		if trade == nil {
			return nil, errors.New("历史分笔包含空记录")
		}
		at := trade.Time.In(day.Location())
		y, m, d := at.Date()
		minute := at.Hour()*60 + at.Minute()
		if y != day.Year() || m != day.Month() || d != day.Day() ||
			trade.Price <= 0 || trade.Volume < 0 ||
			!((minute >= 565 && minute <= 690) || (minute >= 780 && minute <= 900)) {
			return nil, errors.New("历史分笔日期、时间或量价无效")
		}
		copy := *trade
		copy.Time = at.Truncate(time.Minute)
		normalized = append(normalized, &copy)
		volume += int64(trade.Volume)
	}
	if volume == 0 {
		return nil, errors.New("历史分笔没有有效成交量，保留缺口")
	}
	sort.SliceStable(normalized, func(i, j int) bool { return normalized[i].Time.Before(normalized[j].Time) })
	ks := normalized.Klines()
	out := make([]*pull.KlineMinute, 0, len(ks))
	for _, k := range ks {
		out = append(out, &pull.KlineMinute{
			Time: k.Time.Format(protocol.TimeLayout), TradingDate: day.Format(time.DateOnly), Open: k.Open, High: k.High,
			Low: k.Low, Close: k.Close, Volume: protocol.ToShares(k.Volume),
			Amount: k.Amount, Source: pull.MinuteSourceTrade,
		})
	}
	return out, nil
}
