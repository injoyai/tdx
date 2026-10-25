package local

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestWriteDayFloatPrices(t *testing.T) {
	k := &protocol.Kline{
		Time: time.Date(2026, 7, 8, 0, 0, 0, 0, protocol.Location()),
		Open: 0.29, High: 10.056, Low: 0.01, Close: 10.05, Amount: 123.125, Volume: 2,
	}
	b, err := WriteDay("sz000001", protocol.Klines{k})
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []uint32{29, 1006, 1, 1005} {
		if got := binary.LittleEndian.Uint32(b[4+i*4:]); got != want {
			t.Errorf("price %d: %d, want %d", i, got, want)
		}
	}
	if got := math.Float32frombits(binary.LittleEndian.Uint32(b[20:])); got != 123.125 {
		t.Fatal("amount unit", got)
	}
}

func TestWritersRejectUnrepresentablePrices(t *testing.T) {
	for _, write := range []func(string, protocol.Klines) ([]byte, error){WriteDay, WriteMinute1, WriteMinute5} {
		for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), math.MaxFloat64} {
			for _, k := range (protocol.Klines{{Close: bad}, {Amount: bad}}) {
				if b, err := write("sz000001", protocol.Klines{k}); err == nil || b != nil {
					t.Fatal("accepted invalid float", bad, err)
				}
			}
		}
	}
	for _, bad := range []float64{-1, float64(math.MaxUint32)/100 + 0.01} {
		if _, err := WriteDay("sz000001", protocol.Klines{{Close: bad}}); err == nil {
			t.Fatal("accepted out-of-range integer price", bad)
		}
	}
}

func writeLocalFixture(t *testing.T, dir, code, sub, ext string, data []byte) {
	t.Helper()
	filename := filepath.Join(dir, "vipdoc", code[:2], sub, code+ext)
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadDay(t *testing.T) {
	for _, code := range []string{"sz000001", "bj920000", "sh000001"} {
		t.Run(code, func(t *testing.T) {
			dir := t.TempDir()
			data := make([]byte, 32)
			binary.LittleEndian.PutUint32(data[0:4], 20260708)
			for i, price := range []uint32{1050, 1100, 1000, 1080} {
				binary.LittleEndian.PutUint32(data[4+i*4:8+i*4], price)
			}
			binary.LittleEndian.PutUint32(data[20:24], math.Float32bits(216000))
			volume := uint32(20000)
			if code == "sh000001" {
				volume = 200
			}
			binary.LittleEndian.PutUint32(data[24:28], volume)
			writeLocalFixture(t, dir, code, "lday", ".day", data)
			ks, err := ReadDay(dir, code)
			if err != nil {
				t.Fatal(err)
			}
			if len(ks) != 1 {
				t.Fatalf("records=%d, want 1", len(ks))
			}
			k := ks[0]
			if k.Time.Format("20060102") != "20260708" || k.Open != 10.5 ||
				k.High != 11 || k.Low != 10 || k.Close != 10.8 ||
				k.Volume != 200 || k.Amount != 216000 {
				t.Fatalf("unexpected daily record: %+v", k)
			}
		})
	}
}

func TestReadMinute(t *testing.T) {
	for _, code := range []string{"sz000001", "sh000001"} {
		for _, period := range []int{1, 5} {
			t.Run(code+"/"+strconv.Itoa(period), func(t *testing.T) {
				dir := t.TempDir()
				data := make([]byte, 64)
				date := uint16((2026-2004)*2048 + 7*100 + 8)
				for i, minute := range []uint16{571, 899} {
					rec := data[i*32 : (i+1)*32]
					binary.LittleEndian.PutUint16(rec[0:2], date)
					binary.LittleEndian.PutUint16(rec[2:4], minute)
					for j, price := range []float32{10.5, 11, 10, 10.8} {
						binary.LittleEndian.PutUint32(rec[4+j*4:8+j*4], math.Float32bits(price))
					}
				}
				binary.LittleEndian.PutUint32(data[20:24], math.Float32bits(216000))
				volume := uint32(20000)
				if code == "sh000001" {
					volume = 200
				}
				binary.LittleEndian.PutUint32(data[24:28], volume)
				sub, ext := "minline", ".lc1"
				if period == 5 {
					sub, ext = "fzline", ".lc5"
				}
				writeLocalFixture(t, dir, code, sub, ext, data)
				read := ReadMinute1
				if period == 5 {
					read = ReadMinute5
				}
				ks, err := read(dir, code)
				if err != nil {
					t.Fatal(err)
				}
				want := 1
				if period == 5 {
					want = 2
				}
				if len(ks) != want {
					t.Fatalf("records=%d, want %d", len(ks), want)
				}
				k := ks[0]
				if k.Time.Format("2006010215:04") != "2026070809:31" ||
					k.Close != float64(float32(10.8)) || k.Volume != 200 || k.Amount != 216000 {
					t.Fatalf("unexpected minute record: %+v", k)
				}
			})
		}
	}
}

// TestWriteMinutePlaceholder 验证 .lc1 生成时每个交易日最后一条真实数据(15:00)前补 14:59 占位记录,.lc5 不补
func TestWriteMinutePlaceholder(t *testing.T) {
	// 跨两个交易日,每个交易日含 9:31 与 15:00 两分钟数据
	mk := func(day int, h, m int, vol int64) *protocol.Kline {
		return &protocol.Kline{
			Open: 10.5, High: 11.0, Low: 10.0, Close: 10.8,
			Amount: 15534, Volume: vol, Time: time.Date(2026, 7, day, h, m, 0, 0, time.Local),
		}
	}
	ks := protocol.Klines{
		mk(8, 9, 31, 13808), mk(8, 15, 0, 86100),
		mk(9, 9, 31, 13808), mk(9, 15, 0, 86100),
	}

	// .lc1: 每个交易日补 1 条占位,共 2 个交易日 → 6 条记录(4数据+2占位)
	bs1, err := WriteMinute1("sz000001", ks)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs1)/32 != 6 {
		t.Fatalf(".lc1 记录数 = %d, want 6(4数据+2占位)", len(bs1)/32)
	}
	// 记录顺序应为: 9:31, 14:59占位, 15:00, 9:31, 14:59占位, 15:00
	// 即第2、5条(索引1、4)是占位(量=0额=0,分钟=899)
	for idx := 1; idx < 6; idx += 3 {
		rec := bs1[idx*32 : idx*32+32]
		if m := binary.LittleEndian.Uint16(rec[2:4]); m != 899 {
			t.Fatalf(".lc1 占位[%d] 分钟 = %d, want 899(14:59)", idx, m)
		}
		if binary.LittleEndian.Uint32(rec[24:28]) != 0 || math.Float32frombits(binary.LittleEndian.Uint32(rec[20:24])) != 0 {
			t.Fatalf(".lc1 占位[%d] 应为量=0额=0: 量=%d 额=%v", idx, binary.LittleEndian.Uint32(rec[24:28]), math.Float32frombits(binary.LittleEndian.Uint32(rec[20:24])))
		}
	}
	// 非占位记录(索引0)应保留真实数据
	if m := binary.LittleEndian.Uint16(bs1[2:4]); m != 571 {
		t.Fatalf(".lc1 首条 分钟 = %d, want 571(9:31)", m)
	}

	// .lc5: 不补占位,共 4 条记录
	bs5, err := WriteMinute5("sz000001", ks)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs5)/32 != 4 {
		t.Fatalf(".lc5 记录数 = %d, want 4(无占位)", len(bs5)/32)
	}
}

// TestWriteMinutePlaceholderDedup 验证: 数据本身已含尾盘占位(14:59 量=0额=0)时,生成 .lc1 不重复补占位
func TestWriteMinutePlaceholderDedup(t *testing.T) {
	// 模拟实时拉取数据: 14:58真实, 14:59占位(量=0额=0), 15:00收盘 —— 已含尾盘占位
	mk := func(h, m int, vol int64, amt int64) *protocol.Kline {
		return &protocol.Kline{
			Open: 10.5, High: 11.0, Low: 10.0, Close: 10.8,
			Amount: float64(amt) / 1000, Volume: vol, Time: time.Date(2026, 7, 8, h, m, 0, 0, time.Local),
		}
	}
	ks := protocol.Klines{
		mk(9, 31, 13808, 15534000),
		mk(14, 58, 900, 1000000),
		mk(14, 59, 0, 0), // 已含占位
		mk(15, 0, 86100, 9990000),
	}
	bs1, err := WriteMinute1("sz000001", ks)
	if err != nil {
		t.Fatal(err)
	}
	// 数据已含占位,不应重复补 → 仍为 4 条
	if len(bs1)/32 != 4 {
		t.Fatalf(".lc1 记录数 = %d, want 4(数据已含占位不重复补)", len(bs1)/32)
	}
}

func TestWriteMinutePartialDay(t *testing.T) {
	day := time.Date(2026, 10, 12, 0, 0, 0, 0, time.Local)
	ks := protocol.Klines{
		&protocol.Kline{Time: day.Add(9*time.Hour + 31*time.Minute), Close: 10, Volume: 1, Amount: 10},
		&protocol.Kline{Time: day.Add(9*time.Hour + 32*time.Minute), Close: 10, Volume: 1, Amount: 10},
	}
	bs, err := WriteMinute1("sz000001", ks)
	if err != nil || len(bs) != 64 {
		t.Fatal("partial day gained a tail placeholder", len(bs), err)
	}
	for i, want := range []uint16{571, 572} {
		if got := binary.LittleEndian.Uint16(bs[i*32+2 : i*32+4]); got != want {
			t.Fatal("minute order", got, want)
		}
	}
}

// TestWriteDayVolume 验证日线成交量单位: 股票写 手×100=股,指数原样写手
func TestWriteDayVolume(t *testing.T) {
	k := &protocol.Kline{
		Open: 10.5, High: 11.0, Low: 10.0, Close: 10.8,
		Amount: 15534, Volume: 13808, Time: time.Date(2026, 7, 8, 0, 0, 0, 0, time.Local),
	}
	ks := protocol.Klines{k}
	// 股票
	bs, err := WriteDay("sz000001", ks)
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 32 {
		t.Fatalf("日线长度 = %d, want 32", len(bs))
	}
	if v := binary.LittleEndian.Uint32(bs[24:28]); v != 1380800 {
		t.Fatalf("股票成交量 = %d, want 1380800(手×100)", v)
	}
	// 指数: 原样写手
	bs2, err := WriteDay("sh000001", ks)
	if err != nil {
		t.Fatal(err)
	}
	if v := binary.LittleEndian.Uint32(bs2[24:28]); v != 13808 {
		t.Fatalf("指数成交量 = %d, want 13808(原样手)", v)
	}
}
