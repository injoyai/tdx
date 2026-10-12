package tdx

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

// TestExHqLive 实连扩展行情验证。需联网,默认跳过;设 TDX_EXHQ_LIVE=1 启用。
//
//	TDX_EXHQ_LIVE=1 go test . -run TestExHqLive -v
func TestExHqLive(t *testing.T) {
	if os.Getenv("TDX_EXHQ_LIVE") != "1" {
		t.Skip("set TDX_EXHQ_LIVE=1 to run live extended-market test")
	}
	cli := liveTestClient(t, true, false)

	cnt, err := cli.getExtendedCount()
	if err != nil || cnt <= 0 {
		t.Fatalf("ExCount: cnt=%d err=%v", cnt, err)
	}
	t.Logf("ExCount=%d", cnt)

	ins, err := cli.getExtendedInstruments(0, 5)
	if err != nil || len(ins) == 0 {
		t.Fatalf("ExInstruments: n=%d err=%v", len(ins), err)
	}
	for _, x := range ins {
		t.Logf("instr cat=%d mkt=%d %s %s", x.Category, x.Market, x.Code, x.Name)
	}

	quotesResp, err := cli.GetQuote("hk00700") // 港股腾讯
	if err != nil || len(quotesResp) != 1 {
		t.Fatalf("GetQuote: %v", err)
	}
	q := quotesResp[0]
	t.Logf("HK00700 pre=%.2f open=%.2f high=%.2f low=%.2f price=%.2f bid1=%.2f ask1=%.2f",
		q.PreClose, q.Open, q.High, q.Low, q.Price, q.Bid[0], q.Ask[0])
	if q.Price <= 0 {
		t.Errorf("HK quote price invalid: %.2f", q.Price)
	}

	bars, err := cli.GetKline(protocol.TypeKlineDay, "hk00700", 0, 5) // 港股日K，Trade 为百股
	if err != nil || len(bars.List) == 0 {
		t.Fatalf("GetKline: n=%d err=%v", len(bars.List), err)
	}
	for _, b := range bars.List {
		t.Logf("bar %s O=%.2f H=%.2f L=%.2f C=%.2f vol=%d", b.Time, b.Open, b.High, b.Low, b.Close, b.Volume)
	}
	markets, err := cli.GetMarkets()
	if err != nil || len(markets) == 0 {
		t.Fatal("ExMarkets", len(markets), err)
	}
	t.Logf("ExMarkets=%d", len(markets))
	latest, err := time.Parse("2006-01-02 15:04", bars.List[len(bars.List)-1].Time)
	if err != nil {
		t.Fatal(err)
	}
	date, _ := strconv.ParseUint(latest.Format("20060102"), 10, 32)
	minute, err := cli.GetMinute("hk00700")
	if err != nil {
		t.Fatal("GetMinute", err)
	}
	t.Logf("GetMinute=%d", len(minute.List))
	historyMinute, err := cli.GetHistoryMinute(latest.Format("20060102"), "hk00700")
	if err != nil {
		t.Fatal("GetHistoryMinute", err)
	}
	t.Logf("GetHistoryMinute date=%d count=%d", date, len(historyMinute.List))
	trades, err := cli.GetTrade("hk00700", 0, 5)
	if err != nil {
		t.Fatal("GetTrade", err)
	}
	t.Logf("GetTrade=%d", len(trades.List))
	historyTrades, err := cli.GetHistoryTrade(latest.Format("20060102"), "hk00700", 0, 5)
	if err != nil {
		t.Fatal("GetHistoryTrade", err)
	}
	t.Logf("GetHistoryTrade date=%d count=%d", date, len(historyTrades.List))
	quotes, err := cli.GetQuoteList(31, 2, 0, 5)
	if err != nil || len(quotes) == 0 {
		t.Fatal("GetQuoteList", len(quotes), err)
	}
	t.Logf("GetQuoteList=%d first=%s price=%.3f", len(quotes), quotes[0].Code, quotes[0].Price)
	ranges, err := cli.GetKlineRange("hk00700", latest.Format("20060102"), latest.Format("20060102"))
	if err != nil {
		t.Fatal("GetKlineRange", err)
	}
	t.Logf("GetKlineRange date=%d count=%d", date, ranges.Count)
}
