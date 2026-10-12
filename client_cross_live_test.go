package tdx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
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
	rawDay, err := day.StandardResponse()
	if err != nil {
		t.Fatal(err)
	}
	samples["sz_day"] = rawDay.List
	hour, err := std.GetKline60Minute("sz000001", 0, 5)
	if err != nil || len(hour.List) != 5 {
		t.Fatal("standard hourly sample", hour, err)
	}
	rawHour, err := hour.StandardResponse()
	if err != nil {
		t.Fatal(err)
	}
	samples["sz_hour"] = rawHour.List
	if err := std.CloseAll(); err != nil {
		t.Fatal(err)
	}
	ex := liveTestClient(t, true, false)
	bars, err := ex.GetKline(protocol.TypeKlineDay, "hk00700", 0, 5)
	if err != nil || len(bars.List) != 5 {
		t.Fatal("extended daily sample", bars, err)
	}
	rawBars := make([]protocol.ExKline, 0, len(bars.List))
	for _, k := range bars.List {
		rawBars = append(rawBars, *k.Extended)
	}
	samples["hk_day"] = rawBars
	quote, err := ex.GetQuote("hk00700")
	if err != nil || len(quote) != 1 || quote[0].Price <= 0 {
		t.Fatal("extended quote sample", quote, err)
	}
	samples["hk_quote"] = quote[0].Extended
	latest, err := time.Parse("2006-01-02 15:04", bars.List[len(bars.List)-1].Time)
	if err != nil {
		t.Fatal(err)
	}
	date, _ := strconv.ParseUint(latest.Format("20060102"), 10, 32)
	minute, err := ex.GetHistoryMinute(latest.Format("20060102"), "hk00700")
	if err != nil || len(minute.List) == 0 {
		t.Fatal("extended historical minute sample", err)
	}
	samples["hk_minute_date"] = date
	rawMinute := make([]protocol.ExMinuteTick, 0, len(minute.List))
	for _, k := range minute.List {
		rawMinute = append(rawMinute, *k.Extended)
	}
	samples["hk_minute"] = rawMinute
	rangeResult, err := ex.GetKlineRange("hk00700", latest.Format("20060102"), latest.Format("20060102"))
	ranges := []protocol.ExRangeKline{}
	if err == nil {
		for _, k := range rangeResult.List {
			ranges = append(ranges, *k.RangeExtended)
		}
	}
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
