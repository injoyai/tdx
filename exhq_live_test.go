package tdx

import (
	"os"
	"testing"
	"time"
)

// TestExHqLive 实连扩展行情验证。需联网,默认跳过;设 TDX_EXHQ_LIVE=1 启用。
//
//	TDX_EXHQ_LIVE=1 go test . -run TestExHqLive -v
func TestExHqLive(t *testing.T) {
	if os.Getenv("TDX_EXHQ_LIVE") == "" {
		t.Skip("set TDX_EXHQ_LIVE=1 to run live extended-market test")
	}
	cli, err := DialExHqDefault()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cli.Close()
	cli.SetTimeout(8 * time.Second)
	time.Sleep(800 * time.Millisecond)

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
}
