package extend

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/injoyai/tdx/protocol"
)

// 通达信本地数据文件解析
// 官方协议参考 pytdx(TdxDailyBarReader/TdxLCMinBarReader) 与 mootdx:
//
//	.dir 目录:  vipdoc/<sh|sz|bj>/lday|minline|fzline
//	.day 日线:  每 32 字节一条
//	            00~03 uint32 日期(YYYYMMDD)
//	            04~19 4×uint32 开盘/最高/最低/收盘(价格×100, 整数)
//	            20~23 float32 成交额(元)
//	            24~27 uint32 成交量(股/手, 见下方单位说明)
//	            28~31 保留
//	.lc1/.lc5 分钟线(1分钟/5分钟): 每 32 字节一条
//	            00~01 uint16 日期: year=num/2048+2004 month=(num%2048)/100 day=(num%2048)%100
//	            02~03 uint16 当日分钟数(从0点起)
//	            04~19 4×float32 开盘/最高/最低/收盘(元)
//	            20~23 float32 成交额(元)
//	            24~27 uint32 成交量(股/手, 见下方单位说明)
//	            28~31 保留
//
//	成交量单位: 指数(.day/.lc1/.lc5)的成交量字段单位是"手", 原值即手;
//	            股票为"股", 需 ÷100 转为"手"。协议单位统一为"手"。

// ReadDay 读取本地日线数据,dir: 通达信安装目录,例如 C:/通达信/
// 注意: 指数与股票的成交量单位不同 —— 股票 .day 的成交量字段为"股",指数为"手",
// 此处统一转换为协议单位"手"(指数原值即手,股票÷100)。
func ReadDay(dir string, code string) (protocol.Klines, error) {
	ex, c, err := decodeCode(code)
	if err != nil {
		return nil, err
	}
	bs, err := readLocal(dir, ex, "lday", c+".day")
	if err != nil {
		return nil, err
	}
	// decodeCode 返回的 c 已带交易所前缀(如 sh000001),直接用于判断指数
	index := protocol.IsIndex(c)
	ks := protocol.Klines{}
	for i := 0; i+32 <= len(bs); i += 32 {
		rec := bs[i : i+32]
		// 00~03 日期 YYYYMMDD
		d := binary.LittleEndian.Uint32(rec[0:4])
		t := time.Date(int(d/10000), time.Month(d/100%100), int(d%100), 0, 0, 0, 0, time.Local)
		vol := int64(binary.LittleEndian.Uint32(rec[24:28]))
		if !index {
			vol /= 100 //股票成交量单位"股"转"手"
		}
		ks = append(ks, &protocol.Kline{
			Open:   priceFromInt(binary.LittleEndian.Uint32(rec[4:8])),
			High:   priceFromInt(binary.LittleEndian.Uint32(rec[8:12])),
			Low:    priceFromInt(binary.LittleEndian.Uint32(rec[12:16])),
			Close:  priceFromInt(binary.LittleEndian.Uint32(rec[16:20])),
			Amount: protocol.Price(int64(math.Round(float64(math.Float32frombits(binary.LittleEndian.Uint32(rec[20:24]))) * 1000))), //元转厘
			Volume: vol,
			Time:   t,
		})
	}
	return ks, nil
}

// ReadMinute1 读取本地 1 分钟线数据(.lc1),dir: 通达信安装目录,例如 C:/通达信/
// 注意: 分钟线成交量单位与日线一致 —— 股票为"股",指数为"手",统一转为协议单位"手"。
// .lc1 每个交易日末尾(14:59,偶见14:58)有一条"量=0额=0"的尾盘占位记录,此处跳过。
func ReadMinute1(dir string, code string) (protocol.Klines, error) {
	return readMinute(dir, code, "minline", ".lc1", true)
}

// ReadMinute5 读取本地 5 分钟线数据(.lc5),dir: 通达信安装目录,例如 C:/通达信/
// 注意: 分钟线成交量单位与日线一致 —— 股票为"股",指数为"手",统一转为协议单位"手"。
// .lc5 无尾盘占位记录,所有记录原样读取。
func ReadMinute5(dir string, code string) (protocol.Klines, error) {
	return readMinute(dir, code, "fzline", ".lc5", false)
}

// readMinute 分钟线读取通用实现,skipTailPlaceholder: 是否跳过 .lc1 尾盘占位
func readMinute(dir string, code string, sub, ext string, skipTailPlaceholder bool) (protocol.Klines, error) {
	ex, c, err := decodeCode(code)
	if err != nil {
		return nil, err
	}
	bs, err := readLocal(dir, ex, sub, c+ext)
	if err != nil {
		return nil, err
	}
	// decodeCode 返回的 c 已带交易所前缀(如 sh000001),直接用于判断指数
	index := protocol.IsIndex(c)
	ks := protocol.Klines{}
	for i := 0; i+32 <= len(bs); i += 32 {
		rec := bs[i : i+32]
		// 00~01 日期
		d := binary.LittleEndian.Uint16(rec[0:2])
		year, month, day := int(d/2048)+2004, int(d%2048/100), int(d%2048%100)
		// 02~03 当日分钟数
		m := binary.LittleEndian.Uint16(rec[2:4])
		hour, minute := int(m/60), int(m%60)
		t := time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.Local)
		amount := math.Float32frombits(binary.LittleEndian.Uint32(rec[20:24]))
		vol := int64(binary.LittleEndian.Uint32(rec[24:28]))
		if !index {
			vol /= 100 //股票成交量单位"股"转"手"
		}
		// 1分钟线(.lc1)每个交易日 14:59(偶见14:58)有一条"量=0额=0"的占位记录,
		// 尾盘无真实成交,这里跳过;5分钟线(.lc5)无占位,全保留。
		// 只跳过尾盘(>=14:58)的量额全零,避免误删盘中真实无成交分钟。
		if skipTailPlaceholder && int(m) >= 898 && amount == 0 && vol == 0 {
			continue
		}
		ks = append(ks, &protocol.Kline{
			Open:   protocol.Yuan(float64(math.Float32frombits(binary.LittleEndian.Uint32(rec[4:8])))),
			High:   protocol.Yuan(float64(math.Float32frombits(binary.LittleEndian.Uint32(rec[8:12])))),
			Low:    protocol.Yuan(float64(math.Float32frombits(binary.LittleEndian.Uint32(rec[12:16])))),
			Close:  protocol.Yuan(float64(math.Float32frombits(binary.LittleEndian.Uint32(rec[16:20])))),
			Amount: protocol.Price(int64(math.Round(float64(amount) * 1000))), //元转厘
			Volume: vol,
			Time:   t,
		})
	}
	return ks, nil
}

// decodeCode 解析股票代码,带交易所前缀(如 sz000001),返回交易所和带前缀的完整代码
func decodeCode(code string) (protocol.Exchange, string, error) {
	ex, c, err := protocol.DecodeCode(code)
	if err != nil {
		return 0, "", err
	}
	return ex, ex.String() + c, nil
}

// readLocal 读取本地数据文件
func readLocal(dir string, ex protocol.Exchange, sub, name string) ([]byte, error) {
	filename := filepath.Join(dir, "vipdoc", ex.String(), sub, name)
	bs, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("读取本地文件失败 %s: %w", filename, err)
	}
	return bs, nil
}

// priceFromInt 将官方日线价格(整数,单位0.01元)转为 Price(单位厘)
func priceFromInt(v uint32) protocol.Price {
	return protocol.Price(v) * 10
}

//****//

// 通达信本地数据文件写入(生成),与 ReadDay/ReadMinute 的格式完全对称。
// 自定义写入位置,不与通达信客户端关联。
// 格式见 local.go 顶部注释:
//
//	.day 日线:  每 32 字节一条
//	            00~03 uint32 日期(YYYYMMDD)
//	            04~19 4×uint32 开盘/最高/最低/收盘(价格×100, 整数)
//	            20~23 float32 成交额(元)
//	            24~27 uint32 成交量(股/手, 指数为手)
//	            28~31 保留
//	.lc1/.lc5 分钟线(1分钟/5分钟): 每 32 字节一条
//	            00~01 uint16 日期: year=num/2048+2004 month=(num%2048)/100 day=(num%2048)%100
//	            02~03 uint16 当日分钟数(从0点起)
//	            04~19 4×float32 开盘/最高/最低/收盘(元)
//	            20~23 float32 成交额(元)
//	            24~27 uint32 成交量(股/手, 指数为手)
//	            28~31 保留

// WriteDay 将日线 K 线编码为通达信 .day 格式字节流(不落盘,由调用方自由处理)。
// code 需带交易所前缀(如 sz000001),用于判断指数与股票(成交量单位不同):
// 股票 .day 存"股",指数存"手"。Kline.Volume 为"手",此处换算: 股票写 手×100=股,指数原样写手。
func WriteDay(code string, ks protocol.Klines) ([]byte, error) {
	_, c, err := decodeCode(code)
	if err != nil {
		return nil, err
	}
	// decodeCode 返回的 c 已带交易所前缀(如 sh000001),直接用于判断指数
	index := protocol.IsIndex(c)
	bs := make([]byte, 0, len(ks)*32)
	for _, k := range ks {
		bs = append(bs, encodeDay(k, index)...)
	}
	return bs, nil
}

// WriteMinute1 将 1 分钟 K 线编码为通达信 .lc1 格式字节流(不落盘,由调用方自由处理)。
// code 需带交易所前缀(如 sz000001)。
// 成交量单位处理与 WriteDay 一致(指数为手,股票×100转股)。
// 注意: 与真实通达信 .lc1 一致,每个交易日末尾(最后一条真实数据 15:00 之前)会补一条
// "量=0额=0"的 14:59 占位记录;若数据本身已含尾盘占位则不再重复补(否则会多一条)。
func WriteMinute1(code string, ks protocol.Klines) ([]byte, error) {
	return writeMinute(code, true, ks)
}

// WriteMinute5 将 5 分钟 K 线编码为通达信 .lc5 格式字节流(不落盘,由调用方自由处理)。
// code 需带交易所前缀(如 sz000001)。
// 成交量单位处理与 WriteDay 一致(指数为手,股票×100转股)。
// 注意: 5 分钟 .lc5 无尾盘占位记录,所有记录原样编码。
func WriteMinute5(code string, ks protocol.Klines) ([]byte, error) {
	return writeMinute(code, false, ks)
}

// writeMinute 分钟线编码通用实现,placeholder: 是否补 .lc1 尾盘占位
func writeMinute(code string, placeholder bool, ks protocol.Klines) ([]byte, error) {
	_, c, err := decodeCode(code)
	if err != nil {
		return nil, err
	}
	// decodeCode 返回的 c 已带交易所前缀(如 sh000001),直接用于判断指数
	index := protocol.IsIndex(c)
	bs := make([]byte, 0, len(ks)*32)
	// 按交易日分组处理,便于判断每个交易日是否已含尾盘占位
	for start := 0; start < len(ks); {
		end := start + 1
		day := ks[start].Time.Format("20060102")
		for end < len(ks) && ks[end].Time.Format("20060102") == day {
			end++
		}
		dayKs := ks[start:end]
		// 是否需补占位: 仅 1 分钟线,且该交易日数据中尚未含尾盘占位(14:58/14:59 且 量=0额=0)
		needPlaceholder := placeholder && !dayHasTailPlaceholder(dayKs)
		for j, k := range dayKs {
			// 占位记录插在当日最后一条真实数据(15:00)之前
			if needPlaceholder && j == len(dayKs)-1 {
				bs = append(bs, encodeMinutePlaceholder(dayKs[j])...)
			}
			bs = append(bs, encodeMinute(k, index)...)
		}
		start = end
	}
	return bs, nil
}

// dayHasTailPlaceholder 判断该交易日数据是否已含尾盘占位记录(当日分钟>=898 即 14:58 及之后 且 量=0额=0)
func dayHasTailPlaceholder(dayKs protocol.Klines) bool {
	for _, k := range dayKs {
		m := k.Time.Hour()*60 + k.Time.Minute()
		if m >= 898 && k.Amount == 0 && k.Volume == 0 {
			return true
		}
	}
	return false
}

// encodeMinutePlaceholder 编码一条分钟线占位记录(量=0额=0,时间取该日末分钟 14:59,
// 价格沿用该日最后一条真实数据的收盘价),与真实通达信 .lc1 每个交易日末尾的占位记录一致。
func encodeMinutePlaceholder(k *protocol.Kline) []byte {
	b := make([]byte, 32)
	// 日期 2048 进制: (year-2004)*2048 + month*100 + day
	num := uint16((k.Time.Year()-2004)*2048 + int(k.Time.Month())*100 + k.Time.Day())
	binary.LittleEndian.PutUint16(b[0:2], num)
	// 当日分钟数固定为当日末分钟(14:59=899),与真实文件一致
	binary.LittleEndian.PutUint16(b[2:4], 899)
	// 价格沿用该日最后收盘价(通达信占位记录价格通常为当日最后成交价)
	close := float32(k.Close.Float64())
	binary.LittleEndian.PutUint32(b[4:8], math.Float32bits(close))
	binary.LittleEndian.PutUint32(b[8:12], math.Float32bits(close))
	binary.LittleEndian.PutUint32(b[12:16], math.Float32bits(close))
	binary.LittleEndian.PutUint32(b[16:20], math.Float32bits(close))
	// 成交额=0, 成交量=0
	return b
}

// encodeDay 编码一条日线记录为 32 字节,index 表示是否指数(成交量单位不同)
func encodeDay(k *protocol.Kline, index bool) []byte {
	b := make([]byte, 32)
	d := uint32(k.Time.Year()*10000 + int(k.Time.Month())*100 + k.Time.Day())
	binary.LittleEndian.PutUint32(b[0:4], d)
	binary.LittleEndian.PutUint32(b[4:8], uint32(k.Open/10)) //Price厘→0.01元
	binary.LittleEndian.PutUint32(b[8:12], uint32(k.High/10))
	binary.LittleEndian.PutUint32(b[12:16], uint32(k.Low/10))
	binary.LittleEndian.PutUint32(b[16:20], uint32(k.Close/10))
	binary.LittleEndian.PutUint32(b[20:24], math.Float32bits(float32(k.Amount.Float64()))) //厘→元
	binary.LittleEndian.PutUint32(b[24:28], toFileVolume(k.Volume, index))                 //手→股(指数手)
	return b
}

// encodeMinute 编码一条分钟线记录为 32 字节,index 表示是否指数(成交量单位不同)
func encodeMinute(k *protocol.Kline, index bool) []byte {
	b := make([]byte, 32)
	// 日期 2048 进制: (year-2004)*2048 + month*100 + day
	num := uint16((k.Time.Year()-2004)*2048 + int(k.Time.Month())*100 + k.Time.Day())
	binary.LittleEndian.PutUint16(b[0:2], num)
	// 当日分钟数
	binary.LittleEndian.PutUint16(b[2:4], uint16(k.Time.Hour()*60+k.Time.Minute()))
	// 价格(元)
	binary.LittleEndian.PutUint32(b[4:8], math.Float32bits(float32(k.Open.Float64())))
	binary.LittleEndian.PutUint32(b[8:12], math.Float32bits(float32(k.High.Float64())))
	binary.LittleEndian.PutUint32(b[12:16], math.Float32bits(float32(k.Low.Float64())))
	binary.LittleEndian.PutUint32(b[16:20], math.Float32bits(float32(k.Close.Float64())))
	// 成交额(元)
	binary.LittleEndian.PutUint32(b[20:24], math.Float32bits(float32(k.Amount.Float64())))
	// 成交量(股, 指数手)
	binary.LittleEndian.PutUint32(b[24:28], toFileVolume(k.Volume, index))
	return b
}

// toFileVolume 将协议成交量(手)转为文件存储值: 股票→股(×100), 指数→手(原值)
func toFileVolume(vol int64, index bool) uint32 {
	if index {
		return uint32(vol)
	}
	return uint32(vol * 100)
}
