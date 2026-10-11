package tdx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// 只获取小样本；独立来源的抓取与字段比对按项目规则 8.6 单独串行执行。
func TestCrossValidationSamplesLive(t *testing.T) {
	if os.Getenv("TDX_CROSS_LIVE") != "1" {
		t.Skip("set TDX_CROSS_LIVE=1 to save rate-limited comparison samples")
	}
	samples := map[string]any{"captured_at": time.Now().Format(time.RFC3339)}
	std := liveTestClient(t, false, false)
	day, err := std.GetKlineDay("sz000001", 0, 5)
	if err != nil || len(day.List) != 5 {
		t.Fatal("standard daily sample", day, err)
	}
	samples["sz_day"] = day.List
	hour, err := std.GetKline60Minute("sz000001", 0, 5)
	if err != nil || len(hour.List) != 5 {
		t.Fatal("standard hourly sample", hour, err)
	}
	samples["sz_hour"] = hour.List
	if err := std.CloseAll(); err != nil {
		t.Fatal(err)
	}
	ex := liveTestClient(t, true, false)
	bars, err := ex.ExBars(9, 31, "00700", 0, 5)
	if err != nil || len(bars) != 5 {
		t.Fatal("extended daily sample", bars, err)
	}
	samples["hk_day"] = bars
	quote, err := ex.ExQuote(31, "00700")
	if err != nil || quote.Price <= 0 {
		t.Fatal("extended quote sample", quote, err)
	}
	samples["hk_quote"] = quote
	latest, err := time.Parse("2006-01-02 15:04", bars[len(bars)-1].Datetime)
	if err != nil {
		t.Fatal(err)
	}
	date, _ := strconv.ParseUint(latest.Format("20060102"), 10, 32)
	minute, err := ex.ExHistMinute(31, "00700", uint32(date))
	if err != nil || len(minute) == 0 {
		t.Fatal("extended historical minute sample", err)
	}
	samples["hk_minute_date"] = date
	samples["hk_minute"] = minute
	ranges, err := ex.ExBarsRange(31, "00700", uint32(date), uint32(date))
	if err != nil || len(ranges) == 0 {
		t.Fatal("extended historical range sample", err)
	}
	samples["hk_range"] = ranges
	bs, err := json.MarshalIndent(samples, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join("output", "live-validation", "cross")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tdx-samples.json"), bs, 0644); err != nil {
		t.Fatal(err)
	}
}
