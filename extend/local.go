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
//	            24~27 uint32 成交量(股)
//	            28~31 保留
//	.lc1/.lc5 分钟线(1分钟/5分钟): 每 32 字节一条
//	            00~01 uint16 日期: year=num/2048+2004 month=(num%2048)/100 day=(num%2048)%100
//	            02~03 uint16 当日分钟数(从0点起)
//	            04~19 4×float32 开盘/最高/最低/收盘(元)
//	            20~23 float32 成交额(元)
//	            24~27 uint32 成交量(股)
//	            28~31 保留

const (
	// MinuteType1 1分钟线,对应 minline/*.lc1
	MinuteType1 = 1
	// MinuteType5 5分钟线,对应 fzline/*.lc5
	MinuteType5 = 5
)

// ReadDay 读取本地日线数据,dir: 通达信安装目录,例如 C:/通达信/
func ReadDay(dir string, code string) (protocol.Klines, error) {
	ex, c, err := decodeCode(code)
	if err != nil {
		return nil, err
	}
	bs, err := readLocal(dir, ex, "lday", c+".day")
	if err != nil {
		return nil, err
	}
	ks := protocol.Klines{}
	for i := 0; i+32 <= len(bs); i += 32 {
		rec := bs[i : i+32]
		// 00~03 日期 YYYYMMDD
		d := binary.LittleEndian.Uint32(rec[0:4])
		t := time.Date(int(d/10000), time.Month(d/100%100), int(d%100), 0, 0, 0, 0, time.Local)
		ks = append(ks, &protocol.Kline{
			Open:   priceFromInt(binary.LittleEndian.Uint32(rec[4:8])),
			High:   priceFromInt(binary.LittleEndian.Uint32(rec[8:12])),
			Low:    priceFromInt(binary.LittleEndian.Uint32(rec[12:16])),
			Close:  priceFromInt(binary.LittleEndian.Uint32(rec[16:20])),
			Amount: protocol.Price(int64(math.Round(float64(math.Float32frombits(binary.LittleEndian.Uint32(rec[20:24]))) * 1000))), //元转厘
			Volume: int64(binary.LittleEndian.Uint32(rec[24:28]) / 100),                                                        //股转手
			Time:   t,
		})
	}
	return ks, nil
}

// ReadMinute 读取本地分钟线数据,dir: 通达信安装目录,type: MinuteType1(1分钟)/MinuteType5(5分钟)
func ReadMinute(dir string, code string, typ int) (protocol.Klines, error) {
	ex, c, err := decodeCode(code)
	if err != nil {
		return nil, err
	}
	sub, ext := "minline", ".lc1"
	if typ == MinuteType5 {
		// 5分钟线存放于 fzline 目录,扩展名 .lc5
		sub, ext = "fzline", ".lc5"
	}
	bs, err := readLocal(dir, ex, sub, c+ext)
	if err != nil {
		return nil, err
	}
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
		ks = append(ks, &protocol.Kline{
			Open:   protocol.Yuan(float64(math.Float32frombits(binary.LittleEndian.Uint32(rec[4:8])))),
			High:   protocol.Yuan(float64(math.Float32frombits(binary.LittleEndian.Uint32(rec[8:12])))),
			Low:    protocol.Yuan(float64(math.Float32frombits(binary.LittleEndian.Uint32(rec[12:16])))),
			Close:  protocol.Yuan(float64(math.Float32frombits(binary.LittleEndian.Uint32(rec[16:20])))),
			Amount: protocol.Price(int64(math.Round(float64(math.Float32frombits(binary.LittleEndian.Uint32(rec[20:24]))) * 1000))), //元转厘
			Volume: int64(binary.LittleEndian.Uint32(rec[24:28]) / 100),                                                           //股转手
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
