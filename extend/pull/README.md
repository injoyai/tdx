# 🔄 行情数据拉取服务 (pull)

将通达信行情数据**增量拉取并落盘 sqlite**：编排多市场（`Unit` 可插拔）、并发、重试、增量去重、周期合并。

只存**日线 + 1分钟线**原始数据，其余周期（5/15/30/60 分钟、N 日等）由查询侧纯内存派生，不落盘。

---

## 🧩 架构

```
pull.Service ──编排──> Unit(市场) ──拉取──> 通达信服务器
     │                    │
     │                    └── a_stock / index / etf_lof / block (7709, 走 tdx.Manage)
     │                    └── future / hk / us          (7727, 走 tdx.IPool)
     │
     └──存储──> sqlite（一代码一库，见下方存储布局）
```

- **`Unit`**：一个可拉取的独立市场/品种类别。新增市场 = 新增一个实现 + `pull.Register`，无需改框架。
- **`Service`**：负责并发调度（协程池 + 进度条）、单条重试、按市场粒度的当日增量去重、引擎缓存（同一库文件只打开一次）。

---

## 🚀 快速开始

```go
package main

import (
	"context"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/extend/pull"
	_ "github.com/injoyai/tdx/extend/pull/market" // 注册全部内置市场 Unit
)

func main() {
	// 标准行情(7709)连接源：股票/指数/ETF/板块需要
	m, err := tdx.NewManage()
	if err != nil {
		panic(err)
	}
	if closer, ok := m.IPool.(interface{ Close() error }); ok {
		defer closer.Close()
	}

	// 扩展行情(7727)连接池：期货/港股/美股需要
	ex, err := tdx.NewPool(func() (*tdx.Client, error) {
		return tdx.DialExHqDefault()
	}, 4)
	if err != nil {
		panic(err)
	}
	defer ex.Close()

	s, err := pull.NewService(&pull.Config{
		Dir:     "./output/pullv2", // 数据根目录
		StartAt: "20260101",        // 首次全量的最早日期；空 = 只拉最近两年
		Manage:  m,
		ExPool:  ex,
		Workday: m.Workday, // 交易日历（可选，非交易日自动跳过）
	})
	if err != nil {
		panic(err)
	}
	defer s.Close()

	// 拉取（当日已拉过自动跳过；must=true 强制重拉）
	if err := s.Update(context.Background()); err != nil {
		panic(err)
	}
}
```

> 完整演示见 [`example/PullV2`](../../example/PullV2)：全市场 / 自定义代码列表 / 只拉日线 三种用法。

### 两种拉取模式

- **全量模式**（`Codes` 为空，上例）：全部注册市场，各自自动发现代码（整市场拉取）。
- **白名单模式**（`Codes` 非空）：只拉列出的代码，市场由 `ParseCode` 自动路由；未涉及的市场不拉。

---

## 🗺 内置市场

| 枚举常量 | 值（即存储目录） | 市场 | 连接 | 说明 |
|---|---|---|---|---|
| `pull.MarketAStock` | `cn/stock` | 沪深股票 | 7709 | `GetKline*` |
| `pull.MarketIndex` | `cn/index` | 沪深指数 | 7709 | `GetIndex*`（含 `sh000001` 等） |
| `pull.MarketEtfLof` | `cn/etf` | ETF/LOF | 7709 | `GetKline*` |
| `pull.MarketBlock` | `cn/block` | 板块指数 | 7709 | `880xxx/881xxx`，来自 `block_zs.dat` |
| `pull.MarketFuture` | `future` | 期货（中金所/郑商所/大商所/上期所等） | 7727 | `ExBars` |
| `pull.MarketHK` | `hk/stock` | 港股主板 | 7727 | `ExBars` |
| `pull.MarketHKIndex` | `hk/index` | 港股指数（恒生系/中华系） | 7727 | `ExBars`，市场编码27，含 HSI/VHSI/CES100 等 |
| `pull.MarketUS` | `us/stock` | 美股 | 7727 | `ExBars`，股票/ETF/指数混合（协议层无法区分） |

> 市场标识统一用 `pull.Market`（`type Market string`）枚举，替代裸字符串；**枚举值即两级「地区/资产」存储目录路径**（`Code.DirName()` 直接返回枚举值），自定义市场可扩展自己的 Market 值。

> `import _ "github.com/injoyai/tdx/extend/pull/market"` 即注册全部；`Codes` 为空 = 全量拉取所有注册市场。

---

## ⚙️ 配置项

| 字段 | 说明 | 默认值 |
|---|---|---|
| `Dir` | 数据根目录（**必填**） | — |
| `Codes` | 拉取代码列表（自动路由市场，见 `pull.ParseCode`）；空 = 全部注册市场自动发现 | 自动发现 |
| `Day` / `Minute` | 是否拉日线 / 1分钟线 | 两者都 true |
| `Goroutines` | 并发数 | `8` |
| `StartAt` | 起始日期 `YYYYMMDD` | 最近两年 |
| `Retry` | 单条失败重试次数 | `tdx.DefaultRetry` |
| `Updated` | 增量去重库 | 自动创建于 `Dir/updated.db` |
| `Manage` | 标准行情(7709)连接源 | 无（std 市场需要） |
| `ExPool` | 扩展行情(7727)连接池 | 无（ex 市场需要） |
| `Workday` | 交易日历 | 无（不判断交易日） |

配置全部通过代码参数传入（本库作为第三方引用，不写配置文件）。

> **股本数据自动启用**：`tdx.NewManage()` 默认塞入空 Gbbq（股本变迁数据未初始化），
> 拉取股票日线时 pull 会检测到并**自动初始化**（首跑全量拉取一次，约几分钟，落库
> `./data/database/gbbq.db`；之后每日 05:09 定时增量更新）。之后流通股/总股本/换手率
> 自动回填到日线。若不想启用，传入自定义 `IGbbq` 实现或 `WithGbbq` 即可。

### 代码自动路由

`Codes` 是扁平的代码列表，无需按市场分组——`pull.ParseCode` 自动识别所属市场：

```go
Codes: []string{
    "sz000001", // 沪深股票（带前缀）
    "600000",   // 沪深股票（6 位裸数字，自动补 sh 前缀）
    "510300",   // ETF（自动补前缀并分类）
    "399001",   // 深证成指（指数）
    "880001",   // 板块指数
    "00700",    // 港股（5 位数字）
    "HSI",      // 恒生指数（知名港股指数白名单）
    "AAPL",     // 美股（纯字母）
    "cff/IF2609", // 期货（带交易所前缀）
}
```

> 注意：`000001` 按股票（平安银行 `sz000001`）路由；上证指数需带前缀写 `sh000001`。
> 其余港股指数（HZ50xx/CESxxx 等）用完整键指定，如 `hk/index.CES120`。

---

## 💾 存储布局

```
{Dir}/
├── updated.db                  # 增量去重标记
├── cn/                          # 中国大陆
│   ├── stock/day/{code}.db      # 沪深股票
│   ├── stock/min/{code}/{code}-{year}.db
│   ├── index/ ...               # 沪深指数
│   ├── etf/ ...                 # ETF/LOF
│   └── block/ ...               # 板块指数
├── hk/                          # 香港
│   ├── stock/ ...               # 港股主板
│   └── index/ ...               # 港股指数（恒生系/中华系）
├── us/                          # 美国（股票/ETF/指数混合）
│   └── stock/ ...
└── cn/
    └── future/ ...              # 期货（各交易所共用）
```

两级「地区/资产」目录（`cn/stock` / `cn/etf` / `hk/index` / `us/stock` / `cn/future` …），按地区或资产类别整体删除/归档即删对应目录。**文件名用原始代码**（目录已含市场信息，无需市场前缀），如 `cn/stock/day/sh600000.db`、`hk/stock/day/00700.db`、`hk/index/day/HSI.db`、`us/stock/day/AAPL.db`；期货多交易所代码含 `/`（如 `cff/IF2609`），文件名替换为 `-`：`cn/future/day/cff-IF2609.db`。同市场内代码唯一，无撞名风险。

### 单位约定

| 字段 | 单位 |
|---|---|
| 价格（Open/High/Low/Close） | 元（float64） |
| Volume | **股**（协议"手"×100，`pull.ToShares`） |
| Amount | 元 |

> 注意：与本地 `.day/.lc1/.lc5` 文件不同（指数/股票"手/股"混存），pull 库内统一为**股**，上层无需再区分市场。

### 增量写入

拉取时按库内最后时间戳增量拉取，写入时**删除 `Unix>=from` 后批量插入**（事务内幂等）；
空数据不写库（避免产生空库文件）。

---

## 🔍 数据查询

```go
// 日线（升序）；start/end 零值 = 不限制该端
ks, _ := s.QueryDay(pull.Code{Market: pull.MarketAStock, Code: "sh600000"}, start, end)

// 1分钟线：自动按年定位文件并拼接，不存在的年份跳过
ms, _ := s.QueryMin(pull.Code{Market: pull.MarketAStock, Code: "sh600000"}, start, end)

// 库内最后一条时间戳（空库返回 0）
last, _ := s.LastDayUnix(pull.Code{Market: pull.MarketAStock, Code: "sh600000"})
```

---

## 📊 周期合并

```go
// 日线 → 近似 N 日周期（固定 N 根分块，非日历对齐；5≈周、20≈月、60≈季、250≈年）
weekly := pull.DayToPeriod(ks, 5)

// 1分钟线 → N 分钟周期（5/15/30/60 等）
m5 := pull.MinuteToPeriod(ms, 5)
```

---

## 🔌 自定义市场

```go
type MyUnit struct{}

func (u *MyUnit) Name() string { return "my" }
func (u *MyUnit) Codes(ctx context.Context, s *pull.Service) ([]pull.Code, error) { /* ... */ }
func (u *MyUnit) FetchDay(ctx context.Context, s *pull.Service, code pull.Code) error { /* ... */ }
func (u *MyUnit) FetchMin(ctx context.Context, s *pull.Service, code pull.Code) error { /* ... */ }

func init() { pull.Register(&MyUnit{}) }
```

增量判断、落库、重试、并发全部由 `Service` 统一处理，Unit 只需负责"拉原始 K 线 + 转股"。
参考实现：[`market/hk.go`](market/hk.go)（扩展行情）、[`market/a_stock.go`](market/a_stock.go)（标准行情）。

---

## 🧪 测试

```bash
go test ./extend/pull/... -count=1
```

> 存储/合并/单位换算均有单测（离线）；全市场拉取需联网，见 `example/PullV2`。
