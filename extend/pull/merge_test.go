package pull

import (
	"testing"
	"time"
)

func mkDay(unix int64, open, close float64, vol int64) *KlineDay {
	return &KlineDay{
		Unix:   unix,
		Open:   open,
		High:   close,
		Low:    open,
		Close:  close,
		Volume: vol,
		Amount: 0,
	}
}

func TestDayToPeriod(t *testing.T) {
	// 6 个连续自然日的日线（2026-01-05 起，Merge 为固定 N 根分块、不按日历对齐）
	base := time.Date(2026, 1, 5, 15, 0, 0, 0, time.Local)
	ks := make([]*KlineDay, 0, 6)
	for i := 0; i < 6; i++ {
		ks = append(ks, mkDay(base.AddDate(0, 0, i).Unix(), float64(i+1), float64(i+1), int64(i+1)))
	}

	// 合并成 5 日周期：1 根（前5个）+ 1 根（第6个）
	out := DayToPeriod(ks, 5)
	if len(out) != 2 {
		t.Fatalf("DayToPeriod(len=%d,5) = %d 根, want 2", len(ks), len(out))
	}
	// 第一根：Open=1，High=5，Low=1，Close=5，Volume=1+2+3+4+5=15
	if out[0].Open != 1 || out[0].High != 5 || out[0].Low != 1 || out[0].Close != 5 {
		t.Errorf("第一根 OHLC 错误: %+v", out[0])
	}
	if out[0].Volume != 15 {
		t.Errorf("第一根 Volume = %d, want 15", out[0].Volume)
	}
	// 第二根：Open=6, Close=6, Volume=6
	if out[1].Open != 6 || out[1].Close != 6 || out[1].Volume != 6 {
		t.Errorf("第二根错误: %+v", out[1])
	}
}

func TestDayToPeriodSingle(t *testing.T) {
	// n<=1 原样返回
	ks := []*KlineDay{mkDay(1, 1, 1, 1)}
	if out := DayToPeriod(ks, 1); len(out) != 1 {
		t.Fatalf("n=1 应原样返回, got %d", len(out))
	}
	if out := DayToPeriod(nil, 5); len(out) != 0 {
		t.Fatalf("空输入应返回空, got %d", len(out))
	}
}

func mkMinute(unix int64, open, close float64, vol int64) *KlineMinute {
	return &KlineMinute{
		Unix:   unix,
		Open:   open,
		High:   close,
		Low:    open,
		Close:  close,
		Volume: vol,
	}
}

func TestMinuteToPeriod(t *testing.T) {
	// 10 根 1 分钟线，合并成 5 分钟
	base := time.Date(2026, 1, 5, 9, 31, 0, 0, time.Local)
	ks := make([]*KlineMinute, 0, 10)
	for i := 0; i < 10; i++ {
		ks = append(ks, mkMinute(base.Add(time.Duration(i)*time.Minute).Unix(), float64(i+1), float64(i+1), int64(i+1)))
	}

	out := MinuteToPeriod(ks, 5)
	if len(out) != 2 {
		t.Fatalf("MinuteToPeriod(len=%d,5) = %d 根, want 2", len(ks), len(out))
	}
	if out[0].Open != 1 || out[0].High != 5 || out[0].Low != 1 || out[0].Close != 5 {
		t.Errorf("第一根 OHLC 错误: %+v", out[0])
	}
	if out[0].Volume != 15 {
		t.Errorf("第一根 Volume = %d, want 15", out[0].Volume)
	}
	if out[1].Open != 6 || out[1].Close != 10 || out[1].Volume != 40 {
		t.Errorf("第二根错误: %+v", out[1])
	}
}

func TestPeriodMergePreservesPrecisionAndInput(t *testing.T) {
	for _, price := range []float64{0.1234, -1.2345} {
		days := []*KlineDay{
			{Unix: 1, Open: price, High: price + 0.0002, Low: price - 0.0002, Close: price, Amount: 0.0004, Volume: 2, Turnover: 1, FloatStock: 100, TotalStock: 200},
			{Unix: 2, Open: price, High: price + 0.0003, Low: price - 0.0003, Close: price + 0.0001, Amount: 0.0005, Volume: 3, Turnover: 2, FloatStock: 110, TotalStock: 220},
		}
		got := DayToPeriod(days, 2)
		if len(got) != 1 || got[0].Open != days[0].Open || got[0].Close != days[1].Close || got[0].High != days[1].High || got[0].Low != days[1].Low || got[0].Amount != days[0].Amount+days[1].Amount || got[0].Volume != 5 || got[0].Turnover != 3 || got[0].FloatStock != 110 || got[0].TotalStock != 220 || got[0].Unix != 2 {
			t.Fatal("day precision/metadata lost", got)
		}
		minutes := []*KlineMinute{
			{Unix: 1, Open: price, High: price + 0.0002, Low: price - 0.0002, Close: price, Amount: 0.0004, Volume: 2, Source: MinuteSourceTrade},
			{Unix: 2, Open: price, High: price + 0.0003, Low: price - 0.0003, Close: price + 0.0001, Amount: 0.0005, Volume: 3},
		}
		min := MinuteToPeriod(minutes, 2)
		if len(min) != 1 || min[0].Open != got[0].Open || min[0].Close != got[0].Close || min[0].High != got[0].High || min[0].Low != got[0].Low || min[0].Amount != got[0].Amount || min[0].Volume != 5 || min[0].Source != MinuteSourceTrade {
			t.Fatal("minute precision/source lost", min)
		}
		if days[0].Volume != 2 || minutes[0].Volume != 2 {
			t.Fatal("mutated input")
		}
	}
}
