# 📈 通达信协议解析

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go)](https://golang.org)  
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

---

## 📚 参考资源

- [`gotdx`](https://github.com/bensema/gotdx)
- [`mootdx`](https://github.com/mootdx/mootdx)
- [`tdx2db`](https://github.com/jing2uo/tdx2db)
- [`niexq-tdx`](https://github.com/niexqc/niexq-tdx)
- 推荐 **Web可视化界面 + RESTful API**：[tdx-api](https://github.com/oficcejo/tdx-api)

---

## 🛠 功能列表 & 完成情况

| 功能             | 状态    | 主要方法                                            |
|----------------|-------|-------------------------------------------------|
| 盘口(5档报价)       | ✅ 已完成 | `GetQuote`                                      |
| 代码数量/列表        | ✅ 已完成 | `GetCount` `GetCode` `GetCodeAll`               |
| 分时图 / 历史分时图    | ✅ 已完成 | `GetMinute` `GetHistoryMinute`                  |
| 分时成交 / 历史分时成交  | ✅ 已完成 | `GetTrade` `GetHistoryTrade`                    |
| K线 / 历史K线 / 指数 | ✅ 已完成 | `GetKline*` `GetIndex*`                         |
| 股本变迁(除权除息)     | ✅ 已完成 | `GetGbbq` `GetGbbqAll`                          |
| 前/后复权(对齐通达信)   | ✅ 已完成 | `Gbbq.QFQKlineDay` `HFQKlineDay` `QFQ` `HFQ`     |
| 集合竞价           | ✅ 已完成 | `GetCallAuction`                               |
| 财务信息           | ✅ 已完成 | `GetFinanceInfo`                               |
| F10 公司资料       | ✅ 已完成 | `GetCompanyCategory` `GetCompanyContent`        |
| 板块成分(地域/概念/指数) | ✅ 已完成 | `GetBlockData` `GetBlockDataWithIndex`          |
| 大型指数成分(中证2000等) | ✅ 已完成 | `GetSpBlock`                                    |
| 行业归属(通达信/申万)   | ✅ 已完成 | `GetTdxHy`                                      |
| 报表/配置文件下载      | ✅ 已完成 | `GetReportFile` `GetZHBFiles`                   |
| 板块指数代码(id)映射   | ✅ 已完成 | `GetTdxZs` `GetTdxBk`                           |
| 个股统计 / 资金流向    | ✅ 已完成 | `GetTdxStat` `GetTdxStat2`                      |
| 新股申购           | ✅ 已完成 | `GetXgsg`                                       |
| 扩展行情(期货/港股/外盘) | ✅ 已完成 | `DialExHq` + `ExQuote` `ExBars` `ExTrade` 等     |

---

## 🚀 快速开始

```go
package main

import (
	"fmt"
	"github.com/injoyai/tdx"
)

func main() {
	// 连接服务器，开启日志，支持断线重连
	c, err := tdx.Dial("124.71.187.122:7709", tdx.WithDebug(), tdx.WithRedial())
	if err != nil {
		panic(err)
	}

	// 获取行情
	resp, err := c.GetQuote("sz000001", "sh600008")
	if err != nil {
		panic(err)
	}

	for _, v := range resp {
		fmt.Printf("%#v\n", v)
	}

	<-c.Done()
}
```

---

## 📦 板块与板块指数代码(id)

板块成分文件(`block_*.dat`)本身**不含板块指数代码(id)**，id 映射在 `tdxzs.cfg`(全称)，而成分文件用简称，二者经 `tdxbk.cfg`(简称↔全称) 桥接。`GetBlockDataWithIndex` 自动完成关联(命中率约 100%)。

```go
c, _ := tdx.DialDefault()
defer c.Close()

// 板块成分 + 板块指数代码(id)。file 可选: protocol.BlockFileGN(概念)/FG(地域风格)/ZS(指数)
blocks, _ := c.GetBlockDataWithIndex(protocol.BlockFileGN)
for _, b := range blocks {
	fmt.Printf("板块=%s id=%s 类型=%d 成分数=%d\n", b.Name, b.Index, b.Type, len(b.Codes))
}

// 仅成分(无 id, 不额外下载 zhb.zip)
plain, _ := c.GetBlockData(protocol.BlockFileGN)
_ = plain
```

### 大型指数成分(中证2000/1000/500/A500/国证2000)

`block_zs.dat` 单板块成分上限 400，装不下中证2000/1000/500 等大型指数，服务端把它们放在文本文件 `spblock.dat`（无上限），用 `GetSpBlock` 获取。

> **沪深300** 例外：仍在 `block_zs.dat`，用 `GetBlockData(protocol.BlockFileZS)` 取（成分 300）。

```go
// 专业板块: 中证2000/中证1000/中证500/中证A500/国证2000/深证成指 等
sp, _ := c.GetSpBlock()
for _, b := range sp {
	fmt.Printf("板块=%s 成分数=%d\n", b.Name, len(b.Codes)) // Codes 7字符: 市场(0深/1沪/2北)+代码
}

// 按名取中证2000成分
if b := protocol.SpBlockByName(sp, "中证2000"); b != nil {
	fmt.Printf("中证2000 成分数=%d\n", len(b.Codes)) // 2000
}
```

> 演示: [`example/GetSpBlock`](example/GetSpBlock)。

---

## 🔁 复权日线 (前复权 / 后复权, 对齐通达信)

复权基于股本变迁(gbbq)的除权除息事件做**仿射变换** `price_adj = QFQMul × price_raw + QFQAdd`，
结果**四舍五入到分(2 位小数)，与电脑端通达信桌面端逐日对齐**(含配股、大比例送转、股改停牌复合事件均已实盘核验)。
成交量/额不复权。`Gbbq` 内置 gbbq 数据自动更新与本地缓存(sqlite)。

```go
c, _ := tdx.DialDefault()
defer c.Close()

gb, _ := tdx.NewGbbq(tdx.WithGbbqClient(c)) // 首次会拉取并缓存 gbbq

// 方式一: 一步到位拉取全量历史 + 复权
qfq, _ := gb.QFQKlineDay("sh600519") // 前复权日线
hfq, _ := gb.HFQKlineDay("sh600519") // 后复权日线
for _, k := range qfq {
	fmt.Printf("%s 开%.2f 高%.2f 低%.2f 收%.2f\n",
		k.Time.Format("2006-01-02"), k.Open.Float64(), k.High.Float64(), k.Low.Float64(), k.Close.Float64())
}

// 方式二: 已有不复权 K 线时直接复权
resp, _ := c.GetKlineDayAll("sh600519")
qfq2 := gb.QFQ("sh600519", resp.List)
_ = qfq2

// 方式三: 仅取复权因子(每个除权区间一个仿射系数), 自行施加
fs := gb.GetFactors("sh600519", resp.List)
for _, f := range fs {
	_ = f.QFQPrice(protocol.Yuan(100)) // 对任意原始价做前复权
}
```

> 价格类型 `protocol.Price` 为 **int64, 单位厘(元×1000)**，`.Float64()` 得元。股票最小变动 0.01 元(分)，ETF/可转债/指数到 0.001 元(厘)。
> 完整演示见 [`example/GetQFQKline`](example/GetQFQKline)。

---

## 🏢 财务 / F10 公司资料 / 行业归属

```go
c, _ := tdx.DialDefault()
defer c.Close()

// 财务/基本面: 流通股本/总股本/上市日期/股东户数/净利润等
fi, _ := c.GetFinanceInfo(protocol.ExchangeSH, "600519")
fmt.Println(fi.LiuTongGuBen, fi.ZongGuBen, fi.GuDongRenShu, fi.JingLiRun)

// F10 公司资料: 先取分类, 再读正文
cats, _ := c.GetCompanyCategory(protocol.ExchangeSH, "600519")
content, _ := c.GetCompanyContent(protocol.ExchangeSH, "600519", cats[0].Filename, cats[0].Start, cats[0].Length)
_ = content

// 行业归属(通达信新行业 T + 申万行业 X), 全市场
hy, _ := c.GetTdxHy()
for _, v := range hy { _ = v.TdxHy; _ = v.SwHy }
```

> 演示: [`example/GetFinanceInfo`](example/GetFinanceInfo)、[`example/GetTdxHy`](example/GetTdxHy)。

---

## 🗄 报表/配置数据 (zhb.zip)

通达信板块/配置文件以 `zhb.zip` 总包经 report file 协议(0x06B9)整体下发，含 44 个文件(tdxzs.cfg/tdxbk.cfg/incon.dat/tdxstat.cfg 等)。

```go
// 任意报表文件原始字节(无需预查大小, 自动分块拉取)
raw, _ := c.GetReportFile("zhb.zip")

// 下载 zhb.zip 并解压 → 文件名→原始字节
files, _ := c.GetZHBFiles()
for name := range files { fmt.Println(name) }

// 板块名↔指数代码(id)   /   概念简称↔全称
zs, _ := c.GetTdxZs()   // []*protocol.TdxZs{Name, Code(880xxx), Type, ...}
bk, _ := c.GetTdxBk()   // []*protocol.TdxBk{Short, Full}
_ = zs; _ = bk
```

---

## 📊 个股统计 / 资金流向 / 新股申购

来自 `zhb.zip` 的全市场逐股数据。`tdxstat.cfg` 字段经 10 只大市值股对照实盘核验(见 `protocol/model_stat.go` 命中率注释)，未核验字段保留在 `Fields`。

```go
// 个股综合统计: 市盈TTM/静态市盈/股息率/涨跌幅/连涨连跌天数/区间涨跌幅(5/10/20/60日/YTD, 均已核验)
stat, _ := c.GetTdxStat()
for _, s := range stat {
	fmt.Printf("%s PE_TTM=%.2f 静PE=%.2f 股息=%.2f%% 连涨跌=%d 5日=%.2f%% 20日=%.2f%% YTD=%.2f%%\n",
		s.Code, s.PETTM, s.PEStatic, s.DivYield, s.TrendDays, s.Chg5, s.Chg20, s.ChgYTD)
}

// 资金流向 + 板块归属(股→板块id 反向映射)
stat2, _ := c.GetTdxStat2()
s2b := protocol.StockBlockIndex(stat2) // map[证券代码]板块指数代码
_ = s2b

// 新股申购
xgsg, _ := c.GetXgsg()
for _, x := range xgsg {
	fmt.Printf("%s %s 申购日=%s 发行价=%.3f\n", x.Code, x.Name, x.Date, x.IssuePrice)
}
```

> 完整演示见 [`example/DumpReportFile`](example/DumpReportFile)：下载 zhb.zip、解压、解析、板块 id 关联一条龙。

---

## 🌍 扩展行情 TdxExHq (期货 / 港股 / 外盘, 端口 7727)

扩展行情走独立服务(端口 7727)，需用 `DialExHq*` 单独连接。

```go
ex, err := tdx.DialExHqDefault() // 或 DialExHq(addr) / DialExHqHosts(hosts)
if err != nil { panic(err) }
defer ex.Close()

markets, _ := ex.ExMarkets()                       // 市场代码表
n, _ := ex.ExCount()                               // 品种数量
insts, _ := ex.ExInstruments(0, 100)               // 品种(代码)列表分页
q, _ := ex.ExQuote(market, code)                   // 单品种五档行情
bars, _ := ex.ExBars(category, market, code, 0, 20) // K线
ticks, _ := ex.ExTrade(market, code, 0, 30)        // 分笔成交
_ = markets; _ = n; _ = insts; _ = q; _ = bars; _ = ticks
```

---

## 🌐 服务器列表 (端口 7709)

### 🏙 上海

| IP              | 测试时间       | 运营商 | 结果 |
|-----------------|------------|-----|----|
| 124.71.187.122  | 2026-05-16 | 华为  | ✅️ |
| 122.51.120.217  | 2026-05-16 | 腾讯  | ✅️ |
| 111.229.247.189 | 2026-05-16 | 腾讯  | ✅️ |
| 122.51.232.182   | 2026-05-16 | 腾讯  | ✅️ |
| 118.25.98.114   | 2026-05-16 | 腾讯  | ✅️ |
| 124.70.199.56  | 2026-05-16 | 华为  | ✅️ |
| 121.36.225.169   | 2026-05-16 | 华为  | ✅️ |
| 123.60.70.228   | 2026-05-16 | 华为  | ✅️ |
| 123.60.73.44    | 2026-05-16 | 华为  | ✅️ |
| 124.70.133.119  | 2026-05-16 | 华为  | ✅️ |
| 124.71.187.72   | 2026-05-16 | 华为  | ✅️ |
| 123.60.84.66    | 2026-05-16 | 华为  | ✅️ |
|124.223.163.242| 2026-05-16 |腾讯云| ✅️ |
|150.158.160.2|2026-05-16|腾讯云| ✅️ |
|101.35.121.35|2026-05-16|腾讯云| ✅️ |
|111.231.113.208|2026-05-16|腾讯云| ✅️ |

### 🏙 北京

| IP             | 测试时间       | 运营商 | 结果 |
|----------------|------------|-----|----|
| 62.234.50.143  | 2026-05-16 | 腾讯云  | ✅️ |
| 81.70.151.186  | 2026-05-16 | 腾讯云  | ✅️ |
| 101.42.240.54  | 2026-05-16 | 腾讯云  | ✅️ |
| 101.43.159.194  | 2026-05-16 | 腾讯云  | ✅️ |
| 120.53.8.251 | 2026-05-16 | 腾讯云  | ✅️ |
| 152.136.191.169  | 2026-05-16 | 腾讯云  | ✅️ |
| 49.232.15.141  | 2026-05-16 | 腾讯云  | ✅️ |
| 82.156.174.84  | 2026-05-16 | 腾讯云  | ✅️ |
| 101.42.164.241  | 2026-05-16 | 腾讯云  | ✅️ |

### 🏙 广州

| IP              | 测试时间       | 运营商 | 结果 |
|-----------------|------------|-----|----|
| 124.71.9.153"| 2026-05-16 |   华为| ✅️ |
| 116.205.163.254"| 2026-05-16 | 华为| ✅️ |
| 116.205.171.132"| 2026-05-16 |华为| ✅️ |
| 116.205.183.150"| 2026-05-16 |华为| ✅️ |
| 111.230.186.52"| 2026-05-16 |腾讯| ✅️ |
| 110.41.2.72"| 2026-05-16 |华为| ✅️ |
| 110.41.147.114"| 2026-05-16 |华为| ✅️ |
| 101.33.225.16"| 2026-05-16 |腾讯云| ✅️ |
| 175.178.112.197"| 2026-05-16 |腾讯云| ✅️ |
| 175.178.128.227"| 2026-05-16 |腾讯云| ✅️ |
| 43.139.95.83"| 2026-05-16 |腾讯云| ✅️ |
| 159.75.29.111"| 2026-05-16 |腾讯云| ✅️ |
| 43.139.18.171"| 2026-05-16 |腾讯云| ✅️ |
| 81.71.32.47"| 2026-05-16 |腾讯云| ✅️ |
| 129.204.230.128"| 2026-05-16 |腾讯云| ✅️ |

### 🏙 武汉

| IP            | 测试时间       | 运营商 | 结果 |
|---------------|------------|-----|----|
| 119.97.185.59 | 2026-01-26 | 电信  | ✅️ |

---

## ⚠️ 免责声明

1. 本项目仅供 **学习、研究和技术交流** 使用，禁止用于任何商业或非法用途。
2. 使用本项目产生的任何数据、损失或法律责任，作者 **不承担任何责任**。
3. 对第三方服务器或服务的访问，用户需自行遵守相关法律法规及服务协议。
4. 请勿将本项目用于侵犯他人权益或违反监管规定的行为。

---

## 📄 许可证

MIT License - 详见 [LICENSE](LICENSE)

⭐ 喜欢这个项目吗？点个 Star 支持一下吧！  

