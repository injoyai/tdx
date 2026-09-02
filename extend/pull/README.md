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

	s, err := pull.NewService(&pull.PullConfig{
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

---

## 🗺 内置市场

| Unit 名 | 市场 | 连接 | 说明 |
|---|---|---|---|
| `a_stock` | 沪深股票 | 7709 | `GetKline*` |
| `index` | 沪深指数 | 7709 | `GetIndex*`（含 `sh000001` 等） |
| `etf_lof` | ETF/LOF | 7709 | `GetKline*` |
| `block` | 板块指数 | 7709 | `880xxx/881xxx`，来自 `block_zs.dat` |
| `future` | 期货（中金所/郑商所/大商所/上期所等） | 7727 | `ExBars` |
| `hk` | 港股 | 7727 | `ExBars` |
| `us` | 美股 | 7727 | `ExBars` |

> `import _ "github.com/injoyai/tdx/extend/pull/market"` 即注册全部；`PullConfig.Units` 选择子集。

---

## ⚙️ 配置项

| 字段 | 说明 | 默认值 |
|---|---|---|
| `Dir` | 数据根目录（**必填**） | — |
| `Units` | 拉取哪些市场；nil = 全部已注册 | 全部 |
| `Codes` | 按市场名覆盖代码列表，key=`Unit.Name()` | 自动发现 |
| `Day` / `Minute` | 是否拉日线 / 1分钟线 | 两者都 true |
| `Goroutines` | 并发数 | `8` |
| `StartAt` | 起始日期 `YYYYMMDD` | 最近两年 |
| `Retry` | 单条失败重试次数 | `tdx.DefaultRetry` |
| `Updated` | 增量去重库 | 自动创建于 `Dir/updated.db` |
| `Manage` | 标准行情(7709)连接源 | 无（std 市场需要） |
| `ExPool` | 扩展行情(7727)连接池 | 无（ex 市场需要） |
| `Workday` | 交易日历 | 无（不判断交易日） |

配置全部通过代码参数传入（本库作为第三方引用，不写配置文件）。

---

## 💾 存储布局

```
{Dir}/
├── updated.db          # 增量去重标记
├── day/{key}.db        # 日线，一代码一库
└── min/{key}/{year}.db # 1分钟线，一代码一年一库
```

`key` 为带市场前缀的唯一键（`sh600000` / `HK00700` / `US.AAPL` / `future.IF2609`），天然避免跨市场撞名。

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
ks, _ := s.QueryDay(pull.Code{Market: "a_stock", Code: "sh600000"}, start, end)

// 1分钟线：自动按年定位文件并拼接，不存在的年份跳过
ms, _ := s.QueryMin(pull.Code{Market: "a_stock", Code: "sh600000"}, start, end)

// 库内最后一条时间戳（空库返回 0）
last, _ := s.LastDayUnix(pull.Code{Market: "a_stock", Code: "sh600000"})
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
