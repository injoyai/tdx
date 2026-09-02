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
