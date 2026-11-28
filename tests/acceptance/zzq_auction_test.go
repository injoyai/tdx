package acceptance_test

import (
	"context"
	"encoding/binary"
	"math"
	"net"
	"testing"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

// TestZZQAuctionAmount 回归 M01：竞价拆分金额必须遵循市场声明的数量倍率。
//
// 需要把 tests/acceptance/_fixtures/zzq_trade.go 通过 -overlay 挂载为
// protocol/market/zzq.go（默认夹具声明分笔未接入，无法构造真实拆分路径），
// 并用 -run TestZZQAuctionAmount 运行，避免与默认夹具的分笔断言冲突。
func TestZZQAuctionAmount(t *testing.T) {
	requireZZQ(t)
	i, err := protocol.ParseInstrument("zzq.123456")
	if err != nil {
		t.Fatal(err)
	}
	if rules, ok := i.Rules(); !ok || !rules.Trade.Supported {
		t.Skip("zzq 分笔未启用（默认夹具），M01 回归需挂载 zzq_trade 夹具")
	}

	cases := []struct {
		name                  string
		quantity, amount      float32
		barVolume, restVolume int64
		barAmount, restAmount float64
	}{
		{"竞价即全部成交", 100, 100, 10, 0, 100, 0},
		{"竞价后仍有余量", 300, 300, 10, 20, 100, 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := startAuctionClient(t, tc.quantity, tc.amount)
			r, err := c.GetKlineMinute241Until("zzq.123456", func(tdx.Kline) bool { return false })
			if err != nil {
				t.Fatal(err)
			}
			if r.Count != 3 {
				t.Fatalf("拆分后数量 = %d, want 3", r.Count)
			}
			bar, rest, plain := r.List[0], r.List[1], r.List[2]
			if bar.Time != "2026-11-20 09:30" || bar.Volume != tc.barVolume {
				t.Fatalf("竞价记录 = %+v", bar)
			}
			if bar.Amount == nil || *bar.Amount != tc.barAmount {
				t.Fatalf("竞价金额 = %v, want %v", bar.Amount, tc.barAmount)
			}
			if rest.Time != "2026-11-20 09:31" || rest.Volume != tc.restVolume {
				t.Fatalf("09:31 余量记录 = %+v", rest)
			}
			if rest.Amount == nil || *rest.Amount != tc.restAmount {
				t.Fatalf("09:31 余量金额 = %v, want %v", rest.Amount, tc.restAmount)
			}
			// 两部分相加必须还原原分钟线业务量额。
			if bar.Volume+rest.Volume != int64(tc.quantity)*10/100 {
				t.Fatalf("量不守恒: %d + %d", bar.Volume, rest.Volume)
			}
			if *bar.Amount+*rest.Amount != float64(tc.amount) {
				t.Fatalf("额不守恒: %v + %v", *bar.Amount, *rest.Amount)
			}
			// 09:32 普通分支保持原结果。
			if plain.Time != "2026-11-20 09:32" || plain.Volume != 10 || plain.Amount == nil || *plain.Amount != 100 {
				t.Fatalf("09:32 普通分支被改动: %+v", plain)
			}
		})
	}
}

// startAuctionClient 只应答 241 拆分需要的报文：三根分钟线与一条 09:25 竞价分笔。
func startAuctionClient(t *testing.T, quantity, amount float32) *tdx.StConn {
	t.Helper()
	server, clientConn := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			h, b, err := readFrame(t, server)
			if err != nil {
				return
			}
			if binary.LittleEndian.Uint32(h[1:5]) == 0 {
				continue
			}
			typ := binary.LittleEndian.Uint16(h[10:12])
			var data []byte
			switch typ {
			case protocol.TypeKline:
				if binary.LittleEndian.Uint16(b[12:14]) != 0 {
					data = protocol.Bytes(uint16(0))
				} else if b[8] == protocol.TypeKlineDay {
					data = auctionDay()
				} else {
					data = auctionBars(quantity, amount)
				}
			case protocol.TypeHistoryTrade, protocol.TypeTrade:
				data = auctionTrade()
			}
			if err := writeFrame(server, h, data); err != nil {
				return
			}
		}
	}()
	dial := func(_ context.Context) (ios.ReadWriteCloser, string, error) { return clientConn, "offline", nil }
	c, err := tdx.DialWith(dial, tdx.WithRedial(false), tdx.WithLevel(tdx.LevelNone))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c.Close()
		server.Close()
		<-done
	})
	c.SetTimeout(2 * time.Second)
	return c
}

// auctionBars 构造 09:31（参与拆分）与 09:32（普通分支）两根分钟线，
// 每根开高低收均为 10.00，原始数量槽 100 → 业务量 10。
func auctionBars(quantity, amount float32) []byte {
	b := protocol.Bytes(uint16(2))
	year, month, date := 2026, 11, 20
	for n, minute := range []uint16{9*60 + 31, 9*60 + 32} {
		qty, amt := quantity, amount
		if n == 1 {
			qty, amt = 100, 100
		}
		b = append(b, protocol.Bytes(uint16((year-2004)*2048+month*100+date))...)
		b = append(b, protocol.Bytes(minute)...)
		b = append(b, wireInt(10000)...) // open=10.00
		b = append(b, wireInt(0)...)
		b = append(b, wireInt(0)...)
		b = append(b, wireInt(0)...)
		b = append(b, protocol.Bytes(math.Float32bits(qty))...)
		b = append(b, protocol.Bytes(math.Float32bits(amt))...)
	}
	return b
}

// auctionDay 构造 2026-11-20 的日线，仅用于 GetHistoryTradeDay 的交易日期校验。
func auctionDay() []byte {
	b := protocol.Bytes(uint16(1))
	b = append(b, protocol.Bytes(uint32(20261120))...)
	b = append(b, 0, 0, 0, 0) // 四个价格差均为 0
	b = append(b, protocol.Bytes(math.Float32bits(100))...)
	b = append(b, protocol.Bytes(math.Float32bits(1000))...)
	return b
}

// auctionTrade 构造 09:25 竞价分笔：价格 10 元、原始量 1（业务量 10）。
func auctionTrade() []byte {
	b := protocol.Bytes(uint16(1))
	b = append(b, 0, 0, 0, 0) // 历史分笔 4 字节头
	b = append(b, protocol.Bytes(uint16(9*60+25))...)
	b = append(b, wireInt(1000)...) // 价格 10.00
	b = append(b, wireInt(1)...)    // 原始量 1
	b = append(b, wireInt(0)...)    // 状态
	b = append(b, wireInt(0)...)
	return b
}
