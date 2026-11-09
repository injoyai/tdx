# 🔄 行情数据拉取服务 (pull)

将通达信行情数据**增量拉取并持久化**（必须显式传入 `Store`）：编排多市场（`Unit` 可插拔）、并发、重试、增量去重、周期合并。

只存**日线 + 1分钟线**原始数据，其余周期（5/15/30/60 分钟、N 日等）由查询侧纯内存派生，不落盘。周期合并保留原始浮点精度，不转换成整数厘。

v2 的 K 线及历史分笔拉取统一放在本包。`Service` 负责 K 线拉取编排及存储调用，`Trade` 负责历史分笔及合成分钟线 CSV 下载导出。原 `extend.PullKline` 已移除，调用方式与旧数据库处理见 [v2 迁移指南](../../docs/v2-migration.md)。

---

## 历史分笔下载与 CSV 导出

```go
downloader := pull.NewTrade("./output/trade")
downloader.Pool = pool
downloader.Workday = calendar
downloader.CodeName = codeCache.GetName // 可选，用于导出名称
err := downloader.PullYear(ctx, 2025, "sz000001")
```

`ctx` 为 `context.Context`，`Pool` 为标准行情连接池，`Workday` 为提供 RangeYear 的历史交易日历接口；缺少其中任意组件会返回错误。`CodeName` 为可选的 `func(string) string`，未提供时导出名称为空。组件由调用方维护和关闭。`PullYear` 按指定年份下载，`Pull(ctx, startYear, endYear, code)` 只执行明确年份范围（含两端）；使用标准行情分笔及沪深 241 分钟合成规则。

输出仍按 `分时成交/1分钟/5分钟/15分钟/30分钟/60分钟/{代码}-{年份}.csv` 保存，保留原表头、UTF-8 BOM、竞价和午休边界。该入口复用 `extend/export` 编码，不创建 `Service` 或 SQLite 行情库；无需导入 `market` 注册市场。旧分笔下载包已移除，调用迁移见 [v2 迁移指南](../../docs/v2-migration.md#历史分笔下载并入-pull)。


## 🧩 架构

```
pull.Service ──编排──> Unit(市场) ──拉取──> 通达信服务器
     │                    │
     │                    └── a_stock / index / etf_lof / block (7709, 走 Pool)
     │                    └── future / hk / us          (7727, 走 tdx.ClientPool)
     │
     └──存储──> Store（调用方提供）
```

- **`Unit`**：一个可拉取的独立市场/品种类别。新增市场 = 新增一个实现 + `pull.Register`，无需改框架。
- **`Service`**：负责并发调度（有界并发 + 可选进度回调）、单条重试、按市场粒度的增量去重。市场间顺序执行，市场内代码并发；`Update` 只执行一次，定时调度由调用方安排。

---

## 🚀 快速开始

```go
import (
    "context"
    "github.com/injoyai/tdx/v2"
    "github.com/injoyai/tdx/v2/extend/pull"
    _ "github.com/injoyai/tdx/v2/extend/pull/market"
    "github.com/injoyai/tdx/v2/extend/storage/sqlite"
)

func syncExample(ctx context.Context, pool tdx.ClientPool) error {
    backend, err := sqlite.New("./output/pull")
    if err != nil { return err }
    defer backend.Close()
    service, err := pull.New(&pull.Config{
        Store: backend,
        Pool: pool,
        Codes: []string{"sz000001"},
        Day: true,
        StartAt: "20260101",
    })
    if err != nil { return err }
    defer service.Close()
    return service.UpdateContext(ctx, true)
}
```

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

内置扩展市场的交易所编号、简称及前缀解析复用 `protocol.Exchange`。通过 `pull.ParseCode` 构造 `Code`；拉取适配器只接受当前 Unit 支持的规范交易所前缀，期货仍保留 `cff/IF2609` 等代码形式及对应存储文件名。

### 市场 K 线配置

`Market.Info()` 返回行情协议、解码分类、日线/1 分钟线的请求类型和 `VolumeScale`，并用 `TickVolumeScale` 声明报价/分时/分笔倍率。内置有效倍率全部为正，集中在 [`protocol/market.go`](../../protocol/market.go)。

`Market.Info().TimeZone` 按市场枚举硬编码：国内市场（含当前国内期货）为 `Asia/Shanghai`，港股及港股指数为 `Asia/Hong_Kong`，美股为 `America/New_York`。全部内置枚举项均有时区，供调用方安排任务使用；`Info()` 保持 `(MarketInfo, bool)` 接口，不增加缺少时区的运行时报错逻辑。查证来源、用法及夏令时说明见 [统一接口文档](../../docs/unified-client.md)。`CloseTime` 另存当地收盘边界（`HH:MM`），不含更新延迟；pull 按下面的规则判断是否需要更新，不改变行情源时间字符串。

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

## 自定义存储

行情同步必须通过 `Config.Store` 注入 `storage.BarStore`；服务只依赖存储契约，不选择数据库、不创建文件。原 Dir 和 MaxCachedEngines 配置已移入具体存储实现。共享模型位于 model，接口契约、生命周期和原文件辅助方法的迁移见 [存储接口说明](../../docs/storage.md)。

## ⚙️ 配置项

| 字段 | 说明 | 默认值 |
|---|---|---|
| `Store` | 行情同步必填；仅获取元数据时可省略 | 无 |
| `Codes` | 拉取代码列表（自动路由市场，见 `pull.ParseCode`）；空 = 全部注册市场自动发现 | 自动发现 |
| `Day` / `Minute` | 是否拉日线 / 1分钟线 | 两者都 true |
| `TradeFallback` | 用历史分笔补充缺失分钟线，仅股票/ETF/LOF | `false` |
| `Goroutines` | 并发数 | `8` |
| `StartAt` | 起始日期 `YYYYMMDD` | 最近两年 |
| `Retry` | 每个阶段最大尝试次数，仅重试瞬时错误 | `tdx.DefaultRetry` |
| `RetryDelay` | 初始退避，指数递增、最多 30 秒；等待可取消 | `5s` |
| `OnProgress` | 串行逐证券完成回调 | 无，默认不打印进度 |
| `UniverseVersion` | 自动证券池版本，变化后允许同窗口补拉 | 空 |
| `Schedules` | 按市场覆盖 TimeZone、CloseTime 和 Delay | 内置市场使用原默认值 |
| `Pool` | 标准行情(7709)连接池 | 无（std 行情拉取需要） |
| `ExPool` | 扩展行情(7727)连接池 | 无（ex 市场需要） |
| `ListCodes` | 标准证券自动发现回调，每轮只调用一次；指定 Codes 时跳过 | 无 |

配置全部通过代码参数传入（本库作为第三方引用，不写配置文件）。`New` 会保存配置快照，后续修改原始 `Config` 不影响已创建的服务。

行情按源日期时分保存：日线 `Time="YYYY-MM-DD"`，分钟线 `Time="YYYY-MM-DD HH:MM"`，均为主键。查询、增量边界和年度分库使用源时间，不依赖主机 `TZ`。不额外保存 `TradingDate`，也不推断美股时区或期货夜盘交易日。

日线和分钟线均只保存源时间字符串与已确认的交易日；分钟线的 `Unix`、`TimeZone` 字段和 `Timestamp()` 已移除。存储只校验源日期时分格式、年份和范围，不应用夏令时或来源时区规则。已有 `source-time-v1` 库可以继续读写；历史额外列不再映射或使用，也不会自动删列。

`StartAt` 为空时使用东八区当前日期推算默认起始日期；非空值必须是合法的 `YYYYMMDD` 日期，否则创建服务会直接返回错误，不创建数据库。

### 显式获取与内存查询

New 仅保存依赖并校验配置，不联网、不启动定时任务。行情同步需要 Store；仅获取元数据时使用 `pull.New(&pull.Config{Pool: pool})` 即可，无需存储或市场注册。应用显式调用：

```go
codeSnapshot, err := s.FetchCodes(ctx) // 默认沪深北；可明确传入交易所范围
if err != nil { return err }
codeIndex, err := metadata.NewCodes(codeSnapshot.Rows)
if err != nil { return err }
events, err := s.FetchGbbq(ctx, []string{"sh600000"})
if err != nil { return err }
equity, err := metadata.NewGbbq(events.Rows)
if err != nil { return err }
calendarSnapshot, err := s.FetchWorkdays(ctx) // 当前仅从上证指数提取中国历史日期
if err != nil { return err }
calendar, err := metadata.NewWorkday(calendarSnapshot)
if err != nil { return err }
_ = codeIndex
_ = equity
_ = calendar
```

行情同步默认不输出进度条。Config.OnProgress 接收串行的逐证券完成事件；Completed 包括最终失败项，Err 表示该证券的执行错误。回调应及时返回，不能同步再次调用本 Service.Update。examples/pull 演示把进度写到 stderr。

三个 Fetch 方法只获取并校验快照，不写数据库；失败返回零值快照，成功返回的 FetchedAt 表示获取完成时间。应用可直接构造查询对象，也可先显式调用 SQLite 的 ReplaceCodes、ReplaceGbbq、SaveWorkdays 保存，再发布结果。保存失败时继续保留旧查询对象。

行情同步使用 UpdateContext(ctx, must...)，Update 是 Background 的便捷入口。标准证券自动发现必须提供 ListCodes(ctx)，每轮只调用一次并固定结果；回调可从应用内存、数据库或 FetchCodes 的结果生成 model.Code 列表。板块从板块文件发现，扩展市场从 ExPool 发现。指定 Codes 时跳过代码发现。

日线不再回填 FloatStock、TotalStock、Turnover。历史股本与换手率由 gbbq 查询／计算，缺失股本返回不可用，不从当前数据回填历史。manage、CodeCache、Config.Gbbq 已删除，应用直接组装组件。详见[架构说明](../../docs/data-architecture.md)。

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

## 数据与存储边界

文件布局、建表、连接缓存和事务实现由具体 Store 负责；内置 SQLite 在主模块的 [storage/sqlite](../storage/sqlite/README.md) 单独包。MySQL 由用户实现 Store。本包负责同步编排；model 提供共享数据，storage 定义持久化契约。

### 单位约定

| 字段 | 单位 |
|---|---|
| 价格（Open/High/Low/Close） | 证券原币种或报价单位（float64） |
| Volume | 直接使用统一 Client 的最小单位：股票为股，期货为张；可转债等国内默认项尚未逐接口校准 |
| Amount | *float64 原币种成交额；nil 表示未知，非 nil 的 0 表示真实零 |
| Source（分钟线） | 空字符串=原生数据；`trade`=分笔合成，聚合结果含合成输入时保留该标记 |

> 与本地 `.day/.lc1/.lc5` 文件的单位不同。国内默认 ×100 不是单位校准结论，不能仅依据字段名宣称其已验证为股数；港股指数不作为成交数量输出。

拉取层直接保存统一 `tdx.Kline` 的时间、价格和成交量，不再提取 `Standard` / `Extended` 后重复换算，也不受旧 `StandardResponse()` 的 uint16 记录数限制。Client 的 `Amount=nil` 保留为 nil，SQLite 写入 SQL NULL，不额外增加状态字段。扩展原始 `Amount` 与持仓字节重叠，不能作为成交额；需要原始字段时使用 Client 记录的 `Extended`。

周期合并仅在全部组成记录的 Amount 都已知时求和，任一记录为 nil 则合计为 nil；真实零仍是有效金额。聚合不修改输入金额，价格和金额都不做汇率换算。已有 `source-time-v1` 数据库可继续使用，多余的旧 `TradingDate` / `AmountUnknown` 列不再映射，新库不创建它们，不自动删列或修改历史值。旧库中既有 0 仍读为非 nil 的 0，不从历史值猜测其有效性；曾自行写入非零未知金额的旧库需由调用方核对后重拉。

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

### 收盘后更新

市场枚举保存当地收盘边界，pull 统一延迟 5 分钟后更新：

| 市场 | `CloseTime` | 允许更新的当地时间 |
|---|---|---|
| 国内市场（含当前国内期货） | `15:30` | `15:35` 起 |
| 港股、港股指数 | `16:10` | `16:15` 起 |
| 美股（包含盘后时段） | `20:00` | `20:05` 起，按纽约时区处理夏令时 |

上述时间是本项目约定的市场分组边界，不是逐品种交易时段表。普通 `Update()` 在市场当地当天的允许时间之前跳过该市场，之后首次调用才发起拉取；调用方应在相应时间之后、当地午夜之前安排调用。服务不会自行等待、休眠或启动定时任务。非交易日仍允许请求一次，不用中国交易日历过滤其他市场。

完成标记由 `Config.Store` 的 `LoadUpdate/SaveUpdate` 统一读写，按任务配置和市场隔离：只有该市场全部任务成功才记录，失败可在下次调用重试。成功后保存本轮开始时选定的更新窗口边界（Unix 秒），不使用任务结束时的墙上时钟；标记达到当前市场更新边界（含）才算完成；读取失败不拉取该市场，写入失败返回错误并允许下次重试。旧整体标记保留在库中但不再使用，避免早收盘市场挡住晚收盘市场。不再单独配置 Tracker。

`Update(true)` 允许提前拉取，但仍按最近一次已到达的更新边界去重；提前拉取只认证前一窗口，即使任务跨过当天更新边界才结束，也不会阻止当天收盘后的更新。收盘后只拉一次，并不保证行情服务器已提供所有盘后记录；这些时间不会扩展数据源本身的覆盖范围。

自动发现返回空代码列表时不写完成标记，下次允许重新发现。应用在同一天发布不同证券池时，应更新 UniverseVersion 并重新构造 Service；相同版本继续按原规则去重。版本表示证券范围，名称变化无需更新。

### 执行与资源释放

同一个 `Service.Update` 中，港股、港股指数、美股和期货通过 Client.GetInstruments 共享一次完整品种发现，SDK 负责协议分页及完整性校验，再各自过滤市场代码；成功的空列表也仅缓存到本轮结束，失败不缓存。下一轮更新重新查询，不跨 Service 或连接池共享。同一轮并发发现共用在途结果，等待可响应本轮 context 取消；发现失败后后续调用可重试。状态锁不覆盖连接池等待或协议请求；独立查询捕获自己的 context，其迟到结果不会发布到后来的更新轮次。在 `Update` 之外调用 `Service.GetInstruments()` 时重新查询；返回切片可修改，不影响其他市场的快照。同一个 Service 的并发 `Update` 调用顺序执行，仍按市场完成标记去重。

`Update(must ...bool)` 同步执行一次拉取，等待所有已调度任务结束后返回；不再接受 `context.Context`。
超时、连接中断等瞬时错误按 Retry 配置重试；自定义错误可实现 Temporary() bool 并返回 true。配置、数据校验等其他错误直接返回。退避初始为 RetryDelay，默认五秒，倍增至最多三十秒；等待可取消。耗尽后返回错误，该市场不会标记完成。
标准行情直接复用根包 `GetKlineUntil`、`GetKlineMinute241Until`，保存统一结果中的量价和源时间；行情、元数据和分笔统一通过 `ClientPool.DoContext` 借用连接。`UpdateContext(ctx, must...)` 支持取消连接等待；原生 SDK 的请求视图也会取消响应等待和后续分页、逐日查询。自定义 Client 可实现 WithContext(context.Context) tdx.Client，否则其单次请求仍须有超时。取消后不继续提交行情或完成标记。

同一 Service 的更新仍串行执行。等待前一轮 Update 的 UpdateContext 也可被 context 取消或超时，不必等活动任务结束，且不会替换活动任务的 context。

迁移调用方式：`s.Update(ctx)` → `s.UpdateContext(ctx)`，`s.Update(ctx, true)` → `s.UpdateContext(ctx, true)`；无需取消时使用 `s.Update()`。
自定义 `Unit` 的 `Codes/FetchDay/FetchMinute` 方法也需移除首个 `context.Context` 参数，见下方接口示例。

调用方应在更新结束后 `defer s.Close()`；SQLite 后端的行情引擎按最近使用顺序淘汰，使用中的查询/事务由引用计数保护。
外部传入的连接池和各类 Store 均由调用方管理，构造失败或 `Service.Close()` 时也不会关闭它们。先等待拉取结束并关闭 Service，再关闭共享组件；Service 不持有任何自建存储。

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
日线合并只处理原始量价字段。新增日历/时段接口不修改输入，按时间排序，不填造缺失记录。
`MinuteToSessions` 使用收盘时刻 `(start,end]` 分桶，时段开始时刻的独立记录单独保留；时段外数据或重叠时段报错。
聚合接口不再接收 `loc`：日历聚合直接使用 `Time` 源日期；分钟聚合按源钟面时段分桶，不进行时区转换。跨午夜时段如 `{StartMinute: 21*60, EndMinute: 2*60+30}` 按时段开始日对齐，但不把它当成期货交易日。跨市场真实时刻比较需先确认来源时区。


---

## 🔌 自定义市场

```go
type MyUnit struct{}

func (u *MyUnit) Name() string { return "my" }
func (u *MyUnit) Schedule() pull.Schedule {
    return pull.Schedule{TimeZone: "America/New_York", CloseTime: "20:00", Delay: 5*time.Minute}
}
func (u *MyUnit) Codes(s *pull.Service) ([]model.Code, error) { /* ... */ }
func (u *MyUnit) FetchDay(s *pull.Service, code model.Code) error { /* ... */ }
func (u *MyUnit) FetchMinute(s *pull.Service, code model.Code) error { /* ... */ }

func init() { pull.Register(&MyUnit{}) }
```

自定义市场必须通过 Config.Schedules 或可选 Schedule() pull.Schedule 方法声明调度；缺失时 New 返回错误。Config.Schedules 优先于内置默认值及 Unit 方法。显式 Schedule.Delay=0 表示不延迟；非法时区、时间、负延迟或延迟跨入次日的配置在构造时拒绝。

`Service` 提供调度、去重、查询和存储能力；Unit 负责调用协议、确定增量边界并调用 Save。
自定义历史分钟补全可使用 `MinuteYearComplete`/`SaveMinuteComplete`，只能在完整扫描成功后写完成标记。
参考实现：[`market/hk.go`](market/hk.go)（扩展行情）、[`market/a_stock.go`](market/a_stock.go)（标准行情）。

---

## 🧪 测试

```bash
go test ./pull/... -count=1
```

> 存储/合并/单位换算均有单测（离线）；全市场拉取需联网，见 `examples/pull`。

内置证券代码与 `protocol.ParseInstrument` 共用解析规则，包括 `000001.SZ`、`BRK.B` 和中文交易所前缀。自定义 `market.code` 必须先注册对应 Unit；完整内置市场键与简称使用相同的规范化文件名，例如 `cn/stock.600000` 和 `sh600000` 均使用 `sh600000.db`。
