package extend

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/injoyai/tdx/protocol"
)

func TestReadDay(t *testing.T) {
	ks, err := ReadDay("D:\\软件\\通达信\\", "sz000001")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("日线数量: %d", len(ks))
	for _, v := range ks[len(ks)-5:] {
		t.Logf("%s 开:%.3f 高:%.3f 低:%.3f 收:%.3f 额:%.0f 量:%d手",
			v.Time.Format("2006-01-02"),
			v.Open.Float64(), v.High.Float64(), v.Low.Float64(), v.Close.Float64(),
			v.Amount.Float64(), v.Volume)
	}
}

func TestReadMinute(t *testing.T) {
	// 1分钟 .lc1
	ks, err := ReadMinute1("D:\\软件\\通达信\\", "sz000001")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("1分钟数量: %d", len(ks))
	for _, v := range ks[len(ks)-3:] {
		t.Logf("  %s 开:%.3f 高:%.3f 低:%.3f 收:%.3f 额:%.0f 量:%d手",
			v.Time.Format("2006-01-02 15:04"),
			v.Open.Float64(), v.High.Float64(), v.Low.Float64(), v.Close.Float64(),
			v.Amount.Float64(), v.Volume)
	}
	// 5分钟 .lc5
	ks5, err := ReadMinute5("D:\\软件\\通达信\\", "sz000001")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("5分钟数量: %d", len(ks5))
	for _, v := range ks5[len(ks5)-3:] {
		t.Logf("  %s 开:%.3f 高:%.3f 低:%.3f 收:%.3f 额:%.0f 量:%d手",
			v.Time.Format("2006-01-02 15:04"),
			v.Open.Float64(), v.High.Float64(), v.Low.Float64(), v.Close.Float64(),
			v.Amount.Float64(), v.Volume)
	}
}

func TestReadDayBJ(t *testing.T) {
	ks, err := ReadDay("D:\\软件\\通达信\\", "bj920000")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("北交所日线数量: %d", len(ks))
	if len(ks) > 0 {
		last := ks[len(ks)-1]
		t.Logf("  最新: %s 收:%.3f", last.Time.Format("2006-01-02"), last.Close.Float64())
	}
}

var _ = protocol.ExchangeSZ

// TestWriteMinutePlaceholder 验证 .lc1 生成时每个交易日最后一条真实数据(15:00)前补 14:59 占位记录,.lc5 不补
func TestWriteMinutePlaceholder(t *testing.T) {
	// 跨两个交易日,每个交易日含 9:31 与 15:00 两分钟数据
	mk := func(day int, h, m int, vol int64) *protocol.Kline {
		return &protocol.Kline{
			Open: protocol.Yuan(10.5), High: protocol.Yuan(11.0), Low: protocol.Yuan(10.0), Close: protocol.Yuan(10.8),
			Amount: 15534000, Volume: vol, Time: time.Date(2026, 7, day, h, m, 0, 0, time.Local),
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
			Open: protocol.Yuan(10.5), High: protocol.Yuan(11.0), Low: protocol.Yuan(10.0), Close: protocol.Yuan(10.8),
			Amount: protocol.Price(amt), Volume: vol, Time: time.Date(2026, 7, 8, h, m, 0, 0, time.Local),
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

// TestWriteDayVolume 验证日线成交量单位: 股票写 手×100=股,指数原样写手
func TestWriteDayVolume(t *testing.T) {
	k := &protocol.Kline{
		Open: protocol.Yuan(10.5), High: protocol.Yuan(11.0), Low: protocol.Yuan(10.0), Close: protocol.Yuan(10.8),
		Amount: 15534000, Volume: 13808, Time: time.Date(2026, 7, 8, 0, 0, 0, 0, time.Local),
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