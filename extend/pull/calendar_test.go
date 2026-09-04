package pull

import (
	"testing"
	"time"
)

func TestCalendarWeekAndMetadata(t *testing.T) {
	loc := time.FixedZone("test", 8*3600)
	day := func(y int, m time.Month, d int) *KlineDay {
		return &KlineDay{Unix: time.Date(y, m, d, 15, 0, 0, 0, loc).Unix(), Open: 1, High: 2, Low: 1, Close: 2, Volume: 100, FloatStock: 1000, TotalStock: 2000, Turnover: 10}
	}
	ks := []*KlineDay{day(2026, 1, 5), day(2025, 12, 29), day(2026, 1, 2)}
	ks[2].FloatStock = 1200
	out, err := DayToCalendar(ks, CalendarWeek, loc)
	if err != nil || len(out) != 2 {
		t.Fatal(out, err)
	}
	if out[0].Volume != 200 || out[0].FloatStock != 1200 || out[0].Turnover != 20 || out[0].Unix != ks[2].Unix {
		t.Fatal("ISO 跨年周或元数据错误", out[0])
	}
	if ks[0].Unix != time.Date(2026, 1, 5, 15, 0, 0, 0, loc).Unix() {
		t.Fatal("修改了输入顺序")
	}
	for period, want := range map[CalendarPeriod]int{CalendarMonth: 2, CalendarQuarter: 2, CalendarYear: 2} {
		out, err := DayToCalendar(ks, period, loc)
		if err != nil || len(out) != want {
			t.Fatal(period, len(out), err)
		}
	}
	if out := DayToPeriod(ks, 1); out[0].FloatStock != 1000 || out[0].Turnover != 10 {
		t.Fatal("n=1 丢失字段")
	}
}

func TestMinuteSessions241TwoDays(t *testing.T) {
	loc := time.FixedZone("test", 8*3600)
	sessions := []TradingSession{{570, 690}, {780, 900}}
	var ks []*KlineMinute
	for day := 5; day <= 6; day++ {
		for _, minute := range []int{570} {
			ks = append(ks, &KlineMinute{Unix: time.Date(2026, 1, day, 0, minute, 0, 0, loc).Unix(), Open: 1, High: 1, Low: 1, Close: 1, Volume: 10})
		}
		for _, session := range sessions {
			for m := session.StartMinute + 1; m <= session.EndMinute; m++ {
				ks = append(ks, &KlineMinute{Unix: time.Date(2026, 1, day, 0, m, 0, 0, loc).Unix(), Open: 1, High: 1, Low: 1, Close: 1, Volume: 1})
			}
		}
	}
	out, err := MinuteToSessions(ks, 5, loc, sessions)
	if err != nil || len(out) != 98 {
		t.Fatal(len(out), err)
	}
	var volume int64
	for _, k := range out {
		volume += k.Volume
	}
	if volume != 500 || out[0].Volume != 10 || out[49].Volume != 10 {
		t.Fatal("跨日错位或量丢失", volume)
	}
	if time.Unix(out[25].Unix, 0).In(loc).Format("15:04") != "13:05" {
		t.Fatal("午休后未重新对齐")
	}
}

func TestMinuteSessionsMissingAndOvernight(t *testing.T) {
	loc := time.FixedZone("test", 8*3600)
	base := time.Date(2026, 1, 5, 9, 30, 0, 0, loc)
	ks := []*KlineMinute{{Unix: base.Add(time.Minute).Unix(), Volume: 1}, {Unix: base.Add(5 * time.Minute).Unix(), Volume: 2}, {Unix: base.Add(6 * time.Minute).Unix(), Volume: 3}}
	out, err := MinuteToSessions(ks, 5, loc, []TradingSession{{570, 690}})
	if err != nil || len(out) != 2 || out[0].Volume != 3 || out[1].Volume != 3 {
		t.Fatal("缺分钟导致桶偏移", out, err)
	}
	night := time.Date(2026, 1, 5, 21, 0, 0, 0, loc)
	ks = []*KlineMinute{{Unix: night.Add(time.Minute).Unix(), Volume: 1}, {Unix: night.Add(3 * time.Hour).Unix(), Volume: 2}, {Unix: night.Add(3*time.Hour + time.Minute).Unix(), Volume: 3}}
	out, err = MinuteToSessions(ks, 240, loc, []TradingSession{{1260, 150}})
	if err != nil || len(out) != 1 || out[0].Volume != 6 {
		t.Fatal("夜盘跨午夜被拆开", out, err)
	}
	if _, err := MinuteToSessions(ks, 5, loc, []TradingSession{{570, 690}}); err == nil {
		t.Fatal("时段外应报错")
	}
	if _, err := MinuteToSessions(nil, 5, loc, []TradingSession{{570, 690}, {600, 900}}); err == nil {
		t.Fatal("时段重叠应报错")
	}
	if _, err := DayToCalendar(nil, "bad", loc); err == nil {
		t.Fatal("周期校验缺失")
	}
}
