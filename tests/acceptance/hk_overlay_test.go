package acceptance_test

import (
	"context"
	"encoding/binary"
	"io"
	"math"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/extend/storage/sqlite"
	"github.com/injoyai/tdx/v2/protocol"
)

// 测试报文固定：一根 K 线的原始数量槽为 2。正常构建下香港基金分钟倍率为 100，
// overlay 只把市场 49 的分钟倍率改为 10，因此期望值由环境变量给出。
func hkFundMinuteScale(t *testing.T) int64 {
	t.Helper()
	raw := os.Getenv("TDX_ACCEPT_HK_FUND_MINUTE_SCALE")
	if raw == "" {
		return 100
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func readFrame(t *testing.T, r net.Conn) ([]byte, []byte, error) {
	h := make([]byte, 12)
	if _, err := io.ReadFull(r, h); err != nil {
		return nil, nil, err
	}
	n := int(binary.LittleEndian.Uint16(h[6:8])) - 2
	if n < 0 {
		return nil, nil, io.ErrUnexpectedEOF
	}
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	return h, b, err
}

func writeFrame(w net.Conn, h, data []byte) error {
	resp := make([]byte, 16+len(data))
	copy(resp, []byte{0xb1, 0xcb, 0x74, 0, 0x1c})
	copy(resp[5:9], h[1:5])
	copy(resp[10:12], h[10:12])
	binary.LittleEndian.PutUint16(resp[12:14], uint16(len(data)))
	binary.LittleEndian.PutUint16(resp[14:16], uint16(len(data)))
	copy(resp[16:], data)
	_, err := w.Write(resp)
	return err
}

// extendedBars 构造一根扩展 K 线：收 10、量槽 2、金额 0.001（按品种规则再换算）。
func extendedBars(category uint8) []byte {
	data := append(make([]byte, 0, 18+2+32), make([]byte, 18)...)
	data = binary.LittleEndian.AppendUint16(data, 1)
	switch {
	case protocol.IsIntraday(category):
		ymd := uint16((2026-2004)<<11 | 11*100 + 20)
		data = binary.LittleEndian.AppendUint16(data, ymd)
		data = binary.LittleEndian.AppendUint16(data, 9*60+32)
	default:
		data = binary.LittleEndian.AppendUint32(data, 20261120)
	}
	for _, v := range []float32{10, 10, 10, 10} {
		data = binary.LittleEndian.AppendUint32(data, math.Float32bits(v))
	}
	data = binary.LittleEndian.AppendUint32(data, math.Float32bits(0.001)) // position / amount
	data = binary.LittleEndian.AppendUint32(data, 2)                       // trade
	data = binary.LittleEndian.AppendUint32(data, math.Float32bits(10))
	return data
}

// start dials an extended fixture that answers every bars page. Only the first
// page carries one bar; later pages are empty so paging terminates.
func startExtendedFixture(t *testing.T) *tdx.ExConn {
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
			if len(b) < 14 || binary.LittleEndian.Uint16(h[10:12]) != protocol.TypeExBars {
				if err := writeFrame(server, h, nil); err != nil {
					return
				}
				continue
			}
			var data []byte
			if binary.LittleEndian.Uint32(b[14:18]) == 0 {
				data = extendedBars(uint8(binary.LittleEndian.Uint16(b[10:12])))
			} else {
				data = make([]byte, 20) // 空页仍返回完整计数头
			}
			if err := writeFrame(server, h, data); err != nil {
				return
			}
		}
	}()
	dial := func(context.Context) (ios.ReadWriteCloser, string, error) { return clientConn, "offline", nil }
	c, err := tdx.DialExWith(dial, tdx.WithRedial(false), tdx.WithLevel(tdx.LevelNone))
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

func TestHKFundMinuteRuleIsolation(t *testing.T) {
	c := startExtendedFixture(t)
	fundScale := hkFundMinuteScale(t)

	minute, err := c.GetKline(protocol.TypeKlineMinute2, "02800.49", 0, 10)
	if err != nil || len(minute.List) != 1 {
		t.Fatal(minute, err)
	}
	if got, want := minute.List[0].Volume, 2*fundScale; got != want {
		t.Fatalf("香港基金分钟量=%d, want %d", got, want)
	}
	if minute.List[0].Close != 10 {
		t.Fatalf("香港基金分钟价格=%v", minute.List[0].Close)
	}

	day, err := c.GetKline(protocol.TypeKlineDay2, "02800.49", 0, 10)
	if err != nil || len(day.List) != 1 {
		t.Fatal(day, err)
	}
	if day.List[0].Volume != 200 {
		t.Fatalf("香港基金日线量=%d, want 200（修改只作用于分钟规则）", day.List[0].Volume)
	}

	stock, err := c.GetKline(protocol.TypeKlineMinute2, "00700.31", 0, 10)
	if err != nil || len(stock.List) != 1 || stock.List[0].Volume != 200 {
		t.Fatalf("香港股票分钟量=%v, err=%v", stock, err)
	}

	us, err := c.GetKline(protocol.TypeKlineMinute2, "us.AAPL", 0, 10)
	if err != nil || len(us.List) != 1 || us.List[0].Volume != 2 {
		t.Fatalf("美股分钟量=%v, err=%v", us, err)
	}

	index, err := c.GetKline(protocol.TypeKlineMinute2, "hk.HSI", 0, 10)
	if err != nil || len(index.List) != 1 {
		t.Fatal(index, err)
	}
	if index.List[0].Volume != 0 {
		t.Fatalf("港股指数分钟量=%d, want 0", index.List[0].Volume)
	}
	if index.List[0].Amount == nil {
		t.Fatal("港股指数成交额为空")
	}
	if got := *index.List[0].Amount; math.Abs(got-1000) > 0.01 {
		t.Fatalf("港股指数成交额=%v, want 1000", got)
	}

	// 同一报文经 pull 适配器写入临时 SQLite，再按统一 MarketID+Code 回读。
	store, err := sqlite.NewPull(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	s, err := pull.New(&pull.Config{
		Store: store, ExPool: singlePool{c: c}, StartAt: "20260101", Minute: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	unit, ok := pull.Get(protocol.MarketHKFund.String())
	if !ok {
		t.Fatal("缺少香港基金 pull 适配器")
	}
	code := model.Code{Market: protocol.MarketHKFund, Code: "02800"}
	if err := unit.FetchMinute(s, code); err != nil {
		t.Fatal(err)
	}
	rows, err := s.QueryMinute(code, "", "")
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	if got, want := rows[0].Volume, 2*fundScale; got != want {
		t.Fatalf("SQLite 分钟量=%d, want %d", got, want)
	}
	if rows[0].Close != 10 || rows[0].Open != 10 {
		t.Fatalf("SQLite 价格字段=%+v", rows[0])
	}
}

type singlePool struct{ c tdx.Client }

func (p singlePool) DoContext(ctx context.Context, f func(tdx.Client) error) error {
	return f(tdx.WithContext(ctx, p.c))
}
