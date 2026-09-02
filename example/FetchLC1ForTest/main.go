package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/extend"
	"github.com/injoyai/tdx/protocol"
)

// 实时拉取股票/指数 1分钟(.lc1)与5分钟(.lc5)K线,生成通达信本地文件供客户端导入测试
// 输出统一放在 ./output/lc1/ 下(见 AGENTS.md 8.2)
func main() {
	outDir := filepath.Join("output", "lc1")
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		logs.PanicErr(err)
	}

	c, err := tdx.DialWith(tdx.NewHostDial(tdx.Hosts), tdx.WithDebug())
	logs.PanicErr(err)
	defer c.Close()

	for _, code := range []string{"sz000001", "sh000001"} {
		// 1分钟 .lc1
		writeMinute1(c, code, outDir)
		// 5分钟 .lc5
		writeMinute5(c, code, outDir)
	}

	fmt.Println("完成, 输出目录:", outDir)
}

// writeMinute1 拉取 1 分钟K线,编码为通达信 .lc1 字节流并写入本地文件
func writeMinute1(c *tdx.Client, code string, outDir string) {
	index := protocol.IsIndex(code)
	var resp *protocol.KlineResp
	var err error
	if index {
		resp, err = c.GetIndexAll(protocol.TypeKlineMinute, code)
	} else {
		resp, err = c.GetKlineMinuteAll(code)
	}
	if err != nil {
		logs.PanicErr(err)
	}
	if len(resp.List) == 0 {
		logs.Panicf("代码 %s 无1分钟数据", code)
	}
	bs, err := extend.WriteMinute1(code, protocol.Klines(resp.List))
	if err != nil {
		logs.PanicErr(err)
	}
	saveFile(outDir, code, "minline", ".lc1", bs)
	printInfo("1分钟.lc1", code, len(resp.List), len(bs), resp.List[0], resp.List[len(resp.List)-1])
}

// writeMinute5 拉取 5 分钟K线,编码为通达信 .lc5 字节流并写入本地文件
func writeMinute5(c *tdx.Client, code string, outDir string) {
	index := protocol.IsIndex(code)
	var resp *protocol.KlineResp
	var err error
	if index {
		resp, err = c.GetIndexAll(protocol.TypeKline5Minute, code)
	} else {
		resp, err = c.GetKline5MinuteAll(code)
	}
	if err != nil {
		logs.PanicErr(err)
	}
	if len(resp.List) == 0 {
		logs.Panicf("代码 %s 无5分钟数据", code)
	}
	bs, err := extend.WriteMinute5(code, protocol.Klines(resp.List))
	if err != nil {
		logs.PanicErr(err)
	}
	saveFile(outDir, code, "fzline", ".lc5", bs)
	printInfo("5分钟.lc5", code, len(resp.List), len(bs), resp.List[0], resp.List[len(resp.List)-1])
}

// saveFile 写入 outDir/vipdoc/<ex>/<sub>/<code><ext>
func saveFile(outDir, code, sub, ext string, bs []byte) {
	ex := code[:2]
	filename := filepath.Join(outDir, "vipdoc", ex, sub, code+ext)
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		logs.PanicErr(err)
	}
	if err := os.WriteFile(filename, bs, 0o644); err != nil {
		logs.PanicErr(err)
	}
}

func printInfo(typName, code string, count, size int, first, last *protocol.Kline) {
	fmt.Printf("%s 代码 %s: 共%d条(%d字节) 时间[%s ~ %s] 最后: 开%.2f 高%.2f 低%.2f 收%.2f 量%d手 额%.0f\n",
		typName, code, count, size,
		first.Time.Format("2006-01-02 15:04"), last.Time.Format("2006-01-02 15:04"),
		last.Open.Float64(), last.High.Float64(), last.Low.Float64(), last.Close.Float64(),
		last.Volume, last.Amount.Float64())
}
