package tdx

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2/protocol"
)

var liveLimit struct {
	sync.Mutex
	next  time.Time
	count int
}

// 所有真实测试共用写入限流，包含内部分页、握手和心跳。
func liveWrite(bs []byte) ([]byte, error) {
	liveLimit.Lock()
	defer liveLimit.Unlock()
	if liveLimit.count >= 1000 {
		return nil, errors.New("live test request budget exhausted")
	}
	if delay := time.Until(liveLimit.next); delay > 0 {
		time.Sleep(delay)
	}
	liveLimit.next = time.Now().Add(time.Second)
	liveLimit.count++
	return bs, nil
}

func liveTestClient(t *testing.T, ex, redial bool) *testClient {
	t.Helper()
	hosts, port, env := Hosts, "7709", "TDX_STD_HOST"
	if ex {
		hosts, port, env = ExHosts, ExPort, "TDX_EX_HOST"
	}
	if host := os.Getenv(env); host != "" {
		hosts = []string{host}
	}
	if len(hosts) > 3 {
		hosts = hosts[:3]
	}
	var selected string
	dials := 0
	dial := func(ctx context.Context) (ios.ReadWriteCloser, string, error) {
		dials++
		if dials > 2 {
			return nil, "", errors.New("live test allows only one reconnect")
		}
		candidates := hosts
		if selected != "" {
			candidates = []string{selected}
		}
		var last error
		for i, host := range candidates {
			if i > 0 {
				select {
				case <-ctx.Done():
					return nil, "", ctx.Err()
				case <-time.After(5 * time.Second):
				}
			}
			conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", withPort(host, port))
			if err == nil {
				selected = withPort(host, port)
				return conn, withPort(host, port), nil
			}
			last = err
			t.Logf("dial %s: %v", withPort(host, port), err)
		}
		return nil, "", last
	}
	op := []client.Option{WithRedial(redial), WithDebug(false), client.WithWriteWith(liveWrite), func(c *client.Client) {
		c.OnReconnect = func(int) (time.Duration, error) {
			return 0, errors.New("live test stops after failed dial")
		}
	}}
	var c *testClient
	var err error
	if ex {
		c, err = testDialWith(dial, true, op...)
	} else {
		c, err = testDialWith(dial, false, op...)
	}
	if err != nil {
		t.Fatal("live dial", err)
	}
	c.SetTimeout(8 * time.Second)
	t.Logf("server=%s concurrency=1 write_interval=1s", selected)
	t.Cleanup(func() {
		c.Close()
		liveLimit.Lock()
		t.Logf("live writes total=%d dial lifecycles=%d", liveLimit.count, dials)
		liveLimit.Unlock()
	})
	return c
}

func TestStandardLive(t *testing.T) {
	if os.Getenv("TDX_STD_LIVE") != "1" {
		t.Skip("set TDX_STD_LIVE=1 to run rate-limited standard-market test")
	}
	c := liveTestClient(t, false, true)
	count, err := c.GetCount(protocol.ExchangeSZ)
	if err != nil || count == 0 {
		t.Fatal("GetCount", count, err)
	}
	t.Logf("SZ codes=%d", count)
	day, err := c.GetKlineDay("sz000001", 0, 5)
	if err != nil || len(day.List) == 0 {
		t.Fatal("GetKlineDay", day, err)
	}
	date := day.List[len(day.List)-1].Standard.Time.Format("20060102")
	t.Logf("day count=%d latest=%s close=%.3f", day.Count, date, day.List[len(day.List)-1].Close)
	// 最新交易日的历史分笔可能尚未归档，使用本次日线样本中较早的交易日。
	historyDate := day.List[0].Standard.Time.Format("20060102")
	hour, err := c.GetKline60Minute("sz000001", 0, 5)
	if err != nil || len(hour.List) == 0 {
		t.Fatal("GetKline60Minute", hour, err)
	}
	t.Logf("60 minute count=%d", hour.Count)
	for _, historical := range []bool{false, true} {
		var trades *TradeResp
		if historical {
			trades, err = c.GetHistoryTrade(historyDate, "sz000001", 0, 5)
		} else {
			trades, err = c.GetTrade("sz000001", 0, 5)
		}
		if err != nil || trades.Count == 0 || int(trades.Count) != len(trades.List) {
			t.Fatalf("trade historical=%v: %v %v", historical, trades, err)
		}
		if trades.List[0].Price == nil || *trades.List[0].Price <= 0 || trades.List[0].Volume < 0 {
			t.Fatal("invalid trade", trades.List[0])
		}
		t.Logf("trade historical=%v count=%d first=%+v", historical, trades.Count, trades.List[0])
	}
	r, err := c.SendFrame(protocol.MBlock.FrameMeta(protocol.BlockFileZS))
	if err != nil {
		t.Fatal("block meta", err)
	}
	size := r.(*protocol.BlockMetaResp).Size
	if size == 0 || size > 1500000 {
		t.Fatalf("block sample size %d outside live budget", size)
	}
	data, err := c.GetBlockFileRaw(protocol.BlockFileZS)
	if err != nil || len(data) != int(size) {
		t.Fatalf("block size=%d got=%d err=%v", size, len(data), err)
	}
	t.Logf("block download verified bytes=%d", len(data))
	c.waitMu.Lock()
	previous := c.writer
	c.waitMu.Unlock()
	if err := previous.conn.Close(); err != nil {
		t.Fatal("controlled disconnect", err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for {
		c.waitMu.Lock()
		changed := c.writer != previous
		c.waitMu.Unlock()
		if changed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("controlled reconnect did not publish connection")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if r, err := c.GetCount(protocol.ExchangeSZ); err != nil || r != count {
		t.Fatal("GetCount after reconnect", r, err)
	}
	t.Log("one controlled reconnect verified")
	c.Close()
	select {
	case <-c.Done():
	default:
		t.Fatal("CloseAll did not finish read lifecycle")
	}
	liveLimit.Lock()
	t.Logf("closed; live protocol writes=%d", liveLimit.count)
	liveLimit.Unlock()
}

// TestExLive 实连扩展行情验证。需联网,默认跳过;设 TDX_EXHQ_LIVE=1 启用。
//
//	TDX_EXHQ_LIVE=1 go test . -run TestExLive -v
func TestExLive(t *testing.T) {
	if os.Getenv("TDX_EXHQ_LIVE") != "1" {
		t.Skip("set TDX_EXHQ_LIVE=1 to run live extended-market test")
	}
	cli := liveTestClient(t, true, false)

	cnt, err := cli.protocolClient.(*ExConn).getExtendedCount()
	if err != nil || cnt <= 0 {
		t.Fatalf("ExCount: cnt=%d err=%v", cnt, err)
	}
	t.Logf("ExCount=%d", cnt)

	ins, err := cli.protocolClient.(*ExConn).getExtendedInstruments(0, 5)
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
	if err := std.Close(); err != nil {
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
