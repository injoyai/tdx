# 🔄 行情数据拉取服务 (pull)

将通达信行情数据**增量拉取并落盘 sqlite**：编排多市场（`Unit` 可插拔）、并发、重试、增量去重、周期合并。

只存**日线 + 1分钟线**原始数据，其余周期（5/15/30/60 分钟、N 日等）由查询侧纯内存派生，不落盘。周期合并保留原始浮点精度，不转换成整数厘。

v2 的 K 线拉取统一使用本服务。`extend/pulltrade` 只负责历史分笔及合成分钟线 CSV 导出，不参与本服务的存储、增量或兜底调度。原 `extend.PullKline` 已移除，调用方式与旧数据库处理见 [v2 迁移指南](../../docs/v2-migration.md)。

---

## 🧩 架构

```
pull.Service ──编排──> Unit(市场) ──拉取──> 通达信服务器
     │                    │
     │                    └── a_stock / index / etf_lof / block (7709, 走 manage.Manage)
     │                    └── future / hk / us          (7727, 走 tdx.ClientPool)
     │
     └──存储──> sqlite（一代码一库，见下方存储布局）
```

- **`Unit`**：一个可拉取的独立市场/品种类别。新增市场 = 新增一个实现 + `pull.Register`，无需改框架。
- **`Service`**：负责并发调度（协程池 + 进度条）、单条重试、按市场粒度的增量去重、有上限的引擎缓存。市场间顺序执行，市场内代码并发；`Update` 只执行一次，定时调度由调用方安排。

---

## 🚀 快速开始

```go
package main

import (

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/manage"
	"github.com/injoyai/tdx/v2/extend/pull"
	_ "github.com/injoyai/tdx/v2/extend/pull/market" // 注册全部内置市场 Unit
)

func main() {
	// 标准行情(7709)连接源：股票/指数/ETF/板块需要
	m, err := manage.New()
	if err != nil {
		panic(err)
	}
	defer m.Close()

	// 扩展行情(7727)连接池：期货/港股/美股需要
	ex, err := tdx.NewPool(func() (tdx.Client, error) {
		return tdx.DialExDefault()
	}, 4)
	if err != nil {
		panic(err)
	}
	defer ex.Close()

	s, err := pull.New(&pull.Config{
		Dir:     "./output/pull", // 数据根目录
		StartAt: "20260101",        // 首次全量的最早日期；空 = 只拉最近两年
		Manage:  m,
		ExPool:  ex,
		Workday: m.Workday, // 交易日历（可选，非交易日自动跳过）
	})
	if err != nil {
		panic(err)
	}
	defer s.Close()

	// 拉取（已完成市场自动跳过；must=true 可在非交易日调用，仍保留市场级去重）
	if err := s.Update(); err != nil {
		panic(err)
	}
}
```

> 完整演示见 [`examples/pull`](../../examples/pull)：默认拉取单证券，可通过代码参数、`-all` 或 `-minute=false` 选择指定范围、全市场或只拉日线。

### 两种拉取模式

- **全量模式**（`Codes` 为空，上例）：全部注册市场，各自自动发现代码（整市场拉取）。
- **白名单模式**（`Codes` 非空）：只拉列出的代码，市场由 `ParseCode` 自动路由；未涉及的市场不拉。

---

## 🗺 内置市场

| 枚举常量 | 值（即存储目录） | 市场 | 连接 | 说明 |
|---|---|---|---|---|
| `protocol.MarketAStock` | `cn/stock` | 沪深股票 | 7709 | `GetKline*` |
| `protocol.MarketIndex` | `cn/index` | 沪深指数 | 7709 | `GetKline*` 自动选择指数解码（含 `sh000001` 等） |
| `protocol.MarketEtfLof` | `cn/etf` | ETF/LOF | 7709 | `GetKline*` |
| `protocol.MarketBond` | `cn/bond` | 可转债 | 7709 | 普通 K 线，按国内默认倍率 |
| `protocol.MarketOther` | `cn/other` | 其他国内品种，含尚未细分的普通债券 | 7709 | 普通 K 线，按国内默认倍率 |
| `protocol.MarketBlock` | `cn/block` | 板块指数 | 7709 | `880xxx/881xxx`，来自 `block_zs.dat` |
| `protocol.MarketFuture` | `cn/future` | 期货（中金所/郑商所/大商所/上期所等） | 7727 | `GetKline` |
| `protocol.MarketHK` | `hk/stock` | 港股主板 | 7727 | `GetKline` |
| `protocol.MarketHKIndex` | `hk/index` | 港股指数（恒生系/中华系） | 7727 | `GetKline`，市场编码27，含 HSI/VHSI/CES100 等 |
| `protocol.MarketUS` | `us/stock` | 美股 | 7727 | `GetKline`，股票/ETF/指数混合（协议层无法区分） |

> 市场标识统一用 `protocol.Market`（`type Market string`）枚举，替代裸字符串；**枚举值即两级「地区/资产」存储目录路径**（`Code.DirName()` 直接返回枚举值），自定义市场可扩展自己的 Market 值。

### 市场 K 线配置

`Market.Info()` 返回行情协议、解码分类、日线/1 分钟线的请求类型和 `VolumeScale`，并用 `TickVolumeScale` 声明报价/分时/分笔倍率。内置有效倍率全部为正，集中在 [`protocol/market.go`](../../protocol/market.go)。

```go
code, err := pull.ParseCode("hk00700")
if err != nil {
    panic(err)
}
info, ok := code.Market.Info()
if !ok {
    panic("没有内置市场配置")
}
// extended、4、20422500（股）
volume, err := info.Day.ToStorageVolume(204225)
if err != nil { panic(err) }
fmt.Println(info.Protocol, info.Day.Category, volume)
```

| 市场 | 日线类型 / 解码量单位 | 1 分钟类型 / 解码量单位 |
|---|---|---|
| 股票、ETF/LOF | 9 / 百股 | 7 / 百股 |
| 可转债、其他国内品种 | 9 / 默认 ×100 | 7 / 默认 ×100 |
| 指数、板块指数 | 9 / 股 | 7 / 股 |
| 港股主板 | 4 / 百股（×100 转股） | 8 / 百股（×100 转股） |
| 期货 | 4 / 张（×1） | 8 / 张（×1） |
| 美股 | 4 / 股（×1） | 8 / 股（×1） |
| 港股指数 | 4 / 原样保留（×1），数量接口拒绝 | 8 / 原样保留（×1），数量接口拒绝 |

`VolumeScale=1` 保留已确认的股数/张数，`100` 为百股换算或用户指定的国内默认倍率；可转债等国内默认项尚未逐接口校准。`ToStorageVolume` 返回 `(int64,error)`，拒绝零/负倍率、负数量和溢出。港股指数含成交额口径，倍率 1 不代表已确认股数，统一数量接口拒绝。未匹配市场（含空值）的 `Info()` 也填充标准行情配置：股票解码、日线类型 9、分钟线类型 7，所有成交量倍率默认 100；布尔值仍返回 `false` 标明非内置市场，自定义 `Unit` 可按实际协议声明配置。

期货、美股旧存储采用默认 ×100，新数据采用 ×1；已有数据库不自动修改，不能混用新旧数值。应保留旧库后重新拉取，或先核实来源再迁移。

标准行情由 `GetKline*` 按代码选择解码类型。股票/ETF 分钟拉取使用 `GetKlineMinute241Until` 保留竞价拆分；可转债、其他国内品种及指数走普通分页，不套用股票竞价拆分。K 线与报价/分时/分笔使用各自声明的倍率，本地文件仍遵循自身协议单位。

港股日线类型 4 已用腾讯控股单根、五根请求及独立来源核对；整数百股精度无法恢复零股。类型 9 的该样本首条量字段为 43977，其他量近似万股，不能直接沿用手转股的换算。未知项需取得对应市场及周期的样本后调整配置。

> `import _ "github.com/injoyai/tdx/v2/extend/pull/market"` 即注册全部；`Codes` 为空 = 全量拉取所有注册市场。

---

## ⚙️ 配置项

| 字段 | 说明 | 默认值 |
|---|---|---|
| `Dir` | 数据根目录（**必填**） | — |
| `Codes` | 拉取代码列表（自动路由市场，见 `pull.ParseCode`）；空 = 全部注册市场自动发现 | 自动发现 |
| `Day` / `Minute` | 是否拉日线 / 1分钟线 | 两者都 true |
| `TradeFallback` | 用历史分笔补充缺失分钟线，仅股票/ETF/LOF | `false` |
| `Goroutines` | 并发数 | `8` |
| `MaxCachedEngines` | 行情数据库引擎缓存上限；使用中的引擎不淘汰，并发借用可暂时超限，释放后回落 | `32` |
| `StartAt` | 起始日期 `YYYYMMDD` | 最近两年 |
| `Retry` | 单条失败重试次数 | `tdx.DefaultRetry` |
| `Tracker` | `*update.Tracker` 增量去重库（`lib/update`） | 自动创建于 `Dir/updated.db` |
| `Manage` | 标准行情(7709)连接源 | 无（std 市场需要） |
| `ExPool` | 扩展行情(7727)连接池 | 无（ex 市场需要） |
| `Workday` | `*workday.Workday` 历史交易日历（`extend/workday`） | 无（不判断交易日） |

配置全部通过代码参数传入（本库作为第三方引用，不写配置文件）。`New` 会保存配置快照，后续修改原始 `Config` 不影响已创建的服务。

行情按源日期时分保存：日线 `Time="YYYY-MM-DD"`，分钟线 `Time="YYYY-MM-DD HH:MM"`，均为主键。查询、增量边界和年度分库使用源时间，不依赖主机 `TZ`。不额外保存 `TradingDate`，也不推断美股时区或期货夜盘交易日。

日线和分钟线均只保存源时间字符串与已确认的交易日；分钟线的 `Unix`、`TimeZone` 字段和 `Timestamp()` 已移除。存储只校验源日期时分格式、年份和范围，不应用夏令时或来源时区规则。已有 `source-time-v1` 库可以继续读写；历史额外列不再映射或使用，也不会自动删列。

数据根目录自动创建 `.time-format`（`source-time-v1`）。不兼容旧 Unix 主键格式；含 `.storage-timezone` 或未标记旧数据库的目录会被拒绝，请配置新的 `Dir` 并重新拉取。不会自动删除、覆盖或迁移旧数据。

`StartAt` 为空时使用东八区当前日期推算默认起始日期；非空值必须是合法的 `YYYYMMDD` 日期，否则创建服务会直接返回错误，不创建数据库。

> **股本数据自动启用**：`manage.New()` 默认塞入空 Gbbq（股本变迁数据未初始化），
> 拉取股票日线时 pull 会检测到并**自动初始化**（首跑全量拉取一次，约几分钟，落库
> `./output/data/database/gbbq.db`；之后每日 09:05 定时增量更新）。之后流通股/总股本/换手率
> 自动回填到日线。只在股票日线任务中触发，查询代码、指数/ETF 和纯分钟任务均不初始化股本。
> 通过 `manage.WithGbbq` 或 `manage.WithDialGbbq` 提供的对象（包括空实现）会原样使用；辅助库位置不随 `Config.Dir` 改变。

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

内置市场统一使用 `protocol.ParseInstrument` 校验：扩展代码最多 9 字节，美股须含字母，期货须同时含字母和数字；`cn/stock` 等完整市场键必须与实际品种匹配，非法输入在创建数据库前返回错误。同一证券的合法输入统一规范化为同一代码和存储文件名，自定义市场保留自身代码规则。

> 注意：`000001` 按股票（平安银行 `sz000001`）路由；上证指数需带前缀写 `sh000001`。
> 其余港股指数（HZ50xx/CESxxx 等）用完整键指定，如 `hk/index.CES120`。

---

## 💾 存储布局

```
{Dir}/
├── .time-format               # source-time-v1 源日期时分格式
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
| 价格（Open/High/Low/Close） | 证券原币种或报价单位（float64） |
| Volume | 直接使用统一 Client 的最小单位：股票为股，期货为张；可转债等国内默认项尚未逐接口校准 |
| Amount | 原币种成交额；Client 未提供金额时保存 0，不能据此认定真实零成交额 |
| Source（分钟线） | 空字符串=原生数据；`trade`=分笔合成，聚合结果含合成输入时保留该标记 |

> 与本地 `.day/.lc1/.lc5` 文件的单位不同。国内默认 ×100 不是单位校准结论，不能仅依据字段名宣称其已验证为股数；港股指数不作为成交数量输出。

拉取层直接保存统一 `tdx.Kline` 的时间、价格和成交量，不再提取 `Standard` / `Extended` 后重复换算，也不受旧 `StandardResponse()` 的 uint16 记录数限制。Client 的 `Amount=nil` 保存为 0，不额外增加状态字段。扩展原始 `Amount` 与持仓字节重叠，不能作为成交额；需要原始字段时使用 Client 记录的 `Extended`。

周期合并直接累加 `Amount`。含有未提供金额的记录时，合计只代表已有金额之和，不保证完整成交额；价格和金额都不做汇率换算。已有 `source-time-v1` 数据库可继续使用，多余的旧 `TradingDate` / `AmountUnknown` 列不再映射，新库不创建它们，不自动删列或修改历史值。曾自行写入非零未知金额的旧库需由调用方核对后重拉。

### 增量写入

拉取时按库内最后源时间增量拉取，写入时**删除 `Time>=from` 后批量插入**（事务内幂等）；
空数据不写库（避免产生空库文件）。

分钟线对所有待补年份共用一次从新到旧的分页，股票/ETF 的集合竞价逐笔查询也不再按年份重复。
历史年份的 `minuteCoverage` 表与 K 线在同一事务中提交。开启 `TradeFallback` 时，股票/ETF 的兜底检查完成后，当年再按最后源时间增量；未完成时仍补查早期历史。
新格式库若没有完成标记，会重新扫描认证；补拉只覆盖本次实际返回的时间段，保留服务器已不提供的本地历史前缀。
空结果、拉取失败以及扩展行情偏移超限均不会写成功标记。`LastDayTime/LastMinuteTime` 不再为不存在的文件建库。
完成标记表示成功处理了服务器当前可提供的数据，不保证其保留了所请求区间的全部历史；空年份会在后续执行中重新检查。

### 历史分笔兜底（股票、ETF/LOF）

通过 `pull.Config{TradeFallback: true}` 显式开启，默认关闭；仅支持股票及 ETF/LOF，指数、板块指数、港美股和期货不启用。关闭时只拉原生分钟线；开启后先拉原生分钟线，仅对其最早日期之前的缺失历史交易日进行补拉；原生返回空时，尝试起始日期至昨天的交易日。网络错误直接返回，不视为历史耗尽。

交易日来自同一品种可获取的日线，成交量为零的日期不请求分笔，因此不依赖调用方一定开启日线落库或配置交易日历。通过现有 `GetHistoryTradeDay` 获取分笔，复用 `Trades.Klines()` 合成 241 根：集合竞价标为 09:30，09:30 的成交归入 09:31，13:00 归入 13:01；空分钟沿用前收价、量额为零。分笔钟面时间会还原至固定东八区，同分钟按返回顺序处理。

这些是近似分钟线，不能保证高低价、成交额与原生分钟线一致，也无法从数据本身证明服务器未漏分笔。数据写入 `KlineMinute.Source="trade"`，原生来源为空字符串。先合并已有本地数据，只补缺少的源时间；本次返回的原生数据可覆盖合成值。继续使用原有删尾重插事务，不改变通用 Save 的写入规则。

分笔为空、无有效成交或请求失败时，返回带代码与日期的错误，保留缺口；已获取的原生分钟及其他日期合成结果仍按年落库，失败不会阻止其他年份提交覆盖范围。完整 241 根的已存日期重跑时跳过分笔请求。失败日期所在年份不提交本次兜底覆盖标记，市场也不会记为完成。

`MinuteCoverage` 新增 `TradeFallback` 与 `Through`（右开端点）；原生扫描不能跳过分笔兜底检查。可用 `QueryMinuteCoverage` 查询、`SaveMinuteFallbackComplete` 原子提交成功范围。跨年时，当年留下的标记若未覆盖至下一年零点，会重新检查年末。表结构由统一建表流程初始化；若当天市场已标记完成，仍遵守原去重规则，在下一更新窗口执行。

`must=true` 的原有行为保留：跳过交易日与整体检查，但仍检查市场级标记。默认更新窗口以固定东八区 15:01 分界。

### 执行与资源释放

同一个 `Service.Update` 中，港股、港股指数、美股和期货共享一次完整的扩展协议品种分页发现，再各自过滤市场代码；成功的空列表也仅缓存到本轮结束，失败不缓存。下一轮更新重新查询，不跨 Service 或连接池共享。在 `Update` 之外调用 `Service.GetInstruments()` 时重新查询；返回切片可修改，不影响其他市场的快照。同一个 Service 的并发 `Update` 调用顺序执行，仍按市场完成标记去重。

`Update(must ...bool)` 同步执行一次拉取，等待所有已调度任务结束后返回；不再接受 `context.Context`。
失败按 `Retry` 配置重试，耗尽后返回错误，该市场不会标记完成。
标准行情直接复用根包 `GetKlineUntil`、`GetKlineMinute241Until`，再通过 `StandardResponse()` 提取原始字段；连接池直接使用 `ClientPool.Do`。
正在进行的拉取等待底层响应或超时，不提供取消接口。

迁移调用方式：`s.Update(ctx)` → `s.Update()`，`s.Update(ctx, true)` → `s.Update(true)`。
自定义 `Unit` 的 `Codes/FetchDay/FetchMinute` 方法也需移除首个 `context.Context` 参数，见下方接口示例。

调用方应在更新结束后 `defer s.Close()`；内部行情引擎按最近使用顺序淘汰，使用中的查询/事务由引用计数保护。
外部传入的连接池与 `Tracker` 仍由调用方管理。

---

## 🔍 数据查询

```go
// 日线：源日期闭区间；空字符串表示不限制该端。
ks, err := s.QueryDay(code, "2026-01-01", "2026-01-31")

// 分钟线：源日期时分闭区间，按源年份定位文件。
ms, err := s.QueryMinute(code, "2026-01-05 09:30", "2026-01-05 15:00")

// 最后一条源日期／日期时分；空库返回空字符串。
lastDay, err := s.LastDayTime(code)
lastMinute, err := s.LastMinuteTime(code, 2026)

// 直接使用源日期时分，不转换为时间戳。
sourceTime := ms[0].Time

```

---

## 📊 周期合并

```go
// 日线 → 近似 N 日周期（固定 N 根分块，非日历对齐；5≈周、20≈月、60≈季、250≈年）
weekly := pull.DayToPeriod(ks, 5)

// 固定 N 根合并（不按交易时段对齐）
m5 := pull.MinuteToPeriod(ms, 5)

// 日历对齐：ISO 周（周一开始）、月、季、年
weeklyCalendar, err := pull.DayToCalendar(ks, pull.CalendarWeek)

// 按源日期时分和交易时段对齐，A 股集合竞价 09:30 独立保留，午休与跨日不混合
m5Aligned, err := pull.MinuteToSessions(ms, 5, []pull.TradingSession{
    {StartMinute: 9*60+30, EndMinute: 11*60+30},
    {StartMinute: 13*60, EndMinute: 15*60},
})
```

`DayToPeriod` 与 `MinuteToPeriod` 保留固定根数语义；`n<=1` 原样返回。
日线合并的股本取最后一根、换手率累加。新增日历/时段接口不修改输入，按时间排序，不填造缺失记录。
`MinuteToSessions` 使用收盘时刻 `(start,end]` 分桶，时段开始时刻的独立记录单独保留；时段外数据或重叠时段报错。
聚合接口不再接收 `loc`：日历聚合直接使用 `Time` 源日期；分钟聚合按源钟面时段分桶，不进行时区转换。跨午夜时段如 `{StartMinute: 21*60, EndMinute: 2*60+30}` 按时段开始日对齐，但不把它当成期货交易日。跨市场真实时刻比较需先确认来源时区。


---

## 🔌 自定义市场

```go
type MyUnit struct{}

func (u *MyUnit) Name() string { return "my" }
func (u *MyUnit) Codes(s *pull.Service) ([]pull.Code, error) { /* ... */ }
func (u *MyUnit) FetchDay(s *pull.Service, code pull.Code) error { /* ... */ }
func (u *MyUnit) FetchMinute(s *pull.Service, code pull.Code) error { /* ... */ }

func init() { pull.Register(&MyUnit{}) }
```

`Service` 提供调度、去重、查询和存储能力；Unit 负责调用协议、确定增量边界并调用 Save。
自定义历史分钟补全可使用 `MinuteYearComplete`/`SaveMinuteComplete`，只能在完整扫描成功后写完成标记。
参考实现：[`market/hk.go`](market/hk.go)（扩展行情）、[`market/a_stock.go`](market/a_stock.go)（标准行情）。

---

## 🧪 测试

```bash
go test ./extend/pull/... -count=1
```

> 存储/合并/单位换算均有单测（离线）；全市场拉取需联网，见 `examples/pull`。

内置证券代码与 `protocol.ParseInstrument` 共用解析规则，包括 `000001.SZ`、`BRK.B` 和中文交易所前缀。自定义 `market.code` 必须先注册对应 Unit；完整内置市场键与简称使用相同的规范化文件名，例如 `cn/stock.600000` 和 `sh600000` 均使用 `sh600000.db`。
