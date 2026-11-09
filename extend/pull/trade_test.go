package pull

import (
	"context"
	"encoding/binary"
	"encoding/csv"
	"errors"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/metadata"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

type tradeTestPool struct {
	tdx.ClientPool
	c tdx.Client
}

type tradeYears struct{ years []int }

func (c *tradeYears) RangeYear(year int, _ func(time.Time) bool) { c.years = append(c.years, year) }

func TestTradeExplicitYearRange(t *testing.T) {
	c := &tradeYears{}
	p := NewTrade(t.TempDir())
	p.Pool, p.Workday = tradeTestPool{}, c
	if err := p.Pull(context.Background(), 2024, 2025, "sz000001"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.years, []int{2024, 2025}) {
		t.Fatal("implicit years were downloaded", c.years)
	}
	for _, years := range [][2]int{{0, 2025}, {2025, 2024}, {2025, 10000}} {
		if err := p.Pull(context.Background(), years[0], years[1], "sz000001"); err == nil {
			t.Fatal("invalid year range accepted", years)
		}
	}
	if err := p.Pull(nil, 2024, 2025, "sz000001"); err == nil {
		t.Fatal("nil context accepted")
	}
}

type cancelTradePool struct{ entered chan struct{} }

func (p *cancelTradePool) DoContext(ctx context.Context, _ func(tdx.Client) error) error {
	close(p.entered)
	<-ctx.Done()
	return ctx.Err()
}

type tradeDay struct{}

func (tradeDay) RangeYear(year int, fn func(time.Time) bool) {
	fn(time.Date(year, 1, 2, 15, 0, 0, 0, protocol.Location()))
}

func TestTradeCancellationBeforeBorrowDoesNotExport(t *testing.T) {
	p := NewTrade(filepath.Join(t.TempDir(), "export"))
	waiting := &cancelTradePool{entered: make(chan struct{})}
	p.Pool, p.Workday = waiting, tradeDay{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- p.PullYear(ctx, 2025, "sz000001") }()
	select {
	case <-waiting.entered:
	case <-time.After(time.Second):
		t.Fatal("trade did not wait for connection")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("trade ignored cancellation")
	}
	if _, err := os.Stat(p.Dir); !os.IsNotExist(err) {
		t.Fatal("canceled trade created export files", err)
	}
}

func (p tradeTestPool) DoContext(ctx context.Context, fn func(tdx.Client) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(p.c)
}

type tradeTestCodes struct{}

func (tradeTestCodes) GetName(string) string { return "测试股票" }

func TestTradeMissingDependencies(t *testing.T) {
	downloader := NewTrade(t.TempDir())
	if err := downloader.PullYear(context.Background(), 2025, "sz000001"); err == nil || !strings.Contains(err.Error(), "Pool") {
		t.Fatal("missing pool", err)
	}
	downloader.Pool = tradeTestPool{}
	if err := downloader.PullYear(context.Background(), 2025, "sz000001"); err == nil || !strings.Contains(err.Error(), "Workday") {
		t.Fatal("missing calendar", err)
	}
}

func TestTradePeriods(t *testing.T) {
	var c tdx.Client
	reader, server := net.Pipe()
	t.Cleanup(func() { server.Close() })
	go func() {
		for {
			header := make([]byte, 12)
			if _, err := io.ReadFull(server, header); err != nil {
				return
			}
			body := make([]byte, int(binary.LittleEndian.Uint16(header[6:8]))-2)
			if _, err := io.ReadFull(server, body); err != nil {
				return
			}
			typ := binary.LittleEndian.Uint16(header[10:12])
			if typ != protocol.TypeHistoryTrade && typ != protocol.TypeKline {
				continue
			}
			// 每日一笔：09:29，10 元，1 手；响应包含记录数及四字节保留位。
			data := []byte{1, 0, 0, 0, 0, 0, 0x39, 0x02, 0xa8, 0x0f, 1, 0, 0}
			if typ == protocol.TypeKline {
				data = protocol.Bytes(uint16(2))
				for _, day := range []uint32{20250102, 20250103} {
					data = append(data, protocol.Bytes(day)...)
					price := []byte{0}
					if day == 20250102 {
						price = []byte{0x90, 0x9c, 1}
					}
					data = append(data, price...)
					data = append(data, 0, 0, 0)
					data = append(data, protocol.Bytes(math.Float32bits(1))...)
					data = append(data, protocol.Bytes(math.Float32bits(1000))...)
				}
			}
			resp := make([]byte, 16+len(data))
			copy(resp, []byte{0xb1, 0xcb, 0x74, 0, 0x1c})
			copy(resp[5:9], header[1:5])
			copy(resp[10:12], header[10:12])
			binary.LittleEndian.PutUint16(resp[12:14], uint16(len(data)))
			binary.LittleEndian.PutUint16(resp[14:16], uint16(len(data)))
			copy(resp[16:], data)
			if _, err := server.Write(resp); err != nil {
				return
			}
		}
	}()
	var err error
	c, err = tdx.DialWith(func(context.Context) (ios.ReadWriteCloser, string, error) {
		return reader, "test", nil
	}, tdx.WithRedial(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	c.SetTimeout(time.Second)

	now := time.Now().In(protocol.Location())
	snapshot := model.WorkdaySnapshot{Market: protocol.MarketAStock, Source: "fixture", From: "2025-01-02", Through: now.Format(time.DateOnly)}
	for _, date := range []time.Time{
		time.Date(2025, 1, 2, 15, 0, 0, 0, protocol.Location()),
		time.Date(2025, 1, 3, 15, 0, 0, 0, protocol.Location()),
		time.Date(now.Year(), now.Month(), now.Day(), 15, 0, 0, 0, now.Location()),
	} {
		snapshot.Dates = append(snapshot.Dates, date.Format(time.DateOnly))
	}
	w, err := metadata.NewWorkday(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	downloader := NewTrade(dir)
	downloader.Pool = tradeTestPool{c: c}
	downloader.Workday = w
	downloader.CodeName = (tradeTestCodes{}).GetName
	if err := downloader.PullYear(context.Background(), 2025, "sz000001"); err != nil {
		t.Fatal(err)
	}
	for _, period := range []int{1, 5, 15, 30, 60} {
		t.Run(strconv.Itoa(period), func(t *testing.T) {
			f, err := os.Open(filepath.Join(dir, strconv.Itoa(period)+"分钟", "sz000001-2025.csv"))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			rows, err := csv.NewReader(f).ReadAll()
			if err != nil {
				t.Fatal(err)
			}
			if got, want := len(rows)-1, 2*((241+period-1)/period); got != want {
				t.Fatalf("data rows=%d, want %d", got, want)
			}
			if rows[1][1] != "09:30" || rows[1][8] != "100" || rows[2][1] != time.Date(2025, 1, 2, 9, 30+period, 0, 0, protocol.Location()).Format("15:04") {
				t.Fatalf("auction/session boundary changed: first=%v second=%v", rows[1], rows[2])
			}
			for i := 2; i < len(rows); i++ {
				if rows[i][0]+rows[i][1] < rows[i-1][0]+rows[i-1][1] {
					t.Fatal("CSV rows out of source-time order")
				}
			}
			volume := 0
			amount := 0.0
			for _, row := range rows[1:] {
				v, err := strconv.Atoi(row[8])
				if err != nil {
					t.Fatal(err)
				}
				volume += v
				if row[7] != "10" {
					t.Fatalf("close=%s, want 10 yuan", row[7])
				}
				a, err := strconv.ParseFloat(row[9], 64)
				if err != nil {
					t.Fatal(err)
				}
				amount += a
			}
			if volume != 200 {
				t.Fatalf("volume=%d, want 200 shares", volume)
			}
			if amount != 2000 {
				t.Fatalf("amount=%v, want 2000 yuan", amount)
			}
		})
	}
}
