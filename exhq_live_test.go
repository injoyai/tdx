package tdx

import (
	"os"
	"strconv"
	"testing"
	"time"
)

// TestExHqLive 实连扩展行情验证。需联网,默认跳过;设 TDX_EXHQ_LIVE=1 启用。
//
//	TDX_EXHQ_LIVE=1 go test . -run TestExHqLive -v
func TestExHqLive(t *testing.T) {
	if os.Getenv("TDX_EXHQ_LIVE") != "1" {
		t.Skip("set TDX_EXHQ_LIVE=1 to run live extended-market test")
	}
	cli := liveTestClient(t, true, false)

	cnt, err := cli.ExCount()
	if err != nil || cnt <= 0 {
		t.Fatalf("ExCount: cnt=%d err=%v", cnt, err)
	}
	t.Logf("ExCount=%d", cnt)

	ins, err := cli.ExInstruments(0, 5)
	if err != nil || len(ins) == 0 {
		t.Fatalf("ExInstruments: n=%d err=%v", len(ins), err)
	}
	for _, x := range ins {
		t.Logf("instr cat=%d mkt=%d %s %s", x.Category, x.Market, x.Code, x.Name)
	}

	q, err := cli.ExQuote(31, "00700") // 港股腾讯
	if err != nil {
		t.Fatalf("ExQuote: %v", err)
	}
	t.Logf("HK00700 pre=%.2f open=%.2f high=%.2f low=%.2f price=%.2f bid1=%.2f ask1=%.2f",
		q.PreClose, q.Open, q.High, q.Low, q.Price, q.Bid[0], q.Ask[0])
	if q.Price <= 0 {
		t.Errorf("HK quote price invalid: %.2f", q.Price)
	}

	bars, err := cli.ExBars(9, 31, "00700", 0, 5) // 日K
	if err != nil || len(bars) == 0 {
		t.Fatalf("ExBars: n=%d err=%v", len(bars), err)
	}
	for _, b := range bars {
		t.Logf("bar %s O=%.2f H=%.2f L=%.2f C=%.2f vol=%d", b.Datetime, b.Open, b.High, b.Low, b.Close, b.Trade)
	}
	markets, err := cli.ExMarkets()
	if err != nil || len(markets) == 0 {
		t.Fatal("ExMarkets", len(markets), err)
	}
	t.Logf("ExMarkets=%d", len(markets))
	latest, err := time.Parse("2006-01-02 15:04", bars[len(bars)-1].Datetime)
	if err != nil {
		t.Fatal(err)
	}
	date, _ := strconv.ParseUint(latest.Format("20060102"), 10, 32)
	minute, err := cli.ExMinute(31, "00700")
	if err != nil {
		t.Fatal("ExMinute", err)
	}
	t.Logf("ExMinute=%d", len(minute))
	historyMinute, err := cli.ExHistMinute(31, "00700", uint32(date))
	if err != nil {
		t.Fatal("ExHistMinute", err)
	}
	t.Logf("ExHistMinute date=%d count=%d", date, len(historyMinute))
	trades, err := cli.ExTrade(31, "00700", 0, 5)
	if err != nil {
		t.Fatal("ExTrade", err)
	}
	t.Logf("ExTrade=%d", len(trades))
	historyTrades, err := cli.ExHistTrade(31, "00700", uint32(date), 0, 5)
	if err != nil {
		t.Fatal("ExHistTrade", err)
	}
	t.Logf("ExHistTrade date=%d count=%d", date, len(historyTrades))
	quotes, err := cli.ExQuoteList(31, 2, 0, 5)
	if err != nil || len(quotes) == 0 {
		t.Fatal("ExQuoteList", len(quotes), err)
	}
	t.Logf("ExQuoteList=%d first=%s price=%.3f", len(quotes), quotes[0].Code, quotes[0].Price)
	ranges, err := cli.ExBarsRange(31, "00700", uint32(date), uint32(date))
	if err != nil {
		t.Fatal("ExBarsRange", err)
	}
	t.Logf("ExBarsRange date=%d count=%d", date, len(ranges))
}
