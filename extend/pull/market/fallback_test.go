package market

import (
	"errors"
	"testing"
	"time"

	"github.com/injoyai/tdx/extend/pull"
	"github.com/injoyai/tdx/protocol"
)

func fallbackTrades(day time.Time) protocol.Trades {
	// 模拟历史协议：钟面时间属于交易日，但 Go 时间标记为 UTC。
	stamp := func(h, m int) time.Time {
		return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, time.UTC)
	}
	return protocol.Trades{
		{Time: stamp(9, 30), Price: 12000, Volume: 2},
		{Time: stamp(9, 25), Price: 10000, Volume: 1},
		{Time: stamp(9, 30), Price: 11000, Volume: 3},
		{Time: stamp(13, 0), Price: 13000, Volume: 4},
	}
}

func TestTradesToMinutesTimeAndValues(t *testing.T) {
	loc := time.FixedZone("test-CN", 8*3600)
	day := time.Date(2025, 1, 6, 0, 0, 0, 0, loc)
	trades := fallbackTrades(day)
	before := *trades[0]
	rows, err := tradesToMinutes(day, trades)
	if err != nil || len(rows) != 241 {
		t.Fatal(len(rows), err)
	}
	if *trades[0] != before {
		t.Fatal("修改了原分笔")
	}
	at := func(h, m int) *pull.KlineMinute {
		stamp := time.Date(2025, 1, 6, h, m, 0, 0, loc).Unix()
		for _, k := range rows {
			if k.Unix == stamp {
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
		nil, {nil}, {{Time: day, Price: 1000, Volume: 1}},
		{{Time: day.AddDate(0, 0, -1).Add(10 * time.Hour), Price: 1000, Volume: 1}},
		{{Time: time.Date(2025, 1, 6, 10, 0, 0, 0, time.UTC), Price: 1000}},
	} {
		if _, err := tradesToMinutes(day, bad); err == nil {
			t.Fatal("无效或空分笔被当成完整数据")
		}
	}
}

func TestMinuteFallbackRecoveryAndCoverage(t *testing.T) {
	s, err := pull.NewService(&pull.Config{Dir: t.TempDir(), StartAt: "20250106", Codes: []string{"sz000001"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	code := pull.Code{Market: pull.MarketAStock, Code: "sz000001"}
	day := s.Start()
	day2 := day.AddDate(0, 0, 1)
	now := time.Date(2026, 1, 6, 16, 0, 0, 0, time.Local)
	current := &pull.KlineMinute{Unix: minuteDay(now, time.Local).Add(9*time.Hour + 31*time.Minute).Unix(), Close: 20}
	// 旧标记和本地原生记录必须保留，但不能因此跳过分笔兜底。
	nativeOld := &pull.KlineMinute{Unix: day.Add(9*time.Hour + 31*time.Minute).Unix(), Close: 99}
	if err := s.SaveMinComplete(code, 2025, day.Unix(), []*pull.KlineMinute{nativeOld}); err != nil {
		t.Fatal(err)
	}
	calls := map[int64]int{}
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
			calls[d.Unix()]++
			if fail && d.Equal(day2) {
				return nil, want
			}
			return fallbackTrades(d), nil
		},
	}
	fetch := func(time.Time) ([]*pull.KlineMinute, error) { return []*pull.KlineMinute{current}, nil }
	if err := pullMinutes(s, code, now, fetch, fallback); !errors.Is(err, want) {
		t.Fatal(err)
	}
	rows, err := s.QueryMin(code, time.Time{}, time.Time{})
	if err != nil || len(rows) != 242 {
		t.Fatal("失败丢失了可用数据", len(rows), err)
	}
	for _, k := range rows {
		if k.Unix == nativeOld.Unix && (k.Close != 99 || k.Source != "") {
			t.Fatal("合成覆盖原生", k)
		}
	}
	c, err := s.QueryMinCoverage(code, 2025)
	if err != nil || c == nil || c.TradeFallback {
		t.Fatal("失败标记完成", c, err)
	}
	if c, err := s.QueryMinCoverage(code, 2026); err != nil || c == nil || !c.TradeFallback {
		t.Fatal("其他年份被失败日期连带影响", c, err)
	}
	fail = false
	if err := pullMinutes(s, code, now, fetch, fallback); err != nil {
		t.Fatal(err)
	}
	if calls[day.Unix()] != 1 || calls[day2.Unix()] != 2 || len(calls) != 2 {
		t.Fatal("重复补已完成日或请求停牌日", calls)
	}
	rows, err = s.QueryMin(code, time.Time{}, time.Time{})
	if err != nil || len(rows) != 483 {
		t.Fatal(len(rows), err)
	}
	c, err = s.QueryMinCoverage(code, 2025)
	end := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local).Unix()
	if err != nil || c == nil || !c.TradeFallback || c.From != day.Unix() || c.Through != end {
		t.Fatal(c, err)
	}
	if err := pullMinutes(s, code, now, func(from time.Time) ([]*pull.KlineMinute, error) {
		if from.Unix() != current.Unix {
			t.Fatal("已完成历史仍重扫", from)
		}
		return []*pull.KlineMinute{current}, nil
	}, fallback); err != nil {
		t.Fatal(err)
	}
	if calls[day.Unix()] != 1 || calls[day2.Unix()] != 2 {
		t.Fatal(calls)
	}
}

func TestMinuteFallbackEmptyAndNativeFailure(t *testing.T) {
	s, err := pull.NewService(&pull.Config{Dir: t.TempDir(), StartAt: "20260105", Codes: []string{"sz000001"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := pull.Code{Market: pull.MarketAStock, Code: "sz000001"}
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
	if err := pullMinutes(s, c, now, func(time.Time) ([]*pull.KlineMinute, error) { return nil, network }, fallback); !errors.Is(err, network) || calls != 0 {
		t.Fatal("网络失败进入兜底", err, calls)
	}
	native := &pull.KlineMinute{Unix: minuteDay(now, time.Local).Add(10 * time.Hour).Unix(), Close: 10}
	if err := pullMinutes(s, c, now, func(time.Time) ([]*pull.KlineMinute, error) { return []*pull.KlineMinute{native}, nil }, fallback); err == nil {
		t.Fatal("空分笔未报告缺口")
	}
	rows, err := s.QueryMin(c, time.Time{}, time.Time{})
	if err != nil || len(rows) != 1 || rows[0].Source != "" {
		t.Fatal("空分笔生成了假 K 线", rows, err)
	}
	if coverage, err := s.QueryMinCoverage(c, 2026); err != nil || coverage != nil {
		t.Fatal("空分笔标记完成", coverage, err)
	}
	// 新原生数据可以替换已有合成值。
	native.Source = pull.MinuteSourceTrade
	if err := s.SaveMin(c, 2026, native.Unix, []*pull.KlineMinute{native}); err != nil {
		t.Fatal(err)
	}
	replacement := &pull.KlineMinute{Unix: native.Unix, Close: 88}
	from := minuteDay(now, time.Local)
	rows, err = fallback.fill(s, c, from, now, from, []*pull.KlineMinute{replacement})
	if err != nil || len(rows) != 1 || rows[0].Close != 88 || rows[0].Source != "" {
		t.Fatal("原生未替换合成", rows, err)
	}
}

func TestMinuteFallbackYearRollover(t *testing.T) {
	s, err := pull.NewService(&pull.Config{Dir: t.TempDir(), StartAt: "20251230", Codes: []string{"sz000001"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	code := pull.Code{Market: pull.MarketAStock, Code: "sz000001"}
	day := s.Start()
	now := day.AddDate(0, 0, 4).Add(16 * time.Hour)
	ks, err := tradesToMinutes(day, fallbackTrades(day))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMinFallbackComplete(code, 2025, day.Unix(), day.Add(16*time.Hour).Unix(), ks); err != nil {
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
	if err := pullMinutes(s, code, now, func(from time.Time) ([]*pull.KlineMinute, error) {
		if !from.Equal(day) {
			t.Fatal("跨年跳过未覆盖的年末", from)
		}
		return nil, nil
	}, fallback); err != nil {
		t.Fatal(err)
	}
	rows, err := s.QueryMin(code, day, now)
	if err != nil || len(rows) != 482 || calls != 1 {
		t.Fatal("跨年补拉失败", len(rows), calls, err)
	}
	coverage, err := s.QueryMinCoverage(code, 2025)
	if err != nil || coverage == nil || coverage.Through != time.Date(2026, 1, 1, 0, 0, 0, 0, day.Location()).Unix() {
		t.Fatal("跨年覆盖范围未更新", coverage, err)
	}
}

func TestMinuteFallbackDisabledAfterPartialYear(t *testing.T) {
	cfg := &pull.Config{Dir: t.TempDir(), StartAt: "20251230", Codes: []string{"sz000001"}, TradeFallback: true}
	s, err := pull.NewService(cfg)
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
	code := pull.Code{Market: pull.MarketAStock, Code: "sz000001"}
	day := s.Start()
	ks, err := tradesToMinutes(day, fallbackTrades(day))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMinFallbackComplete(code, 2025, day.Unix(), day.Add(16*time.Hour).Unix(), ks); err != nil {
		t.Fatal(err)
	}
	if complete, err := s.MinYearComplete(code, 2025, day.Unix()); err != nil || complete {
		t.Fatal("部分兜底范围被当作整年完成", complete, err)
	}
	now := time.Date(2026, 1, 2, 16, 0, 0, 0, time.Local)
	called := false
	if err := pullMinutes(s, code, now, func(from time.Time) ([]*pull.KlineMinute, error) {
		called = true
		if !from.Equal(day) {
			t.Fatal("关闭兜底后跳过部分覆盖范围", from)
		}
		return nil, nil
	}, nil); err != nil || !called {
		t.Fatal("关闭兜底后没有继续原生扫描", called, err)
	}
}
