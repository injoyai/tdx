package tdx

import (
	"context"
	"errors"
	"net"
	"os"
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

func liveTestClient(t *testing.T, ex, redial bool) *Conn {
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
			conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(host, port))
			if err == nil {
				selected = host
				return conn, host, nil
			}
			last = err
			t.Logf("dial %s:%s: %v", host, port, err)
		}
		return nil, "", last
	}
	op := []client.Option{WithRedial(redial), WithDebug(false), client.WithWriteWith(liveWrite), func(c *client.Client) {
		c.OnReconnect = func(int) (time.Duration, error) {
			return 0, errors.New("live test stops after failed dial")
		}
	}}
	var c *Conn
	var err error
	if ex {
		c, err = dialExHqWith(dial, op...)
	} else {
		c, err = DialWith(dial, op...)
	}
	if err != nil {
		t.Fatal("live dial", err)
	}
	c.SetTimeout(8 * time.Second)
	t.Logf("server=%s:%s concurrency=1 write_interval=1s", selected, port)
	t.Cleanup(func() {
		c.CloseAll()
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
	hour, err := c.GetKline60Minute("sz000001", 0, 5)
	if err != nil || len(hour.List) == 0 {
		t.Fatal("GetKline60Minute", hour, err)
	}
	t.Logf("60 minute count=%d", hour.Count)
	for _, historical := range []bool{false, true} {
		var trades *TradeResp
		if historical {
			trades, err = c.GetHistoryTrade(date, "sz000001", 0, 5)
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
	c.CloseAll()
	select {
	case <-c.Done():
	default:
		t.Fatal("CloseAll did not finish read lifecycle")
	}
	liveLimit.Lock()
	t.Logf("closed; live protocol writes=%d", liveLimit.count)
	liveLimit.Unlock()
}
