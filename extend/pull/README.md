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

- **全量模式**（`Codes` 为空，上例）：已启用的市场各自自动发现代码。协议市场在对应连接池可用且市场定义有调度时自动启用。也可通过 `Schedules` 显式启用路由，例如国际指数。
- **白名单模式**（`Codes` 非空）：只拉列出的代码，市场由服务构造时解析；未涉及的市场不拉。

---

## 🗺 内置市场

| 枚举常量 | 值（即存储目录） | 市场 | 连接 | 说明 |
|---|---|---|---|---|
| `protocol.MarketSH` | `sh` | 上海 | 7709 | 含股票、指数、基金、转债和板块指数 |
| `protocol.MarketSZ` | `sz` | 深圳 | 7709 | 含股票、指数、基金和转债 |
| `protocol.MarketBJ` | `bj` | 北京 | 7709 | 按证券代码选择品种规则 |
| `protocol.MarketCFF`、`MarketCZC`、`MarketDCE`、`MarketSHF`、`MarketGFE`、`MarketQHZ` | `cff`、`czc`、`dce`、`shf`、`gfe`、`qhz` | 各期货交易所及期货类指数 | 7727 | 合约代码保留大小写 |
| `protocol.MarketHK` | `hk/stock` | 港股主板 | 7727 | `GetKline` |
| `protocol.MarketHKFund` | `hk/fund` | 香港基金（含 ETF） | 7727 | 市场编码 49，由通达信品种快照确认 |
| `protocol.MarketHKIndex` | `hk/index` | 港股指数（恒生系/中华系） | 7727 | `GetKline`，市场编码27，含 HSI/VHSI/CES100 等 |
| `protocol.MarketUS` | `us/stock` | 美股 | 7727 | `GetKline`，股票/ETF/指数混合（协议层无法区分） |
| `protocol.MarketCSI` | `csi` | 中证指数 | 7727 | 市场编码 62，如 `csi.000300`，通过 `ExPool` 同步日线和 1 分钟 K 线，量为股、额为元 |
| `protocol.MarketInternationalIndex` | `global/index` | 国际指数 | 7727 | 市场编码 12，如 `wi.A_IXIC`。通过 `Config.Schedules` 配置调度 |

市场键由 `protocol.MarketID` 常量表示，也是 SQLite 的市场目录。通过 `pull.ParseCode` 构造已明确归属的 `Code`。例如 `sh.510300` 存为 `{Market: "sh", Code: "sh510300"}`，`cff.IF2609` 存为 `{Market: "cff", Code: "IF2609"}`。

内置市场在 [`protocol/market/`](../../protocol/market) 逐个声明。增加复用现有报文格式的市场时，在一个文件注册市场键、协议编号、代码规则、量价单位和调度。`pull` 按路由键和协议类型直接选用标准或扩展适配器。

### 市场 K 线配置

`MarketID.Info()` 返回市场简称、协议路由及默认调度资料。`Instrument.Kind()` 返回证券品种，`Instrument.Rules()` 返回当前证券的行情类型、周期和量价换算。 调用方通过 `rules.Kline(category, mode).Supported` 和各类行情的 `Supported` 检查 SDK 支持范围，读取 `Fields` 确认价格、成交量和成交额的可用性。

`MarketID.Info().TimeZone` 读取市场定义。国内市场和当前国内期货为 `Asia/Shanghai`，港股、香港基金及港股指数为 `Asia/Hong_Kong`，美股为 `America/New_York`。国际指数跨多个地区，`Info()` 中没有统一时区和收盘边界。显式选择国际指数代码时，须为 `protocol.MarketInternationalIndex` 配置对应代码范围的更新边界。查证来源、用法及夏令时说明见 [统一接口文档](../../docs/unified-client.md)。`CloseTime` 另存当地收盘边界（`HH:MM`），不含更新延迟；pull 按下面的规则判断是否需要更新，不改变行情源时间字符串。

```go
code, err := pull.ParseCode("hk/stock.00700")
if err != nil {
    panic(err)
}
instrument, err := code.Instrument()
if err != nil {
    panic(err)
}
info, ok := instrument.Rules()
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
| 可转债 | 9 / ×1000 转张 | 7 / ×10 转张 |
| 其他国内品种 | 9 / 默认 ×100 | 7 / 默认 ×100 |
| 标准指数 | 9 / 解码量 ×100，与统一报价口径一致 | 7 / 成交量不可用，统一 Volume 为 0 |
| 板块指数 | 9 / 保留解码量，单位未统一校准 | 7 / 保留解码量，单位未统一校准 |
| 港股主板、香港基金 | 4 / 解码量 ×100 | 8 / 解码量 ×100 |
| 期货 | 4 / 张（×1） | 8 / 张（×1） |
| 美股 | 4 / 股（×1） | 8 / 股（×1） |
| 港股指数 | 4 / 成交量不可用，统一 Volume 为 0 | 8 / 成交量不可用，统一 Volume 为 0 |
| 中证指数（62） | 4 / 百万股 | 8 / 百万股 |

`VolumeScale=0` 表示协议不提供成交量，`VolumeScale=1` 保留解码量，`100` 为百股换算或国内默认倍率。`ToStorageVolume` 返回 `(int64,error)`，拒绝零/负倍率、负数量和溢出。港股指数 K 线保留点位，报文金额 ×1,000,000 转为原币种。注册市场的品种通过 `Instrument.Rules()` 获得量额规则。

沪深北及期货按新的交易所目录写入。旧 `cn/*` 数据目录不参与当前读写。可转债历史分笔和合成分钟线 CSV 的数量列为张。

标准行情由 `GetKline*` 按代码选择解码类型。股票/ETF 分钟拉取使用 `GetKlineMinute241Until` 保留竞价拆分；可转债、其他国内品种及指数走普通分页，不套用股票竞价拆分。K 线与报价/分时/分笔使用各自声明的倍率，本地文件仍遵循自身协议单位。

标准指数的日、周、月、季、年统一量均为解码量 ×100。港股主板、香港基金和美股周、月、季、年线由统一 Client 支持，香港市场量按 ×10000、美股量按 ×100，成交额使用原币种。拉取层仍只保存日线与 1 分钟线。港股日线类型 4 已用腾讯控股单根、五根请求及独立来源核对；整数百股精度无法恢复零股。类型 9 的该样本首条量字段为 43977，其他量近似万股，不能直接沿用手转股的换算。未知项需取得对应市场及周期的样本后调整配置。

---

## 自定义存储

行情同步必须通过 `Config.Store` 注入 `storage.BarStore`；服务只依赖存储契约，不选择数据库、不创建文件。原 Dir 和 MaxCachedEngines 配置已移入具体存储实现。共享模型位于 model，接口契约、生命周期和原文件辅助方法的迁移见 [存储接口说明](../../docs/storage.md)。

## ⚙️ 配置项

| 字段 | 说明 | 默认值 |
|---|---|---|
| `Store` | 行情同步必填；仅获取元数据时可省略 | 无 |
| `Codes` | 拉取代码列表（构造服务时解析市场）；空时自动发现已启用的市场 | 自动发现 |
| `Day` / `Minute` | 是否拉日线 / 1分钟线 | 两者都 true |
| `TradeFallback` | 用历史分笔补充缺失分钟线，仅股票/ETF/LOF | `false` |
| `Goroutines` | 并发数 | `8` |
| `StartAt` | 起始日期 `YYYYMMDD` | 最近两年 |
| `Retry` | 每个阶段最大尝试次数，仅重试瞬时错误 | `tdx.DefaultRetry` |
| `RetryDelay` | 初始退避，指数递增、最多 30 秒；等待可取消 | `5s` |
| `OnProgress` | 串行逐证券完成回调 | 无，默认不打印进度 |
| `OnEvent` | 串行日线／分钟阶段及市场终态回调 | 无 |
| `UniverseVersion` | 自动证券池版本，变化后允许同窗口补拉 | 空 |
| `Schedules` | 按市场覆盖 TimeZone、CloseTime 和 Delay | 使用市场时区和收盘时间，默认延迟 5 分钟。未声明调度的市场须显式配置 |
| `Pool` | 标准行情(7709)连接池 | 无（std 行情拉取需要） |
| `ExPool` | 扩展行情(7727)连接池 | 无（ex 市场需要） |
| `ListCodes` | 标准证券自动发现回调，每轮只调用一次；指定 Codes 时跳过 | 无 |

配置全部通过代码参数传入（本库作为第三方引用，不写配置文件）。`New` 会保存配置快照，后续修改原始 `Config` 不影响已创建的服务。

行情按源日期时分保存：日线 `Time="YYYY-MM-DD"`，分钟线 `Time="YYYY-MM-DD HH:MM"`，均为主键。查询、增量边界和年度分库使用源时间，不依赖主机 `TZ`。不额外保存 `TradingDate`，也不推断美股时区或期货夜盘交易日。

日线和分钟线均只保存源时间字符串与已确认的交易日；分钟线的 `Unix`、`TimeZone` 字段和 `Timestamp()` 已移除。存储只校验源日期时分格式、年份和范围，不应用夏令时或来源时区规则。SQLite 使用 `source-time-v2` 目录；旧 `source-time-v1` 布局不兼容，须使用新目录，详见[存储说明](../../docs/storage.md#文件与旧库)。

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

需要区分日线、分钟线和市场跳过时，配置 `OnEvent`；原 `OnProgress` 的频率和计数保持不变：

```go
cfg.OnEvent = func(e pull.Event) {
    // Stage 为空为市场结果，否则为 day 或 minute 阶段结果。
    // Result.Known=false 的第三方旧 Unit 没有记录数，不应当作返回零条。
    fmt.Fprintf(os.Stderr, "%s %s %s %s fetched=%d saved=%d latest=%s reason=%s err=%v\n",
        e.Market, e.Code.Code, e.Stage, e.Status,
        e.Result.Fetched, e.Result.Saved, e.Result.LatestTime, e.Reason, e.Err)
}
```

事件是终态结果，状态为 `skipped/no_data/success/failed`；Fetched 是最后一次尝试主扫描返回给保存流程的行数，Saved 是该尝试成功保存的行数（含覆盖），不是净新增。最新源时间保持原字符串；重试次数由 Attempts 单独提供。所有回调串行且不得同步重入更新方法。完整字段、跳过原因和自定义 `ResultUnit` 约定见[公开契约](../../docs/public-contracts.md#更新与事件)。

三个 Fetch 方法只获取并校验快照，不写数据库；失败返回零值快照，成功返回的 FetchedAt 表示获取完成时间。应用可直接构造查询对象，也可先显式调用 SQLite 的 ReplaceCodes、ReplaceGbbq、SaveWorkdays 保存，再发布结果。保存失败时继续保留旧查询对象。

行情同步使用 UpdateContext(ctx, must...)，Update 是 Background 的便捷入口。标准证券自动发现必须提供 ListCodes(ctx)，每轮只调用一次并固定结果；回调可从应用内存、数据库或 FetchCodes 的结果生成 model.Code 列表。板块从板块文件发现，扩展市场从 ExPool 发现。指定 Codes 时跳过代码发现。

日线不再回填 FloatStock、TotalStock、Turnover。历史股本与换手率由 gbbq 查询／计算，缺失股本返回不可用，不从当前数据回填历史。manage、CodeCache、Config.Gbbq 已删除，应用直接组装组件。详见[架构说明](../../docs/data-architecture.md)。

### 代码自动路由

`Codes` 是扁平的代码列表。构造服务时解析每个代码的市场。含 `hk.00823` 等共享简称的五位数字代码时，传入 `Config.InstrumentSnapshot` 可复用调用方已有的品种快照，否则构造函数通过 `ExPool` 读取。可以使用 `pull.NewContext` 设定读取期限。读取失败、代码缺失或同时归属 31 和 49 时，构造函数返回错误。

```go
Codes: []string{
    "sz000001", // 沪深股票（带前缀）
    "600000",   // 沪深股票（6 位裸数字，自动补 sh 前缀）
    "510300",   // ETF（自动补前缀并分类）
    "399001",   // 深证成指（指数）
    "880001",   // 板块指数
    "00700",    // 港股，以品种清单确认协议市场
    "hk.00823", // 领展，清单中的协议市场为 49
    "hk02800",  // 盈富基金（香港 ETF，市场 49）
    "HSI",      // 恒生指数（知名港股指数白名单）
    "AAPL",     // 美股（纯字母）
    "cff.IF2609", // 期货（带交易所前缀）
}
```

内置市场统一使用 `protocol.ParseInstrument` 校验：扩展代码最多 9 字节，美股须含字母，期货合约须同时含字母和数字，期货类指数接受纯字母代码；`hk/index` 等完整市场键必须与实际市场匹配，非法输入在创建数据库前返回错误。同一证券的合法输入统一规范化为同一代码和存储文件名，自定义市场保留自身代码规则。

服务建好后使用 `s.ParseCode` 复用同一快照得到查询存储所需的 `model.Code`。没有快照时，`ParseCode` 可以离线解析已导入或内置的五位数字港股，例如 `hk.00700`、`hk.00823` 和 `hk.02800`。未收录的代码需要用 `ParseCodeWithSnapshot` 并传入由 `protocol.NewInstrumentSnapshot` 建立的索引。传入快照时，以快照中的归属为准。

`model.Code.Key()` 对港股股票、基金和指数统一使用 `hk` 前缀，例如 `hk.00700`、`hk.02800`、`hk.HSI`。存储接口仍使用市场类别和代码主体来标识证券，如 `{hk/stock, 00700}` 和 `{hk/index, HSI}`。

> 注意：`000001` 按股票（平安银行 `sz000001`）路由；上证指数需带前缀写 `sh000001`。
> 其余港股指数（HZ50xx/CESxxx 等）用完整键指定，如 `hk/index.CES120`。香港基金的存储目录为 `hk/fund`。

---

## 数据与存储边界

文件布局、建表、连接缓存和事务实现由具体 Store 负责；内置 SQLite 在主模块的 [storage/sqlite](../storage/sqlite/README.md) 单独包。MySQL 由用户实现 Store。本包负责同步编排；model 提供共享数据，storage 定义持久化契约。

### 单位约定

| 字段 | 单位 |
|---|---|
| 价格（Open/High/Low/Close） | 证券原币种或报价单位（float64） |
| Volume | 直接使用统一 Client 的最小单位：股票为股，可转债和期货为张；其他国内默认项尚未逐接口校准 |
| Amount | *float64 原币种成交额；nil 表示未知，非 nil 的 0 表示真实零 |
| Source（分钟线） | 空字符串=原生数据；`trade`=分笔合成，聚合结果含合成输入时保留该标记 |

> 与本地 `.day/.lc1/.lc5` 文件的单位不同。国内默认 ×100 不是单位校准结论，不能仅依据字段名宣称其已验证为股数；港股指数不作为成交数量输出。

拉取层直接保存统一 `tdx.Kline` 的时间、价格、成交量和成交额。港股主板和美股日线、分钟线随新拉取或重拉写入原币种成交额。Client 的 `Amount=nil` 在 SQLite 写入 SQL NULL。历史空值需要重拉对应日期或分钟区间。

周期合并仅在全部组成记录的 Amount 都已知时求和，任一记录为 nil 则合计为 nil；真实零仍是有效金额。聚合不修改输入金额，价格和金额都不做汇率换算。SQLite 使用新 `source-time-v2` 目录，旧 `source-time-v1` 不兼容；通过显式导入保留的旧数据中，多余的旧 `TradingDate` / `AmountUnknown` 列不再映射，新库不创建它们，不自动删列或修改历史值。旧库中既有 0 仍读为非 nil 的 0，不从历史值猜测其有效性；曾自行写入非零未知金额的旧库需由调用方核对后重拉。

### 增量写入

拉取时按库内最后源时间增量拉取，写入时**删除 `Time>=from` 后批量插入**（事务内幂等）；
空数据不写库（避免产生空库文件）。

分钟线对所有待补年份共用一次从新到旧的分页，股票/ETF 的集合竞价逐笔查询也不再按年份重复。
历史年份的 `minuteCoverage` 表与 K 线在同一事务中提交。开启 `TradeFallback` 时，股票/ETF 的兜底检查完成后，当年再按最后源时间增量；未完成时仍补查早期历史。
新格式库若没有完成标记，会重新扫描认证；补拉只覆盖本次实际返回的时间段，保留服务器已不提供的本地历史前缀。
空结果、拉取失败以及扩展行情偏移超限均不会写成功标记。`LastDayTime/LastMinuteTime` 不再为不存在的文件建库。
完成标记表示成功处理了服务器当前可提供的数据，不保证其保留了所请求区间的全部历史；空年份会在后续执行中重新检查。

扩展行情的源时区未知。北京时间 1 月 1 日 20:00 前，上一年仍按进行中的年份增量更新，跳过历史完成判定；新一年的记录仍按返回年份保存。旧覆盖标记没有扫描时间，升级后常规复查仍会沿用已经过去的年份的覆盖标记。若怀疑这些年度缺少年末分钟线，可在服务端仍提供数据时用 `GetKlineRange` 查询对应日期，核对后通过应用合并或重建受影响的范围。

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

需要同窗口再次检查迟到数据时，使用 `s.UpdateWithOptions(ctx, pull.UpdateOptions{Recheck: true})`。Recheck 不放宽收盘限制；需要提前复查时同时设置 AllowEarly。它仍按原增量边界拉取，保留历史分钟覆盖和空扫描缓存，不重扫全部历史。零选项等价于普通更新；并发排队和取消行为与 UpdateContext 相同。复查失败保留旧成功标记，再试须继续设置 Recheck。

自动发现返回空代码列表时不写完成标记，下次允许重新发现。应用在同一天发布不同证券池时，应更新 UniverseVersion 并重新构造 Service；相同版本继续按原规则去重。版本表示证券范围，名称变化无需更新。

### 执行与资源释放

同一个 `Service.Update` 中，港股、港股指数、美股和期货共享 `Client.GetInstruments()` 返回的可读品种列表，再各自过滤市场代码。SDK 负责协议分页和[报告总数与实读数的尾部核验](../../docs/unified-client.md#扩展品种发现)。成功的空列表也仅缓存到本轮结束，失败不缓存。下一轮更新重新查询，不跨 Service 或连接池共享。同一轮并发发现共用在途结果，等待可响应本轮 context 取消；发现失败后后续调用可重试。状态锁不覆盖连接池等待或协议请求；独立查询捕获自己的 context，其迟到结果不会发布到后来的更新轮次。在 `Update` 之外调用 `Service.GetInstruments()` 时重新查询；返回切片可修改，不影响其他市场的快照。同一个 Service 的并发 `Update` 调用顺序执行，仍按市场完成标记去重。

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

### 复用 7727 日线和分钟 K 线

应用启动时从 Go 配置或 JSON 文件创建市场对象，然后调用一次 `protocol.Register`。下面的市场号、代码规则和倍率用于演示，接入时填入已确认的市场参数。

```go
m, err := protocol.NewMarket(protocol.MarketDef{
    Key: "demoex", Exchange: 252, CodePattern: `[A-Z][a-z][0-9]{2}`,
    DayVolumeScale: 10, MinuteVolumeScale: 2,
    DayAmountScale: 100, MinuteAmountScale: 20,
    TimeZone: "Asia/Shanghai", CloseTime: "15:00",
})
if err != nil {
    return err
}
if err := protocol.Register(m); err != nil {
    return err
}

// client 由应用创建，例如 tdx.NewClient() 或对应来源的 tdx.DialEx。
if _, err := client.GetKlineDay("demoex.Aa12", 0, 1); err != nil {
    return err
}
s, err := pull.New(&pull.Config{
    Store: store,
    ExPool: exPool,
    Codes: []string{"demoex.Aa12"},
})
if err != nil {
    return err
}
defer s.Close()
return s.UpdateContext(ctx)
```

省略 `Codes` 时，`pull` 从扩展品种表按协议市场号发现代码。同一逻辑市场的多条路由分别使用各自的 `MarketID` 查询 Unit、配置调度及存储。例如港股 31、49、27 分别使用 `hk/stock`、`hk/fund`、`hk/index`。显式代码可用共享前缀 `hk.02800` 或路由键 `hk/fund.02800`。用 `pull.Get` 按路由键查询，用 `pull.Units` 枚举全部路由及应用 Unit。

每个市场默认使用对象声明的时区、收盘时间和 5 分钟延迟，`Config.Schedules` 可以按路由键覆盖。跨时区市场可省略默认调度，由应用提供覆盖项。

`CodePattern` 匹配完整代码，代码保留大小写且最多 9 字节。量或额倍率为 0 时该字段不可用。也可用 `AmountScale` 同时声明日线和分钟线的成交额倍率。更复杂的路由和数据规则见 [统一 Client](../../docs/unified-client.md#注册行情市场)。

市场定义绑定数据集的历史行情和覆盖记录。SQLite 每条路由只接受一个绑定。首次绑定写入失败可重试，已有绑定须与当前定义指纹一致。相同路由键改变协议编号、代码规则或量额换算后，更新会要求使用独立数据集或显式重建。应用实现 `protocol.Market` 时应通过 `MarketInfo.Version` 标识代码归属和量额等持久数据规则的版本。

### 实现自己的拉取流程

启动时先注册协议市场，应用专有流程再调用 `pull.Register(Unit)`。Unit 名称与已注册协议路由键、市场别名或其他 Unit 重名时会 panic，名称匹配忽略大小写。应用若晚于 Unit 注册同名协议市场，随后的查找和新服务会以协议市场为准。


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
参考实现：[`market/extended.go`](market/extended.go)（扩展行情）、[`market/standard.go`](market/standard.go)（标准行情）。

---

## 🧪 测试

```bash
go test ./extend/pull/... -count=1
```

> 存储/合并/单位换算均有单测（离线）；全市场拉取需联网，见 `examples/pull`。

协议市场的证券代码与 `protocol.ParseInstrument` 共用解析规则，只有 `bj`、`sz`、`sh`、`hk` 可以直接拼接。其他市场使用点分隔，如 `us.BRK.B`、`cff.IF2609`、`demoex.Aa12`。应用专有 Unit 使用 `unit.code`。完整内置市场键与简称使用相同的规范化文件名，例如 `sh.600000` 和 `sh600000` 的日线均使用 `sh/day/sh600000.db`。
