package extend

import (
	"context"
	"encoding/binary"
	"encoding/csv"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/codes"
	"github.com/injoyai/tdx/v2/extend/manage"
	"github.com/injoyai/tdx/v2/extend/workday"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

type tradeTestPool struct {
	tdx.ClientPool
	c tdx.Client
}

func (p tradeTestPool) Do(fn func(tdx.Client) error) error { return fn(p.c) }

type tradeTestCodes struct{ codes.Interface }

func (tradeTestCodes) GetName(string) string { return "测试股票" }

func TestPullTradePeriods(t *testing.T) {
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

	db, err := xorms.NewSQLite(filepath.Join(t.TempDir(), "workday.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Sync2(new(workday.WorkdayModel)); err != nil {
		t.Fatal(err)
	}
	for _, date := range []time.Time{
		time.Date(2025, 1, 2, 15, 0, 0, 0, protocol.Location()),
		time.Date(2025, 1, 3, 15, 0, 0, 0, protocol.Location()),
		tdx.IntegerDay(time.Now().In(protocol.Location())).Add(15 * time.Hour),
	} {
		if _, err := db.Insert(&workday.WorkdayModel{Unix: date.Unix(), Date: date.Format("20060102")}); err != nil {
			t.Fatal(err)
		}
	}
	w, err := workday.New(workday.WithDB(db), workday.WithClient(c), workday.WithSpec("@every 87600h"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	dir := t.TempDir()
	m := &manage.Manage{ClientPool: tradeTestPool{c: c}, Workday: w, Codes: tradeTestCodes{}}
	if err := NewPullTrade(dir).PullYear(context.Background(), m, 2025, "sz000001"); err != nil {
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
			if rows[1][1] != "09:30" || rows[1][8] != "1" || rows[2][1] != time.Date(2025, 1, 2, 9, 30+period, 0, 0, protocol.Location()).Format("15:04") {
				t.Fatalf("auction/session boundary changed: first=%v second=%v", rows[1], rows[2])
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
			if volume != 2 {
				t.Fatalf("volume=%d, want 2 lots", volume)
			}
			if amount != 2000 {
				t.Fatalf("amount=%v, want 2000 yuan", amount)
			}
		})
	}
}
