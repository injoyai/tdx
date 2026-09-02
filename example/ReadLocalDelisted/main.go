// 读取通达信客户端本地缓存中的退市股日线数据(实测: T0002/ds_cache/100#T600001.~~~day)
//
// 背景:
//   退市股(如 600001)在行情服务器上查不到(7709 标准行情 / 7727 扩展行情均无数据),
//   但通达信客户端通过【功能→定制品种→退市品种管理】绑定后,
//   会将其历史日线缓存在本地 <安装目录>/T0002/ds_cache 目录,
//   文件命名为 "<市场标识>#<代码>.~~~day", 如 100#T600001.~~~day
//   (100# 为缓存命名空间/市场标识, T600001 为退市代码, ~~~day 为日线缓存后缀)。
//
// 文件格式(与标准 .day 不同, 价格用 float32 存"元"):
//   头部 14 字节: [0:10] 保留, [10:14] uint32 记录数(LE)
//   之后每条记录 32 字节:
//     [0:4]  uint32  日期 YYYYMMDD
//     [4:8]  float32 开盘(元)
//     [8:12] float32 最高(元)
//     [12:16]float32 最低(元)
//     [16:20]float32 收盘(元)
//     [20:24]float32 成交额(元)
//     [24:28]uint32  成交量(股)
//     [28:32]保留
//
// 运行: go run ./example/ReadLocalDelisted [通达信安装目录]
//   默认目录: D:/软件/通达信
package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"
)

// rec 一条退市股缓存日线记录
type rec struct {
	Date                   time.Time
	Open, High, Low, Close float64
	Amount                 float64 // 成交额(元)
	Volume                 uint32  // 成交量(股)
}

// parse 解析 T0002/ds_cache 下的退市股日线缓存文件
func parse(file string) ([]rec, error) {
	bs, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	if len(bs) < 14 {
		return nil, fmt.Errorf("文件过短: %d 字节", len(bs))
	}
	n := int(binary.LittleEndian.Uint32(bs[10:14]))
	if 14+n*32 > len(bs) {
		return nil, fmt.Errorf("记录数 %d 超出文件大小 %d", n, len(bs))
	}
	out := make([]rec, 0, n)
	for i := 0; i < n; i++ {
		r := bs[14+i*32 : 14+(i+1)*32]
		d := binary.LittleEndian.Uint32(r[0:4])
		out = append(out, rec{
			Date:   time.Date(int(d/10000), time.Month(d/100%100), int(d%100), 0, 0, 0, 0, time.Local),
			Open:   float64(math.Float32frombits(binary.LittleEndian.Uint32(r[4:8]))),
			High:   float64(math.Float32frombits(binary.LittleEndian.Uint32(r[8:12]))),
			Low:    float64(math.Float32frombits(binary.LittleEndian.Uint32(r[12:16]))),
			Close:  float64(math.Float32frombits(binary.LittleEndian.Uint32(r[16:20]))),
			Amount: float64(math.Float32frombits(binary.LittleEndian.Uint32(r[20:24]))),
			Volume: binary.LittleEndian.Uint32(r[24:28]),
		})
	}
	return out, nil
}

func main() {
	dir := "D:/软件/通达信"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	cache := filepath.Join(dir, "T0002", "ds_cache")
	files, err := filepath.Glob(filepath.Join(cache, "*.~~~day"))
	if err != nil {
		fmt.Println("扫描缓存目录失败:", err)
		return
	}
	if len(files) == 0 {
		fmt.Println("未找到退市股缓存文件:", cache)
		return
	}
	for _, f := range files {
		name := filepath.Base(f)
		ks, err := parse(f)
		if err != nil {
			fmt.Printf("[%s] 解析失败: %v\n", name, err)
			continue
		}
		fmt.Printf("==== %s ==== 记录数: %d 范围: %s ~ %s\n",
			name, len(ks), ks[0].Date.Format("2006-01-02"), ks[len(ks)-1].Date.Format("2006-01-02"))
		show := func(k rec) {
			fmt.Printf("  %s 开:%.3f 高:%.3f 低:%.3f 收:%.3f 额:%.0f 量:%d手\n",
				k.Date.Format("2006-01-02"), k.Open, k.High, k.Low, k.Close, k.Amount, k.Volume/100)
		}
		fmt.Println("  前 3 条:")
		for _, k := range ks[:min(3, len(ks))] {
			show(k)
		}
		if len(ks) > 6 {
			fmt.Println("  ...")
		}
		fmt.Println("  后 3 条:")
		for _, k := range ks[max(0, len(ks)-3):] {
			show(k)
		}
		fmt.Println()
	}
}
