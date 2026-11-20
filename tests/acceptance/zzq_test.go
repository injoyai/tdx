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

// zzq 测试市场只在 -overlay 挂载 protocol/market/zzq.go 的验收构建中存在。
func requireZZQ(t *testing.T) {
	t.Helper()
	if _, ok := protocol.MarketID("zzq").Exchange(); !ok {
		t.Skip("zzq fixture 未挂载，跳过验收用例")
	}
}

func wireInt(v int) []byte {
	b := []byte{byte(v & 0x3F)}
	for v >>= 6; v > 0; v >>= 7 {
		b = append(b, byte(v&0x7F))
	}
	for i := 0; i < len(b)-1; i++ {
		b[i] |= 0x80
	}
	return b
}

func standardBar(day uint32, minute uint16, quantity, amount float32) []byte {
	b := protocol.Bytes(uint16(1))
	year, month, date := int(day)/10000, int(day)/100%100, int(day)%100
	b = append(b, protocol.Bytes(uint16((year-2004)*2048+month*100+date))...)
	b = append(b, protocol.Bytes(minute)...)
	b = append(b, wireInt(10000)...) // open=10.00
	b = append(b, wireInt(0)...)
	b = append(b, wireInt(0)...)
	b = append(b, wireInt(0)...)
	b = append(b, protocol.Bytes(math.Float32bits(quantity))...)
	b = append(b, protocol.Bytes(math.Float32bits(amount))...)
	return b
}

func standardDayBar(day uint32, quantity, amount float32) []byte {
	b := protocol.Bytes(uint16(1))
	b = append(b, protocol.Bytes(day)...)
	b = append(b, wireInt(10000)...) // open=10.00
	b = append(b, wireInt(0)...)
	b = append(b, wireInt(0)...)
	b = append(b, wireInt(0)...)
	b = append(b, protocol.Bytes(math.Float32bits(quantity))...)
	b = append(b, protocol.Bytes(math.Float32bits(amount))...)
	return b
}

func historyMinute() []byte {
	b := protocol.Bytes(uint16(2))
	b = append(b, 0, 0, 0, 0)
	for i := 0; i < 2; i++ {
		b = append(b, wireInt(1000)...) // 累计价 10.00
		b = append(b, wireInt(0)...)
		b = append(b, wireInt(1)...) // 1 手
	}
	return b
}

type standardFixture struct {
	requests map[uint16]int
}

// startZZQClient 只应答验收需要的标准报文；其余请求记录后返回空数据。
func startZZQClient(t *testing.T) (*tdx.StConn, *standardFixture) {
	t.Helper()
	fixture := &standardFixture{requests: map[uint16]int{}}
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
			fixture.requests[typ]++
			var data []byte
			switch typ {
			case protocol.TypeKline:
				if len(b) >= 16 && binary.LittleEndian.Uint16(b[12:14]) == 0 {
					category := b[8]
					switch {
					case protocol.IsIntraday(category):
						data = standardBar(20261120, 9*60+32, 100, 1000)
					default:
						data = standardDayBar(20261120, 1, 1000)
					}
				} else {
					data = protocol.Bytes(uint16(0))
				}
			case protocol.TypeHistoryMinute:
				data = historyMinute()
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
	return c, fixture
}

func TestZZQStandardMarket(t *testing.T) {
	requireZZQ(t)
	i, err := protocol.ParseInstrument("zzq123456")
	if err != nil {
		t.Fatal(err)
	}
	if i.Exchange != 200 || i.StorageCode() != "zzq123456" {
		t.Fatalf("解析结果=%+v", i)
	}
	rules, ok := i.Rules()
	if !ok || rules.Protocol != protocol.ProtocolStandard || rules.Security != protocol.SecurityStock {
		t.Fatal("zzq 规则声明", rules, ok)
	}
	if !rules.SplitAuction || rules.TradeFallback {
		t.Fatal("拆竞价与分笔补线必须分别声明", rules)
	}
	if !rules.Quote.Supported {
		t.Fatal("报价能力应在部分能力接入时保持可用")
	}

	c, fixture := startZZQClient(t)

	k, err := c.GetKline(protocol.TypeKlineMinute, "zzq123456", 0, 10)
	if err != nil || len(k.List) != 1 {
		t.Fatal(k, err)
	}
	if k.List[0].Time != "2026-11-20 09:32" || k.List[0].Volume != 10 {
		t.Fatalf("标准分钟入口=%+v", k.List[0])
	}

	m, err := c.GetHistoryMinute("20261120", "zzq123456")
	if err != nil || len(m.List) != 2 {
		t.Fatal(m, err)
	}
	if m.List[1].Time != "09:32" || m.List[1].Volume != 10 {
		t.Fatalf("历史分时入口=%+v", m.List[1])
	}

	d, err := c.GetKline(protocol.TypeKlineDay, "zzq123456", 0, 10)
	if err != nil || len(d.List) != 1 {
		t.Fatal(d, err)
	}
	if d.List[0].Time != "2026-11-20" || d.List[0].Close != 10 || d.List[0].Volume != 10 {
		t.Fatalf("日线入口=%+v", d.List[0])
	}

	// 分笔未接入时应发送前拒绝，且不产生协议请求。
	before := fixture.requests[protocol.TypeTrade]
	if _, err := c.GetTradeAll("zzq123456"); err == nil {
		t.Fatal("未接入的分笔能力没有报错")
	}
	if got := fixture.requests[protocol.TypeTrade]; got != before {
		t.Fatalf("未接入分笔仍发送了 %d 次请求", got-before)
	}

	// 44 市场存在协议编号但尚未注册路由。
	if ex, err := protocol.ParseExchange("44"); err != nil {
		t.Fatal("44 协议编号无法识别", err)
	} else if _, ok := ex.Route(); ok {
		t.Fatal("44 不应被注册为已接入市场")
	}
	if _, err := protocol.ParseInstrument("430047.44"); err == nil {
		t.Fatal("未接入市场的显式代码不应解析成功")
	}
}
