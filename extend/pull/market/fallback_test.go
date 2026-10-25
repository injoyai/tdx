package market

import (
	"errors"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

func fallbackTrades(day time.Time) protocol.Trades {
	// 当前协议解码时间使用请求交易日的时区。
	stamp := func(h, m int) time.Time {
		return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, day.Location())
	}
	return protocol.Trades{
		{Time: stamp(9, 30), Price: 12, Volume: 2},
		{Time: stamp(9, 25), Price: 10, Volume: 1},
		{Time: stamp(9, 30), Price: 11, Volume: 3},
		{Time: stamp(13, 0), Price: 13, Volume: 4},
	}
}

func TestTradesToMinutesTimeAndValues(t *testing.T) {
	loc := time.FixedZone("test-CN", 8*3600)
	day := time.Date(2025, 1, 6, 0, 0, 0, 0, loc)
	trades := fallbackTrades(day)
	// 同一瞬间的 UTC 表示应转换到市场时区，而不是改写钟面值。
	trades[0].Time = trades[0].Time.UTC()
	before := *trades[0]
	rows, err := tradesToMinutes(day, trades)
	if err != nil || len(rows) != 241 {
		t.Fatal(len(rows), err)
	}
	if *trades[0] != before {
		t.Fatal("修改了原分笔")
	}
	at := func(h, m int) *pull.KlineMinute {
		stamp := time.Date(2025, 1, 6, h, m, 0, 0, loc).Format("2006-01-02 15:04")
		for _, k := range rows {
			if k.Time == stamp {
				return k
			}
		}
		t.Fatalf("缺少 %02d:%02d 或发生时区偏移", h, m)
		return nil
	}
	if k := at(9, 30); k.Open != 10 || k.Close != 10 || k.Volume != 100 {
		t.Fatal(k)
	}
	if k := at(9, 31); k.Open != 12 || k.High != 12 || k.Low != 11 || k.Close != 11 || k.Volume != 500 || k.Amount != 5700 {
		t.Fatal(k)
	}
	if k := at(9, 32); k.Close != 11 || k.Volume != 0 || k.Amount != 0 {
		t.Fatal("空分钟未沿用既有填充规则", k)
	}
	if k := at(13, 1); k.Close != 13 || k.Volume != 400 {
		t.Fatal(k)
	}
	var volume int64
	var amount float64
	for _, k := range rows {
		if k.Source != pull.MinuteSourceTrade {
			t.Fatal("缺少来源", k)
		}
		volume += k.Volume
		amount += k.Amount
	}
	if volume != 1000 || amount != 11900 {
		t.Fatal(volume, amount)
	}
	for _, bad := range []protocol.Trades{
		nil, {nil}, {{Time: day, Price: 1, Volume: 1}},
		{{Time: day.AddDate(0, 0, -1).Add(10 * time.Hour), Price: 1, Volume: 1}},
		{{Time: time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC), Price: 1}},
	} {
		if _, err := tradesToMinutes(day, bad); err == nil {
			t.Fatal("无效或空分笔被当成完整数据")
		}
	}
}

func TestMinuteFallbackRecoveryAndCoverage(t *testing.T) {
	s, err := pull.New(&pull.Config{Dir: t.TempDir(), StartAt: "20250106", Codes: []string{"sz000001"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	code := pull.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	day := s.Start()
	day2 := day.AddDate(0, 0, 1)
	now := time.Date(2026, 1, 6, 16, 0, 0, 0, protocol.Location())
	current := &pull.KlineMinute{Time: minuteDay(now, protocol.Location()).Add(9*time.Hour + 31*time.Minute).Format("2006-01-02 15:04"), Close: 20}
	// 旧标记和本地原生记录必须保留，但不能因此跳过分笔兜底。
	nativeOld := &pull.KlineMinute{Time: day.Add(9*time.Hour + 31*time.Minute).Format("2006-01-02 15:04"), Close: 99}
	if err := s.SaveMinuteComplete(code, 2025, day.Format("2006-01-02 15:04"), []*pull.KlineMinute{nativeOld}); err != nil {
		t.Fatal(err)
	}
	calls := map[string]int{}
	fail := true
	want := errors.New("trade page failed")
	fallback := &minuteFallback{
		days: func(time.Time) (protocol.Klines, error) {
			return protocol.Klines{
				{Time: day, Volume: 1}, {Time: day2, Volume: 1},
				{Time: day.AddDate(0, 0, 2), Volume: 0}, // 停牌不请求
				{Time: now, Volume: 1},                  // 原生范围不请求
			}, nil
		},
		trades: func(d time.Time) (protocol.Trades, error) {
			calls[d.Format("2006-01-02 15:04")]++
			if fail && d.Equal(day2) {
				return nil, want
			}
			return fallbackTrades(d), nil
		},
	}
	fetch := func(string) ([]*pull.KlineMinute, error) { return []*pull.KlineMinute{current}, nil }
	if err := pullMinutes(s, code, now, fetch, fallback); !errors.Is(err, want) {
		t.Fatal(err)
	}
	rows, err := s.QueryMinute(code, "", "")
	if err != nil || len(rows) != 242 {
		t.Fatal("失败丢失了可用数据", len(rows), err)
	}
	for _, k := range rows {
		if k.Time == nativeOld.Time && (k.Close != 99 || k.Source != "") {
			t.Fatal("合成覆盖原生", k)
		}
	}
	c, err := s.QueryMinuteCoverage(code, 2025)
	if err != nil || c == nil || c.TradeFallback {
		t.Fatal("失败标记完成", c, err)
	}
	if c, err := s.QueryMinuteCoverage(code, 2026); err != nil || c == nil || !c.TradeFallback {
		t.Fatal("其他年份被失败日期连带影响", c, err)
	}
	fail = false
	if err := pullMinutes(s, code, now, fetch, fallback); err != nil {
		t.Fatal(err)
	}
	if calls[day.Format("2006-01-02 15:04")] != 1 || calls[day2.Format("2006-01-02 15:04")] != 2 || len(calls) != 2 {
		t.Fatal("重复补已完成日或请求停牌日", calls)
	}
	rows, err = s.QueryMinute(code, "", "")
	if err != nil || len(rows) != 483 {
		t.Fatal(len(rows), err)
	}
	c, err = s.QueryMinuteCoverage(code, 2025)
	end := time.Date(2026, 1, 1, 0, 0, 0, 0, protocol.Location()).Format("2006-01-02 15:04")
	if err != nil || c == nil || !c.TradeFallback || c.From != day.Format("2006-01-02 15:04") || c.Through != end {
		t.Fatal(c, err)
	}
	if err := pullMinutes(s, code, now, func(from string) ([]*pull.KlineMinute, error) {
		if from != current.Time {
			t.Fatal("已完成历史仍重扫", from)
		}
		return []*pull.KlineMinute{current}, nil
	}, fallback); err != nil {
		t.Fatal(err)
	}
	if calls[day.Format("2006-01-02 15:04")] != 1 || calls[day2.Format("2006-01-02 15:04")] != 2 {
		t.Fatal(calls)
	}
}

func TestMinuteFallbackEmptyAndNativeFailure(t *testing.T) {
	s, err := pull.New(&pull.Config{Dir: t.TempDir(), StartAt: "20260105", Codes: []string{"sz000001"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := pull.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	now := s.Start().AddDate(0, 0, 2).Add(16 * time.Hour)
	calls := 0
	fallback := &minuteFallback{
		days: func(time.Time) (protocol.Klines, error) {
			calls++
			return protocol.Klines{{Time: s.Start(), Volume: 1}}, nil
		},
		trades: func(time.Time) (protocol.Trades, error) { return nil, nil },
	}
	network := errors.New("native network failed")
	if err := pullMinutes(s, c, now, func(string) ([]*pull.KlineMinute, error) { return nil, network }, fallback); !errors.Is(err, network) || calls != 0 {
		t.Fatal("网络失败进入兜底", err, calls)
	}
	native := &pull.KlineMinute{Time: minuteDay(now, protocol.Location()).Add(10 * time.Hour).Format("2006-01-02 15:04"), Close: 10}
	if err := pullMinutes(s, c, now, func(string) ([]*pull.KlineMinute, error) { return []*pull.KlineMinute{native}, nil }, fallback); err == nil {
		t.Fatal("空分笔未报告缺口")
	}
	rows, err := s.QueryMinute(c, "", "")
	if err != nil || len(rows) != 1 || rows[0].Source != "" {
		t.Fatal("空分笔生成了假 K 线", rows, err)
	}
	if coverage, err := s.QueryMinuteCoverage(c, 2026); err != nil || coverage != nil {
		t.Fatal("空分笔标记完成", coverage, err)
	}
	// 新原生数据可以替换已有合成值。
	native.Source = pull.MinuteSourceTrade
	if err := s.SaveMinute(c, 2026, native.Time, []*pull.KlineMinute{native}); err != nil {
		t.Fatal(err)
	}
	replacement := &pull.KlineMinute{Time: native.Time, Close: 88}
	from := minuteDay(now, protocol.Location())
	rows, err = fallback.fill(s, c, from.Format(protocol.TimeLayout), now.Format(protocol.TimeLayout), from.Format(time.DateOnly), []*pull.KlineMinute{replacement})
	if err != nil || len(rows) != 1 || rows[0].Close != 88 || rows[0].Source != "" {
		t.Fatal("原生未替换合成", rows, err)
	}
}

func TestMinuteFallbackYearRollover(t *testing.T) {
	s, err := pull.New(&pull.Config{Dir: t.TempDir(), StartAt: "20251230", Codes: []string{"sz000001"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	code := pull.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	day := s.Start()
	now := day.AddDate(0, 0, 4).Add(16 * time.Hour)
	ks, err := tradesToMinutes(day, fallbackTrades(day))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMinuteFallbackComplete(code, 2025, day.Format("2006-01-02 15:04"), day.Add(16*time.Hour).Format("2006-01-02 15:04"), ks); err != nil {
		t.Fatal(err)
	}
	calls := 0
	fallback := &minuteFallback{
		days: func(time.Time) (protocol.Klines, error) {
			return protocol.Klines{{Time: day, Volume: 1}, {Time: day.AddDate(0, 0, 1), Volume: 1}}, nil
		},
		trades: func(d time.Time) (protocol.Trades, error) {
			calls++
			if !d.Equal(day.AddDate(0, 0, 1)) {
				t.Fatal("重复拉取已完成日期", d)
			}
			return fallbackTrades(d), nil
		},
	}
	// 原生分钟完全为空时，也应通过分笔补齐上一年的剩余交易日。
	if err := pullMinutes(s, code, now, func(from string) ([]*pull.KlineMinute, error) {
		if from != day.Format(protocol.TimeLayout) {
			t.Fatal("跨年跳过未覆盖的年末", from)
		}
		return nil, nil
	}, fallback); err != nil {
		t.Fatal(err)
	}
	rows, err := s.QueryMinute(code, day.Format("2006-01-02 15:04"), now.Format("2006-01-02 15:04"))
	if err != nil || len(rows) != 482 || calls != 1 {
		t.Fatal("跨年补拉失败", len(rows), calls, err)
	}
	coverage, err := s.QueryMinuteCoverage(code, 2025)
	if err != nil || coverage == nil || coverage.Through != time.Date(2026, 1, 1, 0, 0, 0, 0, day.Location()).Format("2006-01-02 15:04") {
		t.Fatal("跨年覆盖范围未更新", coverage, err)
	}
}

func TestMinuteFallbackDisabledAfterPartialYear(t *testing.T) {
	cfg := &pull.Config{Dir: t.TempDir(), StartAt: "20251230", Codes: []string{"sz000001"}, TradeFallback: true}
	s, err := pull.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// Service 必须使用构造时的配置快照。
	cfg.TradeFallback = false
	cfg.Codes[0] = "sh000001"
	if got := s.Config(); !got.TradeFallback || got.Codes[0] != "sz000001" {
		t.Fatal("Service 配置被调用方修改", got)
	}
	code := pull.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	day := s.Start()
	ks, err := tradesToMinutes(day, fallbackTrades(day))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMinuteFallbackComplete(code, 2025, day.Format("2006-01-02 15:04"), day.Add(16*time.Hour).Format("2006-01-02 15:04"), ks); err != nil {
		t.Fatal(err)
	}
	if complete, err := s.MinuteYearComplete(code, 2025, day.Format("2006-01-02 15:04")); err != nil || complete {
		t.Fatal("部分兜底范围被当作整年完成", complete, err)
	}
	now := time.Date(2026, 1, 2, 16, 0, 0, 0, protocol.Location())
	called := false
	if err := pullMinutes(s, code, now, func(from string) ([]*pull.KlineMinute, error) {
		called = true
		if from != day.Format(protocol.TimeLayout) {
			t.Fatal("关闭兜底后跳过部分覆盖范围", from)
		}
		return nil, nil
	}, nil); err != nil || !called {
		t.Fatal("关闭兜底后没有继续原生扫描", called, err)
	}
}

func TestMinuteFallbackIgnoresEmptyNativeScanCache(t *testing.T) {
	s, err := pull.New(&pull.Config{Dir: t.TempDir(), StartAt: "20250106", Codes: []string{"sz000001"}, TradeFallback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	code := pull.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	day := s.Start()
	now := time.Date(2026, 1, 6, 16, 0, 0, 0, protocol.Location())
	if err := s.SaveMinuteEmptyScan(code, 2025, day.Format("2006-01-02 15:04"), now); err != nil {
		t.Fatal(err)
	}
	calls := 0
	fallback := &minuteFallback{
		days: func(time.Time) (protocol.Klines, error) {
			return protocol.Klines{{Time: day, Volume: 1}}, nil
		},
		trades: func(d time.Time) (protocol.Trades, error) {
			calls++
			return fallbackTrades(d), nil
		},
	}
	err = pullMinutes(s, code, now, func(from string) ([]*pull.KlineMinute, error) {
		if from != day.Format(protocol.TimeLayout) {
			t.Fatal("空原生历史缓存阻止了分笔兜底", from)
		}
		return []*pull.KlineMinute{{Time: now.Add(-time.Hour).Format("2006-01-02 15:04"), Close: 20}}, nil
	}, fallback)
	if err != nil || calls != 1 {
		t.Fatal("未补缺失历史交易日", calls, err)
	}
	rows, err := s.QueryMinute(code, "", "")
	if err != nil || len(rows) != 242 {
		t.Fatal("分笔兜底数据未写入", len(rows), err)
	}
}
