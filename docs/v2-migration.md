# v2 迁移指南

## 公开接口契约收敛

本轮开发版接口直接调整，不新增旧名称转发：

| 原用法 | 当前用法 |
| --- | --- |
| 直接访问具体连接的 Client／Wait／Logger／Event，调用 Reset／Restart／Read 等上游方法 | 内部字段已私有化；连接仅保留业务接口、SendFrame、Write 系列及 Done／Closed／Err／GetKey。协议与日志配置通过构造选项传入，关闭使用 Close |
| StartTimer(spec, retry, updater) | StartTimer(spec, retry, func() error { return service.Update() })；普通更新函数可直接传入，nil 回调报错 |
| protocol.Kline.RiseRate() 返回百分数 | 接收 (ratio, available)，0.01 表示 1%；需要百分数时显式乘 100。不可用和真实零区分，与 extend.Income 一致 |
| metadata.NewCodes(records)／index.Update(records) | 构造接收 (*Codes, error)，替换检查 error；输入须为规范且不重复的沪深北记录，失败保留旧索引，nil 列表有效 |
| model.KlineDay／KlineMinute.Amount 为 float64 | 改为 *float64，未知为 nil，真实零传入零值地址；SQLite 保存为 NULL，原列名和表布局保留，不改写历史零值 |
| 同花顺组合接口要求 tdx.Client | 改为 spider.DaySource，只需实现 GetKlineDayAll；完整 Client 仍可直接传入 |

周期聚合遇到未知金额时结果仍未知，不返回不完整金额之和。原有合法量价、周期便捷入口和本地文件单位不变。旧库的零值无法仅凭数值恢复其原有效性，保留已有值，由应用按明确范围另行核对。

## 存储与元数据边界修复

| 入口 | 当前行为与迁移影响 |
| --- | --- |
| SQLite 行情路径 | 在读写前拒绝越界市场路径和不安全文件名；自定义 `A/B` 返回错误，避免覆盖 `A-B`。期货使用规范 `cff/IF2609` 输入及无大小写歧义的编码布局；旧 source-time-v1 目录拒绝打开，须使用新目录；分钟年份限 1..9999。旧碰撞文件不自动修复 |
| httpserver.CodesSource | 改为只要求 `Snapshot() model.CodeModels`，自定义来源须提供完整独立副本；`*metadata.Codes` 可直接传入 |
| `/codes` | 一次快照返回全部证券，包含债券／其他品种，顺序改为输入快照顺序；分类路由也只读取一次快照，空分类返回 `[]` |
| httpclient.FetchCodes | 完整校验规范沪深北代码、重复证券和空行，非法响应返回 nil 和错误；空列表／null 仍有效 |
| pull.UpdateContext | 等待同一 Service 的前一轮更新时也可取消或超时，不影响活动任务 |
| local.ReadDay／ReadMinute1／ReadMinute5 | 截断、非法日期／时分、非有限值、股票零股无法无损转换为整数手时返回错误，不返回部分记录；合法量价和占位规则不变 |
| SQLite Identity | 使用解析符号链接后的绝对根目录；目录别名身份一致。旧相对路径完成键可能触发一次增量复查，旧键保留 |

路径限制与旧库边界见[存储说明](storage.md#文件与旧库)。

## 遗留接口收敛

本轮直接调整开发阶段接口，不保留旧入口转发。

| 原用法 | 当前用法 |
| --- | --- |
| 自定义 ClientPool 只实现 Do | 实现 DoContext(context.Context, func(tdx.Client) error) error；ClientPool 只要求这一项能力 |
| pull 强制使用 ExConn 并发送品种帧 | 实现 Client.GetInstruments()，SDK 完成扩展全局分页及完整性校验；标准连接返回不支持错误 |
| sqlite.NewUpdate、UpdateStore、Record、标记键 prefix | 已删除；PullStore 内部实现 LoadUpdate/SaveUpdate，updated.db 及 update 表的 Key/Time 列不变 |
| httpclient.DialCodesHTTP/DialGbbqHTTP、CodesHTTP/GbbqHTTP、Update/Close | httpclient.New(address, client)，再显式 FetchCodes(ctx)/FetchGbbq(ctx)；应用安排刷新和快照发布 |
| ListenCodesHTTP/ListenCodesAndGbbqHTTP | CodesHandler(index, equity)，挂载到应用管理的 http.Server |
| export.KlinesToCSV(..., protocol.Klines) | 传入 []tdx.Kline；统一成交量直接按股导出，未知成交额留空 |
| export.TradesToCSV(..., protocol.Trades) | 传入 []tdx.Trade；方向 1/-1/0 映射到原 CSV 的 0/1/2，未知价格／成交额留空 |
| downloader.Pull(ctx, code) | Pull(ctx, startYear, endYear, code)，明确指定年份范围（含两端） |
| model.CodeModel.Price | 已删除，标准报价精度只由连接内部处理 |
| tdx.DialPoolFunc | 已移除；应用需要连接池工厂时自行声明 func() (tdx.ClientPool, error) |
| protocol.Message | 已移除；发送协议请求直接使用 *protocol.Frame |
| protocol.KlineReq 及其 Bytes | 使用 protocol.MKline.Frame(category, code, start, count)，code 包含交易所前缀 |
| protocol.GetPrice、CutInt、CutInt64 | 已移除宽松解码入口；协议消息使用对应 M*.Decode，内部统一校验变长整数的边界和结束位 |
| protocol.String、GetHourMinute、tdx.IntegerDay | 已移除孤立工具；通用字节和日期处理由应用按自身输入契约完成 |
| protocol.Kline.Amplitude | 使用 MaxDifference，仍返回 High-Low 的价差，单位不变 |

具体 Pool 的 Get、Put、Do、Go 便利方法仍可直接使用；同步业务统一使用 DoContext。取消只中断连接等待及后续操作，已经开始的 SDK 查询由有限请求超时控制。pull.UpdateContext 在任务边界检查取消，取消后不继续提交行情或完成标记。行情存储接口本身没有新增 context 参数。

CSV 保留原表头、UTF-8 BOM 和列顺序，分钟合成保留沪深 241 根、竞价及午休规则。日线源时间只有日期，导出时间列为空；CSV 表头仍适用于人民币、股数口径。指标、复权和本地文件编码继续按其整数手契约使用 KlineResp.Klines；复权／本地文件结果导出时按来源契约显式转回统一股数，不能将协议数量直接当作股数。HTTP 原始 JSON 格式和 SQLite 数据布局不变。

## 元数据获取与存储解耦

`Service.SyncCodes`、`SyncGbbq`、`SyncWorkdays` 改名为 `FetchCodes`、`FetchGbbq`、`FetchWorkdays`，只获取并校验快照，不写入存储。失败返回零值快照。`storage.CodesStore`、`GbbqStore`、`WorkdayStore` 及 Config 同名字段删除，不保留转发接口。

只获取元数据时配置 Pool 即可。需要持久化的调用方在 Fetch 成功后显式调用 SQLite 的 ReplaceCodes、ReplaceGbbq、SaveWorkdays，或使用自己的保存方式。标准证券自动发现改为 ListCodes(ctx)，指定 Codes 时可省略。SQLite 元数据文件、事务和范围替换规则不变；不删除或迁移已有数据。

## 日志改为标准库

tdx、扩展包和定时任务使用 log/slog，默认日志跟随应用的 slog.Default。新增 WithLogger(*slog.Logger) 设置独立连接日志；nil 回到默认日志，SDK 不修改全局配置。不新增日志接口或独立日志包。

通讯读写使用 Debug 级别和结构化 prefix/data 字段，保留 HEX／UTF8 编码及 WithDebug、WithLevel 配置。原 ios 把 LevelNone=0、LevelAll=999 直接转换为旧 logs.Level，导致关闭／全部的实际过滤相反；本次修正为 LevelNone 关闭、LevelAll 允许全部，再由 slog Handler 筛选。现有 LevelInfo、LevelRead、LevelWrite、LevelError 等常量值不变。默认 slog Handler 不输出 Debug，查看原始帧须启用 Debug Handler 和对应通讯级别。

示例查询结果使用 fmt 输出到 stdout，诊断通过 slog 输出，默认到 stderr；结果不再带旧日志前缀或依赖 Debug 开关。示例自身的参数校验和执行错误以 exit 1 结束，替代原 panic／exit 2，资源在返回错误时先关闭，不再打印预期错误的 panic 堆栈。标准 flag 的语法错误仍返回 exit 2，-h 仍返回 0。协议数据、重试等待和错误传播不变。

go.mod 将 github.com/injoyai/logs 标为 indirect：上游 ios 仍依赖它，本次不修改上游。连接配置见 [日志说明](unified-client.md#日志)。

## 区间价格涨跌的返回值

`extend.DoIncomes` 改为返回 `(extend.Incomes, error)`；调用方检查错误后再使用结果。输入必须按时间升序排列，nil 记录和非有限收盘价报错，不返回部分结果。偏移参数按 K 线条数计，0 为基准记录；负数及越界偏移仍忽略。固定东八区的起算日期规则保留。

`(*extend.Income).RiseRate()` 改为返回 `(rate float64, available bool)`，不可计算与真实零涨跌分别为 `(0, false)`、`(0, true)`。基准收盘价为零时 Rise 的价差仍可计算，RiseRate 不可用。String 不再输出 NaN% 或 Inf%；不可计算显示“不可计算”，百分数超出显示范围时明确标注。Source、Current 现在保留完整 K 线，而非仅 OHLC。protocol.Kline.RiseRate 也采用相同的 (ratio, available) 契约；无昨收时仍以开盘价为基准。旧百分数调用改为先检查 available，再按显示需要乘 100；Kline.String 保留正常百分数显示，不可用时显示“不可计算”。

完整用法见 [扩展功能说明](../extend/README.md#区间价格涨跌)。

## 完成标记统一存储

`update.Store`、`update.Tracker`、`update.NewWithStore` 和 `pull.Config.Tracker` 已删除，不提供兼容包装。lib/update 只保留定时工具，StartTimer 仍立即执行一次再启动调度；第三个参数改为 func() error，不再提供 Updater 接口。

行情调用方只配置 `pull.Config.Store`。`storage.BarStore` 直接定义 `LoadUpdate(key) (unix int64, has bool, err error)` 和 `SaveUpdate(key, unix) error`，已有行情后端无需改方法签名。pull 按市场计算更新边界，读取完成时间并比较 `has && unix >= cutoff.Unix()`，市场任务全部成功后保存当前 Unix 秒。读写错误继续返回，失败不会被视为完成。

原来为其他业务使用 Tracker 的调用方，应在应用自己的存储中管理标记和业务边界。sqlite 的标记组件已收为 PullStore 内部实现。SQLite 的 `updated.db`、`update` 表和 Key/Time 列、pull 的任务键均保持不变；原先单独通过 Tracker 保存到其他数据库的标记不会自动迁入行情 Store，首次调用可能重新执行增量检查。

## 扩展能力目录

根包和 protocol 负责 tdx SDK；基于 SDK 的业务模型、查询、同步、存储和 HTTP 服务归入 extend。

| 原导入路径 | 当前导入路径 |
| --- | --- |
| github.com/injoyai/tdx/v2/model | github.com/injoyai/tdx/v2/extend/model |
| github.com/injoyai/tdx/v2/metadata | github.com/injoyai/tdx/v2/extend/metadata |
| github.com/injoyai/tdx/v2/storage | github.com/injoyai/tdx/v2/extend/storage |
| github.com/injoyai/tdx/v2/storage/sqlite | github.com/injoyai/tdx/v2/extend/storage/sqlite |
| github.com/injoyai/tdx/v2/extend/metadatahttp | github.com/injoyai/tdx/v2/extend/metadata/httpclient |

pull 保持 github.com/injoyai/tdx/v2/extend/pull，内置市场仍从其 market 子包注册。HTTP 客户端包名从 metadatahttp 改为 httpclient，使用 New 和显式 FetchCodes/FetchGbbq。不保留旧路径转发。数据库文件、表结构、HTTP 路由和 JSON 格式不变；HTTP 元数据刷新改由应用调度。源码分组见 [目录说明](structure.md)。

## 统一同步、存储与纯内存查询

这是公开接口重构，不保留旧自动更新构造和 manage。

原 extend/codes、extend/gbbq、extend/workday 合并到extend/metadata，分别使用 metadata.Codes、metadata.Gbbq、metadata.Workday；三个对象仍独立构造，不提供旧导入路径或转发层。原 codes.Interface、gbbq.Interface 已移除，调用方按实际需求声明接口；HTTP 服务的最小代码查询接口为 httpserver.CodesSource。internal/gbbq 的计算已并入查询实现，lib/gbbq 继续独立负责文件下载、解密和解析。

| 原入口 | 当前方式 |
| --- | --- |
| codes.New(选项或记录...) | metadata.NewCodes(records)，纯内存代码索引 |
| codes.Base / NewBase / internal/codecache | 合并为 metadata.Codes / metadata.NewCodes(records)，空索引用 metadata.NewCodes(nil) |
| codes.CodeModel / CodeModels | 直接使用 model.CodeModel / CodeModels |
| pull.Code / KlineDay / KlineMinute / MinuteCoverage / MinuteEmptyScan，sqlite 同名别名 | 直接使用 model 对应类型；不再由同步器或后端重导出 |
| pull.MinuteSourceTrade | model.MinuteSourceTrade |
| pull.Store | storage.BarStore；Config.Store 字段保留 |
| gbbq.New(选项或事件...) | metadata.NewGbbq(events)，纯内存事件查询与复权 |
| workday.New(选项或快照...) | metadata.NewWorkday(model.WorkdaySnapshot)，纯内存日历 |
| 元数据 WithClient / WithDB / WithStore / WithSpec / Close | 已移除；pull.Fetch* 获取快照，应用自行决定持久化 |
| 各元数据包的 Store / SQLStore | 不再要求元数据存储接口；可直接使用 storage/sqlite 的具体方法 |
| manage.New / InitGbbq / AddWorkdayTask | 已删除；应用创建资源、安排任务、发布快照 |
| Config.CodeCache | ListCodes(ctx)；指定证券时可省略 |
| Config.Gbbq | 已删除，原始行情不回填派生字段 |
| QFQKlineDay / HFQKlineDay | Client 获取行情，再调用 metadata.Gbbq.QFQ / HFQ |
| GetTurnover 的标量返回 | Turnover 返回 (value, available)，区分未知与有效零 |
| pull.Trade.CodeCache | CodeName func(string) string；Workday 接收 RangeYear 接口 |
| update.NewSQLStore / update.Record | 行情通过 BarStore 读写完成标记；SQLite 标记实现为 PullStore 内部细节 |
| 独立数据库模块 | 主模块 storage/sqlite，其他后端由应用实现接口 |

pull.New 不联网、不启动定时任务。显式调用 FetchCodes、FetchGbbq、FetchWorkdays 获取元数据，或 UpdateContext 同步行情。元数据获取仅需 Pool，成功返回经过完整性校验的快照，失败返回零值；应用决定是否持久化及何时发布。使用 SQLite 时显式调用 ReplaceCodes、ReplaceGbbq、SaveWorkdays，数据与刷新时间仍在单次事务中提交。

共享业务模型位于 model。代码记录不再包含数据库 ID、InDate、EditDate，HTTP 代码 JSON 同步移除这些字段；日线不再含 FloatStock、TotalStock、Turnover。保留原始行情的源时间和量价口径，SQLite 旧额外列不会自动删除。新合并后端 sqlite.New(dir) 在 metadata.db 保存元数据；旧独立元数据库和不带覆盖信息的 workday 表不会自动搬迁／推断覆盖范围。

HTTP 元数据服务通过 CodesHandler(index, equity) 接收现成查询快照，由应用监听和关闭。metadata/httpclient.New 不联网，FetchCodes(ctx)/FetchGbbq(ctx) 显式获取完整查询快照。股本响应须包含有效代码和日期，事件代码须与所在分组一致；通过 metadata.NewGbbq 完整校验后返回，由应用成功后发布。空对象或 null 仍表示空快照。

完整用法见[数据架构](data-architecture.md)、[存储契约](storage.md)、[pull](../extend/pull/README.md)。

模块路径为 `github.com/injoyai/tdx/v2`，根包名仍为 `tdx`。下面的旧入口已移除，不再提供兼容别名。

## 市场默认时区与收盘后更新

`protocol.MarketInfo` 新增 `TimeZone string`，由 `Market.Info()` 返回按市场硬编码的 IANA 时区，供调用方调度使用：国内市场及当前国内期货为 `Asia/Shanghai`，港股及港股指数为 `Asia/Hong_Kong`，美股为 `America/New_York`。对应关系及查证来源见 [统一接口文档](unified-client.md)。

`Market.Info()` 保持 `(MarketInfo, bool)` 接口及原有非内置市场处理行为。全部内置枚举项均硬编码对应时区，新增枚举项时须先查证并填写时区，不新增缺少时区的运行时校验或错误返回。

`MarketInfo` 同时新增 `CloseTime string`（当地 `HH:MM`，不含延迟）：国内为 `15:30`，港股及港股指数为 `16:10`，美股为 `20:00`（包含盘后）。pull 统一延迟 5 分钟，收盘延迟边界之前跳过请求，之后按市场每天成功一次；不再使用整体完成标记或固定东八区 15:01 边界。`Update(true)` 仍保留市场级去重，允许提前拉取；提前标记不阻止当天收盘后的更新。`pull.Config.Workday` 已移除，调用方删除该字段，不再按中国日历过滤跨市场更新；`pull.Trade` 的历史交易日历使用不变。详情见 [pull 收盘后更新](../extend/pull/README.md#收盘后更新)。

这些字段不恢复行情结果中已删除的 `TimeZone`、`Unix` 或 `Timestamp()`，不改变源时间字符串和 SQLite 行情格式。无需迁移已有行情数据库。`MarketInfo` 结构体字面量应使用字段名初始化，避免新增字段影响按位置初始化的代码。使用示例见 [统一接口文档](unified-client.md)。

## 成交量统一为最小单位

`Quote`、`KlineResp`、`MinuteResp`、`TradeResp` 移除 `VolumeUnit`，`VolumeUnknown` 和 `protocol.VolumeUnit` 枚举也移除。统一成交量及五档买卖量使用股票股数、期货合约张数；HTTP、分页结果和停止回调使用相同口径。调用方删除原先依据单位再乘 100 的代码，避免重复转换。原始 `Standard/Extended/RangeExtended` 与 `StandardResponse()` 保留各自的协议单位；指标或本地文件编码使用 `KlineResp.Klines()`，由它将统一股数转为整数手。

`protocol.KlineInfo.VolumeUnit` 改为换算倍率 `VolumeScale`，报价/分时/分笔使用 `MarketInfo.TickVolumeScale`，倍率全部为正。国内尚未校准的品种或接口按用户指定默认 100；标准指数/板块 K 线和期货使用已确认的 1。可转债新增 `cn/bond`，其他未细分国内品种使用 `cn/other`，不再冒用股票类别；pull 的目录随类别区分，既有目录不会自动搬移。

可转债市场使用短名 `protocol.MarketBond`，对应 `market.Bond` 和目录 `cn/bond`。若已使用此前的 `MarketConvertibleBond`、`market.ConvertibleBond` 或 `cn/convertible_bond`，需同步更新引用及配置；旧目录不会自动搬移。

`KlineInfo.ToStorageVolume` 现在返回 `(int64, error)`，调用方须处理零/负倍率、负数量和溢出错误，不再对零值配置偷偷采用 ×100。期货、美股 pull 存储从旧默认 ×100 改为保留张数/股数；港股分钟 K 线采用 ×100。已有数据库不自动修改，旧记录不能未经核对混用。

国内可转债及其他默认品种按上述倍率开放，不宣称已经完成单位校准。港股指数含成交额口径，配置为正倍率 1，但统一成交数量接口仍报错，不能把它当作已确认股数。港股 K 线仅有百股精度，不能恢复零股。完整边界见 [数据契约](unified-client.md#数据契约)。以下历史迁移条目的量单位说明以本节为准。

美股日线从周期 9 改为 4：相同 AAPL 日期，旧周期单根返回 1,172,584，多根末条返回 395,396；周期 4 单根、多根均为 39,539,690 股，与独立报价一致。此类旧数据不能整体乘倍数修正，应保留旧库后重新拉取或逐条核对。

## Client 接口与单协议连接

`tdx.Client` 是统一业务接口，`NewClient()` 返回按市场路由的 `*MarketClient`。标准 `Dial/DialDefault/DialWith/DialHosts*` 直接返回 `*StConn`；`DialEx/DialExDefault/DialExHosts` 直接返回 `*ExConn`。两种连接均实现 `Client`，单协议连接不会建立另一类连接。

公开 `Conn` 已移除。内部 `conn` 仅负责 TCP 收发、请求关联、超时及关闭/重连，不实现 `Client`，也不包含业务查询或协议专用解码。标准业务和解码归 `StConn`，拓展归 `ExConn`；共享分页、日期检查等通过绑定业务对象的 `queries` 执行，不再经由传输层绕回 `MarketClient`。

| 旧调用或类型 | 当前调用或类型 |
|---|---|
| 业务参数 `*tdx.Client` 或 `*tdx.Conn` | `tdx.Client` |
| 标准连接变量 `*tdx.Conn` | `*tdx.StConn`，由标准 `Dial*` 直接取得 |
| 拓展连接变量 `*tdx.Conn` | `*tdx.ExConn`，由 `DialEx*` 直接取得 |
| `NewStConn(conn)` / `NewExConn(conn)` | 已移除，直接使用对应 `Dial*` 返回值 |
| `NewPool` 工厂 `func() (*tdx.Conn, error)` | `func() (tdx.Client, error)` |
| `IPool.Get/Put` | 获取、归还 `tdx.Client` |
| `IPool.Do/Go` 回调 `func(*tdx.Conn)` | `func(tdx.Client)`，池中对象通过统一接口查询和关闭 |
| `CloseAll()` | `Close()`，结束整个生命周期并停止重连 |
| 原始 `SendFrame`、连接状态、日志等 | 从 `*StConn` / `*ExConn` 使用；业务 `Client` 不包含这些能力 |
| Codes/Gbbq/Workday 的客户端注入与对应工厂 | 继续接受 `Client` 及 `func() (Client, error)` |

```go
var c tdx.Client = tdx.NewClient(tdx.WithRedial(false))
defer c.Close()
quotes, err := c.GetQuote("sz000001", "hk00700")
_ = quotes; _ = err

pool, err := tdx.NewPool(func() (tdx.Client, error) {
    return tdx.DialDefault(tdx.WithRedial(false))
}, 1)
if err != nil { panic(err) }
defer pool.Close()
err = pool.Do(func(c tdx.Client) error {
    _, err := c.GetKlineDay("sz000001", 0, 100)
    return err
})
```

Go 函数返回类型不支持协变，不能把 `DialDefault` 直接当作 `func() (Client, error)` 传入；使用上述闭包适配。业务接口包装空指针仍按 nil 处理，连接池工厂返回 nil 或空指针客户端时报错并清理已初始化对象。

`Client.Close()` 保持关闭已有及晚到连接、停止重连的语义，可重复调用；传输层不再提供 `CloseAll` 兼容入口。财务、F10、集合竞价、除权、板块、报表和标准分类代码列表由 `StConn` 实现；`ExConn` 对这些接口明确报错。门面自动取得标准连接，标准接口的返回类型和单位保持不变。

## 跨市场统一查询

单协议业务连接实现使用简名 `StConn` / `ExConn`，此前 `StandardClient` / `ExtendedClient` 不保留别名；直接由对应 `Dial*` 创建。

统一返回对象去掉 `Market` 前缀，名称对应如下；`Client`、`MarketClient` 保持现有名称。

| 旧返回类型 | 新返回类型 |
|---|---|
| `tdx.MarketQuote` | `tdx.Quote` |
| `tdx.MarketKlineResp` / `tdx.MarketKline` | `tdx.KlineResp` / `tdx.Kline` |
| `tdx.MarketMinuteResp` / `tdx.MarketMinute` | `tdx.MinuteResp` / `tdx.Minute` |
| `tdx.MarketTradeResp` / `tdx.MarketTrade` | `tdx.TradeResp` / `tdx.Trade` |
| `tdx.MarketCodeResp` / `tdx.MarketCode` | `tdx.CodeResp` / `tdx.Code` |
| `tdx.MarketExchange` | `tdx.Exchange` |
| `protocol.ExQuoteListItem` | `protocol.QuoteListItem` |

上述类型不保留旧名别名，外部类型声明和回调参数需同步改名；字段、单位和 JSON 形状保持不变。
`GetQuoteList` 仍只支持扩展市场；记录的 `Extended/RangeExtended` 原始协议类型保留 `Ex*` 名称，以区分不同的协议结构。

`tdx.NewClient()` 返回业务接口 `tdx.Client`，默认跨市场实现为 `*tdx.MarketClient`，按代码自动选择标准/扩展连接，并通过统一的 `GetQuote/GetKline*/GetMinute/GetTrade/GetCode/GetCodeAll/GetCount/GetMarkets` 返回统一结果。构造不联网，错误由首次查询返回。`Dial*` 和连接池仍可使用；单协议连接的基础行情与代码查询已变更返回类型，须按下表迁移，HTTP 直接返回统一模型；新结果不是旧 `protocol.Klines`，不能直接用于原指标/复权/存储 API。字段、单位及支持边界见 [统一 Client](unified-client.md)。

## 标准与扩展函数收敛（需迁移调用方）

不再公开同一业务的两套函数；以下移除入口没有兼容别名：

| 已移除入口 | 统一调用 |
|---|---|
| `ExQuote(market, code)` | `GetQuote("hk00700")`，返回列表，单品种取 `[0]` |
| `ExBars(category, market, code, start, count)` | `GetKline(category, "hk00700", start, count)`，记录位于 `List` |
| `ExMinute(market, code)` | `GetMinute("hk00700")` |
| `ExHistMinute(market, code, date)` | `GetHistoryMinute("20261009", "hk00700")` |
| `ExTrade(market, code, start, count)` | `GetTrade("hk00700", start, count)` |
| `ExHistTrade(market, code, date, start, count)` | `GetHistoryTrade("20261009", "hk00700", start, count)` |
| `ExQuoteList(...)` | `GetQuoteList(...)`，市场分页语义和参数不变 |
| `ExBarsRange(market, code, date, date2)` | `GetKlineRange("hk00700", "20261009", "20261009")`，一分钟区间，原扩展字段在 `RangeExtended` |
| `GetIndex/GetIndexUntil/GetIndexAll` 及指数周期便捷方法 | 对应 `GetKline/GetKlineUntil/GetKlineAll/GetKlineDay…`，自动选择指数解码；`GetIndexCodeAll` 是代码列表入口，仍保留 |
| `ExMarkets()` | `GetMarkets()`，返回 `[]Exchange`，原分类字段在 `Extended.Category` |
| `ExCount()` | `GetCount(exchange)`，返回指定市场的 `int` 数量，语义不再是全局总数 |
| `ExInstruments(start, count)` | `GetCode(exchange, start, count)`，按市场内偏移分页，返回 `*CodeResp` |

原标准 `Client.GetQuote/GetKline/GetMinute/GetHistoryMinute/GetTrade/GetHistoryTrade` 的返回类型也分别改为 `[]Quote`、`*KlineResp`、`*MinuteResp`、`*TradeResp`，与 `MarketClient` 相同。日期统一使用有效 `YYYYMMDD` 字符串；代码包含市场前缀，可用 `protocol.Instrument{Exchange: ex, Code: code}.String()` 构造。

统一价格为原币种/报价单位浮点值；未知量单位、扩展分笔价格和金额语义保持显式未确认。需要协议特有字段或原始成交量口径时，从记录的 `Standard/Extended` 获取解码结构；标准价格和成交额现在也已改为浮点元值，不再提供整数厘。例如旧 `quote.Kline.Close.Float64()` 改为统一 `quote.Price`，期货持仓从 `quote.Extended.ChiCang` 读取。标准成交转指标时，将每条 `Trade.Standard` 收集为 `protocol.Trades`；不要把未知扩展量按百股直接换算。

`DialDefault/DialExDefault` 仍只连接对应服务，通用 `Get*` 查询其他协议品种会报错；跨市场使用 `NewClient()`。`Client` 与 `MarketClient` 的全部 K线周期、`All/Until` 便捷方法均返回 `KlineResp`；`Until` 的回调参数改为 `Kline`。独立指数 K线查询和 `ExBarsRange` 均已移除，无兼容别名。统一代码路由仍只覆盖已接入的市场；此前直接用任意市场编号调用六组 `Ex*` 的代码，在未接入市场上需使用已有 `SendFrame` 与 `protocol.MEx.Frame*`，不能仅把市场编号替换成未知前缀。

完整成交查询 `GetTradeAll/GetHistoryTradeDay/GetHistoryTradeBefore/GetHistoryTradeFull` 统一返回 `*TradeResp`，支持两类协议。单日历史响应和逐条 `Trade.Date` 保留请求日期；跨日响应的整体 `Date` 为空，每条记录有自己的日期。分页顺序、协议偏移 65535 护栏及标准跨页昨收保持；中途失败不返回部分历史。`Count` 使用 int，原标准 uint16 结果需要显式转换。

历史分笔查询新增同代码、同交易日期的日线检查：没有对应日线时返回空分笔，日线查询失败时返回错误；整日分页仅检查一次，跨日查询逐日检查。调用方不能再依赖无对应日线时分笔接口的非空返回；当天实时分笔不受影响。此保护可能增加日线分页请求，详见 [统一 Client](unified-client.md)。

`GetHistoryTradeBefore/Full` 的交易日历参数改为 `TradingCalendar`（提供 `Range` 方法）。日历实现使用 `*metadata.Workday`（`metadata`），它是中国市场历史日历；其他市场必须由调用方提供对应日历。查询从最早可获取月线所对应月份开始，不保证服务器保留上市以来的全部分笔；`Before` 沿用日历的日期结束边界，获取整日数据，不按结束时刻截断当天分笔。扩展夜盘日期和价格尺度不作猜测换算。

复权和协议数据处理可使用 `resp.StandardResponse()` 获取原 `protocol.KlineResp/MinuteResp/TradeResp`。它只提取保留的标准解码结构，价格和成交额已为浮点元值，不将扩展字段冒充标准数据；扩展结果和超过 uint16 数量上限的结果会报错。它保留标准指数的股数，不能直接作为整数手输入交给 VWAP 或本地文件写入器；这些场景使用新增的 `KlineResp.Klines()`：

```go
r, err := c.GetKlineDayAll("sz000001")
if err != nil { return err }
ks, err := r.Klines()
if err != nil { return err }
vwap := ks.VWAP()
_ = vwap
```

`Klines()` 返回记录副本，将统一 `Kline.Volume` 的股数转为整数手，原始记录及 HTTP 响应不变。缺少标准来源或不能无损换算的股数会报错；此切片适配不受旧响应的 uint16 条数限制，不能把未知转债或扩展单位猜成手数。

`GetKlineRange(code,startDate,endDate)` 固定查询一分钟线，日期使用 YYYYMMDD，包含两端日期。扩展保留原区间协议的固定请求周期 7（与普通扩展一分钟查询的周期映射不同），持仓和结算价等字段在 `RangeExtended`，金额未确认时保持未知，成交量转换为股/张，无法确认单位时报错；标准通过分页筛选日期区间，超出可读取分页范围时报错。未接入的协议市场仍须使用 `SendFrame`。`GetKlineMinute241Until` 在两种 Client 上使用同一签名及统一返回类型，仅适用于标准股票/ETF；指数或扩展品种明确报错，普通分钟查询使用 `GetKlineMinuteUntil`。

代码查询在 `Client` 与 `MarketClient` 上使用相同签名：`GetCount(exchange) (int, error)`、`GetCode(exchange, start uint32, count uint16) (*CodeResp, error)`、`GetCodeAll(exchange) (*CodeResp, error)`。原标准 `GetCode(exchange, start)` 需增加 `count`（旧分页大小为 1000）；`code.Code` 改为 `code.Instrument.Code`，小数位等特有元数据从 `code.Standard` 读取。分页 `count` 为 1..1000；扩展协议需扫描全局列表再按市场过滤，偏移按过滤后的列表计算。北交所数量和分页使用与完整列表相同的报表来源。

`GetMarkets()` 在标准连接上返回内置沪深北，在扩展连接上返回服务器市场表；`NewClient().GetMarkets()` 合并两者，并在扩展发现失败时返回错误。服务器列出的市场不一定已接入统一行情查询。需要旧扩展全局总数或全局分页时，使用既有 `SendFrame(protocol.MEx.FrameCount())` / `SendFrame(protocol.MEx.FrameInstrument(start, count))`，不能用任意单市场查询替代。历史 HTTP `/ex/count`、`/ex/instruments` 继续通过协议层保持原全局语义。pull 的全局品种快照入口改名 `Service.GetInstruments()`，本轮缓存范围保持不变。

HTTP 查询直接返回统一函数的结果，包含证券标识和单位；不再转换回旧 `protocol` 响应。扩展日线/一分钟参数经市场周期映射；特殊市场、周期能否取得数据仍以实际服务支持为准。内置 pull 接口已同步迁移并保留原存储口径。

## 接口替换

| 已移除的入口 | v2 入口 |
|---|---|
| `protocol.HistoryTradeResp` | `protocol.TradeResp` |
| `Client.GetMinuteTrade` | `Client.GetTrade` |
| `Client.GetMinuteTradeAll` | `Client.GetTradeAll` |
| `Client.GetHistoryMinuteTrade` | `Client.GetHistoryTrade` |
| `Client.GetHistoryMinuteTradeDay` | `Client.GetHistoryTradeDay` |
| `Client.GetKlineHour` | `Client.GetKline60Minute` |
| `Client.GetKlineHourAll` | `Client.GetKline60MinuteAll` |
| `Client.GetKlineHourUntil` | `Client.GetKline60MinuteUntil` |
| `extend.NewPullKline` / `PullKlineConfig` / `PullKline` | `pull.New` / `pull.Config` / `pull.Service` |
| `extend.GetBjCodes` / `lib/bse.GetCodes` | `Client.GetCodeAll(protocol.ExchangeBJ)` |

另外移除或统一以下入口：

| 已移除的入口 | v2 用法 |
|---|---|
| `protocol.TypeMinuteTrade` / `TypeHistoryMinuteTrade` | `TypeTrade` / `TypeHistoryTrade`，消息数值不变 |
| `protocol.Factor.QFQ` / `HFQ` | `QFQPrice(raw)` / `HFQPrice(raw)`，或使用 `Mul * raw + Add` |
| `PreKline.PreLast` / `Factor.PreLast` / `QFQFactor()` / `HFQFactor()` | `XRXDs.Pre(ks).Factors()` 获取仿射系数；单事件参考价用 `XRXD.Pre(raw)` |
| `extend.Kline` / `extend.Klines` | 指标使用 `protocol.Kline/Klines`；落库模型使用 `model.KlineDay/KlineMinute`，注意单位转换 |
| `extend.Klines.REF/HHV/LLV` | 调用方检查长度后按切片取值或遍历计算高低值 |
| `extend.Info` | 调用方按需要组合行情、股本和换手率数据 |
| `extend.DefaultDayKlineExportTitle` / `DefaultMinuteKlineExportTitle` | CSV 导出使用 `KlinesToCSV`；自定义列由调用方定义 |

`protocol.Klines` 成为唯一指标实现。VWAP 的输入成交量须为手，成交额为元；`pull` 查询结果使用股和元，只需转换成交量。标准行情指数日线的解码成交量为股，调用 VWAP 前也需转为手。VWAP 修正手股换算，ATR 使用最近 n 个真实波幅，RSI 在纯上涨窗口返回 100、平盘窗口返回 0。RSI/ATR 为已有的简单滚动窗口口径，不改为其他平滑算法。

第三方爬虫使用 `spider.Factor/Factors`，它们是数据源返回的比例模型，不等同于通达信仿射复权模型。

## 请求等待与超时

`SendFrame` 先注册回调再发送，避免快速响应早于等待注册而被丢弃；回调注册与响应处理互斥，超时或写入失败会清理回调与请求缓存，迟到响应会被忽略。标准行情默认请求超时仍为 2 秒，扩展行情仍为 10 秒，覆盖排队、发送和等待响应全过程。发送中途超时或短写会关闭当次连接，避免残缺帧污染后续请求；仅排队或等待响应超时则保留连接。

标准与扩展行情的握手、心跳、业务请求和重连初始化共用写入门控，保护底层连接状态与写入统计；新连接先完整发送握手，再放行业务写入。具体连接的 `Write` 及其字符串等写入方式保持稳定入口。心跳绑定当次连接。`Close()` 取消连接生命周期、关闭传输并等待读取流程退出后释放底层资源，门面还会取消进行中的首次建连和重试。截断或非法的成交、分时、股本和 F10 响应直接返回解码错误；压缩帧最多读取声明解压长度加一字节，超长响应直接报错。

扩展行情响应的头部或声明的记录不足时返回 `io.ErrUnexpectedEOF`，不再返回部分记录或把损坏响应当成空结果；合法的零记录响应仍正常返回。HTTP 服务的 `Close()` 同时释放自建的标准与扩展行情连接池，初始化失败也会清理已建连接池。

TCP 传输使用幂等关闭封装，避免重复关闭导致底层库跳过断开回调与缓存清理；`Origin()` 返回值仍支持 `net.Conn` 接口，调用方应依赖接口而非断言具体的 `*net.TCPConn` 类型。

真实只读验证按 `PROJECT_RULES.md` 第 8.5 节执行。标准与扩展行情测试共用每秒一次、最多 1000 次的写入限流，可用 `TDX_STD_LIVE=1 TDX_EXHQ_LIVE=1 go test -race . -run '^(TestStandardLive|TestExLive)$' -count=1 -v -timeout=150s` 启用；分别用 `TDX_STD_HOST` / `TDX_EX_HOST` 指定服务器 IP 或 `IP:端口`。标准测试只做一次受控重连，普通离线测试跳过真实访问。HTTP 上游验证用 `TDX_HTTP_LIVE=1 go test -race ./extend/httpserver -run '^TestServerLive$' -count=1 -v -timeout=45s`，须在行情测试完成后单独执行，不并行启动。

数据准确性还须按项目规则第 8.6 节对照独立来源，其他来源单线程、请求起始间隔至少 5 秒。`TDX_CROSS_LIVE=1 go test -race . -run '^TestCrossValidationSamplesLive$' -count=1 -v -timeout=45s` 保存小样本到 `output/live-validation/cross/tdx-samples.json`，供另行串行获取独立数据并逐字段比较；该测试成功只表示样本获取成功。港股主板日线现用 category=4，腾讯控股样本的 `ExKline.Trade` 为百股，×100 与独立来源在整数百股精度内一致；不能将该单位推广到其他扩展接口。小时线的来源聚合差异和港股分时的时间标签仍须确认。

标准行情代码、K 线与五档报价也检查记录边界和变长整数；截断或溢出响应直接返回解码错误。`protocol.MQuote.Decode` 统一返回 `(QuotesResp, error)`；不保留丢弃错误的旧签名。

`Hosts`、`ExHosts` 及地域子列表类型改为 `Addrs`（底层为 `[]string`），统一存储 `IP:端口`，从列表取得地址后不要再拼接端口。`NewExRangeDial` 已移除，使用 `NewRangeDial(tdx.ExHosts)`；自定义扩展列表必须显式带端口，若需要保留裸 IP 默认补 `7727` 和空列表使用 `ExHosts` 的行为，直接使用 `DialExHosts(hosts)`。`Dial` / `DialHosts*` 仍兼容裸 IP，默认补 `7709`；`DialEx` / `DialExHosts` 默认补 `7727`，显式端口均保留。

默认地址列表也为空时，拨号返回错误；扩展拨号不会回退到标准列表。将测速结果赋给默认列表后无可用地址时也适用此规则。

`NewTCPDial`、`NewHostDial`、`NewRandomDial`、`NewRangeDial` 只使用传入的完整地址，不补端口、不选择协议默认列表；列表为空时返回拨号错误。原 `New*Dial(nil)` 选择标准列表的用法须改为显式传入 `tdx.Hosts`。需要兼容裸 IPv4/IPv6 和空列表选择协议默认地址时，使用 `Dial` / `DialHosts*` 或 `DialEx` / `DialExHosts`。完整 IPv6 地址使用 `[IPv6]:端口`，裸 IPv6 可以带 zone，如 `fe80::1%eth0`。

四个 `New*Dial` 都使用调用方的 context 建连，每个地址最多等待 5 秒；遍历地址间的退避等待也可取消。轮询和随机拨号函数可以在多个客户端之间并发复用。

测速实现移入 `Addrs.Sort(timeout ...time.Duration) Addrs` 方法，`SortHosts` / `SortExHosts` 已移除。原 `SortHosts(hosts, timeout...)` 改为 `tdx.Addrs(hosts).Sort(timeout...)`；标准和扩展内置列表直接使用 `tdx.Hosts.Sort(timeout...)` / `tdx.ExHosts.Sort(timeout...)`。地址须包含端口；方法返回新的 `Addrs`，不修改输入或全局默认列表。需要保留原更新默认列表的行为时显式赋值：

```go
tdx.Hosts = tdx.Hosts.Sort()
tdx.ExHosts = tdx.ExHosts.Sort()
```

`Addrs` 可直接传给现有接收 `[]string` 的拨号函数，无需转换或泛型；普通 `[]string` 也可以赋给默认列表。若外部代码使用反射或断言检查默认列表的具体类型，需要同步改为 `Addrs`。

超时是整轮测速的时间上限（默认一秒），按 TCP 连接耗时排序；全不可用或输入为空时返回空列表，不使用默认地址，也不无限等待第一个可用地址。测速仅检查 TCP 建连，不代表行情握手或查询成功。

### 更新工具与元数据查询

manage 已删除。codes、gbbq、workday 只处理调用方提供的数据，元数据获取与持久化统一由 pull 和 storage 完成。lib/update 只提供定时工具 StartTimer；它仍会立即执行一次，再启动定时任务，其停止函数必须由调用方持有。

HTTP 元数据客户端只提供 New 和 FetchCodes/FetchGbbq，不持有后台任务或查询缓存；取消由请求 context 控制，应用安排调度和发布。外部 HTTP 数据请求（代码/股本缓存、同花顺、新浪和股本文件下载）默认总超时 30 秒；非成功状态、畸形同花顺响应，以及新浪复权因子中的空记录、非法数字、NaN 或无限值均返回错误。

`lib/zip.Encode` 拒绝源文件与目标文件相同的路径、硬链接或符号链接，检查失败不会截断源文件；压缩写入及关闭错误会返回调用方。

内置 pull 代码复用协议层校验：扩展代码最多 9 字节，美股须含字母，期货须同时含字母和数字，完整市场键须匹配品种。过去被接受的非法配置现在提前返回错误；合法代码沿用原文件名，自定义市场保留自己的代码规则。`Instrument.Market()` 将沪市 880/881 板块归为 `cn/block`，不再误归 `cn/index`。

边界行为修正：`Merge241(n<=0)` 原样返回；`DoIncomes` 忽略负偏移，起始日期晚于全部数据时返回空。pull 的 `DayToPeriod` / `MinuteToPeriod` / `DayToCalendar` 直接合并原始浮点量价，保留超过三位小数及负价格；不再往返整数厘。裸美股代码统一转大写参与去重和文件定位。旧的小写美股库不会自动重命名，使用前应在停机备份后合并到大写文件名，包含相应 WAL/SHM；不要直接覆盖已有大写库。


`local.WriteMinute1` 仅在当日包含 15:00 收盘记录时补尾盘占位，盘中或部分日的数据原样编码；已有占位不会重复补入。

代码快照在明确交易所范围内原子替换，SQLite 内部保留未变化证券身份，但数据库 ID 不再出现在业务模型中。退市项只从当前代码快照移除，不删除其行情或股本历史。

设置请求超时统一使用 `Client.SetTimeout(duration)`；原先直接调用单连接 `Client.Wait.SetTimeout` 的代码需迁移到该方法。具体连接的 `Wait` 保留原类型，用于内部响应关联，直接修改它不会调整 `SendFrame` 的计时器。

## 成交行为

成交接口的参数、返回类型、分页顺序和请求协议保持一致；HTTP 服务的成交查询使用统一结果模型。

标准行情的全部代码、成交、K 线分页使用整数累加偏移；偏移或累计数量超过 `uint16` 上限（65535）会报错，不会回绕或返回错误数量。此时调用方不应将结果视为完整历史。`GetBlockFileRaw` 按元信息校验文件长度，缺块返回 `io.ErrUnexpectedEOF`，过长分块也会报错。

成交查询使用 `GetTrade` / `GetTradeAll` 和 `GetHistoryTrade` / `GetHistoryTradeDay`，支持边界见 [统一 Client](unified-client.md)。

北交所代码通过通达信 `zhb.zip` 中的 `tdxbjmore.cfg` 获取，返回 `CodeResp.List`，包含代码与名称。需要 `bj` 前缀字符串时，将 `"bj"` 拼接到每条记录的 `Code`；该配置可能包含已分配但尚未上市的代码，不等同于官网上市证券列表。示例见 [GetCodeAll](../examples/GetCodeAll/main.go)，使用 `-exchange 2 -all`。

## K 线拉取服务

统一使用 `github.com/injoyai/tdx/v2/extend/pull`。使用内置市场时需要注册适配器：

```go
import (
    "github.com/injoyai/tdx/v2/extend/pull"
    _ "github.com/injoyai/tdx/v2/extend/pull/market"
)
```

| 旧调用 | 新调用 |
|---|---|
| `Types: []string{extend.Day, extend.Minute}` | `Day: true, Minute: true` |
| `Types: []string{extend.Day}` | `Day: true, Minute: false` |
| `Update(m)` | 在 Config 中传入 Pool、Store 和自动发现所需的 ListCodes，然后调用 `s.UpdateContext(ctx)` |
| `Run(m, spec)` | 调用方安排定时任务，在任务内调用 `s.Update()` |
| `DayKlines(code, start, end)` | `s.QueryDay(c, start, end)`，其中 `c` 由 `pull.ParseCode(code)` 获取 |
| `DayKlinesAll(code)` | `s.QueryDay(c, "", "")` |
| `DayKline(code, n)` | 查询升序日线后检查长度，按切片下标选取；最新一根为 `ks[len(ks)-1]` |
| `MinKlines(code, start, end)` | `s.QueryMinute(c, start, end)` |

`Config.Store` 必须显式设置，原 `Config.Dir` 已移除；SQLite 目录由 `sqlite.NewPull(dir, maxCachedEngines)` 配置。`StartAt` 改用 `YYYYMMDD` 字符串，未设置时默认最近两年，非法格式或不存在的日期在创建服务时直接报错；`pull.New` 不创建数据库。`Day` 与 `Minute` 都为 `false` 时默认同时拉取两种数据。服务使用完成后调用 `s.Close()`，注入的存储、连接池及元数据组件仍由调用方关闭。

日线查询返回 `[]*model.KlineDay`，分钟查询返回 `[]*model.KlineMinute`。拉取层直接使用统一 Client 的源时间、浮点价格和最小单位成交量（股票为股、期货为张），不再提取原始协议记录重复换算，不再受 `StandardResponse()` 的 uint16 条数限制。`protocol` 的价格和成交额也为浮点元值，不再乘 1000。

Amount 改为 *float64：Client 未提供金额时保存 nil／SQL NULL，非 nil 的 0 为真实零。移除 AmountUnknown，新库不另加状态列。聚合仅在全部组成记录金额已知时求和，否则为 nil。旧库既有 0 保持非 nil 的 0，不自动推断为未知或重写历史。扩展原始持仓字节不会作为金额落库。源时间格式库的旧状态列不再映射或使用，不自动删列、改历史金额；过去自定义写入非零未知金额的记录需自行核对并重拉。示例见 [pull](../examples/pull/main.go)，完整服务说明见 [pull 文档](../extend/pull/README.md)。

内置市场的参数和倍率集中于 `Market.Info()`。K 线倍率：股票/ETF、可转债、其他国内品种、港股主板为 100，标准指数/板块、美股及期货为 1。报价/分时/分笔使用 `TickVolumeScale`。港股指数保留正倍率 1，但缺少股数口径，统一成交数量接口拒绝。所有转换拒绝零/负倍率，不再使用隐藏兜底。

港股主板日线由 category=9 改为 4，修正成交量请求口径；标准行情指数及板块分钟线直接使用解码后的股数，取消拉取层重复 ×100。已有数据库不会自动修正历史记录；旧港股日线首条的异常量不能靠整体乘倍数恢复，重新获取时请保留旧库，并考虑服务器历史保留范围。

## 数据库支持

storage 仅定义行情 BarStore；SQLite 实现在主模块 storage/sqlite。sqlite.New(dir) 提供行情与元数据具体读写方法；Open(filename)、NewMetadata(db)、NewPull(dir,limit) 可独立使用；完成标记仅作为行情后端内部实现。其他行情后端由应用实现 BarStore；元数据持久化由应用决定。纯内存业务包不接收数据库或 Store。

## 存量数据库

旧服务使用 `day-kline/`、`min-kline/` 与 `update.db`；新服务按市场存储，例如 `cn/stock/day/sz000001.db`、`cn/stock/min/sz000001/sz000001-2026.db`，去重库为 `updated.db`。

旧表与新表的字段结构、时间字段和量价单位不同，不能仅重命名目录直接复用。新服务不会自动读取或转换旧服务数据库；需要单独转换存量数据，或在新目录重新拉取。重新拉取只能恢复服务器仍可提供的历史，旧库应保留以便转换。

新服务自身已有 SQLite 表的补字段、覆盖标记兼容继续保留，与上述旧服务目录迁移是两回事。

## 缓存目录由调用方配置

Go 最低版本为 1.25.0。目录由 SQLite 后端或应用配置。旧元数据文件不会自动搬迁；旧日历缺少来源及覆盖范围，不自动转换为新日历。详见[存储说明](storage.md)。

## 源时间、交易日与真实时刻

统一 API 的日／周／月／季／年 K 线 `Time` 只返回源日期 `YYYY-MM-DD`；日内 K 线保留 `YYYY-MM-DD HH:MM`。不再为日期标签添加 `15:00`。统一 `Kline` 已移除 `TradingDate`，时间只保留 `Time`。`KlineResp` / `MinuteResp` / `TradeResp` 已移除 `TimeZone`，不再附加来源时区或生成时间戳；源时间字符串保持原值。

标准原始协议和本地文件的 `time.Time` 保留固定东八区约定，协议日线 15:00、本地日线 00:00 为内部日期标签，不代表所有市场的成交或收盘时刻。交易日判断和内置中国市场定时任务仍使用东八区，不依赖主机 `TZ`。

pull 存储不兼容旧数据，需为 SQLite 后端指定新的数据目录重新拉取；新目录标记为 `.time-format`，内容为 `source-time-v2`。旧 `.storage-timezone` 或未标记旧数据库会被拒绝，不自动迁移、覆盖或清理旧目录。

| 旧接口／字段 | 新接口／字段 |
| --- | --- |
| 日线 `Unix int64` 主键 | `Time string` 源日期主键 |
| 分钟线 `Unix int64` 主键 | `Time string` 源日期时分主键；不再保留 `Unix` / `TimeZone` 附加字段 |
| `LastDayUnix` / `LastMinuteUnix` / `LastUnix` | `LastDayTime` / `LastMinuteTime` / `LastTime`，空库返回空字符串 |
| `QueryDay` / `QueryMinute` 的 `time.Time` 边界 | 源日期／日期时分字符串闭区间，空字符串不限端点 |
| Save／coverage／empty-scan 的 Unix 边界 | 同一源时间格式的字符串边界 |
| `DayToCalendar(ks, period, loc)` | `DayToCalendar(ks, period)`，按源日期 |
| `MinuteToSessions(ks, n, loc, sessions)` | `MinuteToSessions(ks, n, sessions)`，按源钟面时段，不推断夜盘交易日 |

`KlineDay` / `KlineMinute` 已移除 `TradingDate`。标准、扩展和分笔合成分钟线统一只保存源日期时分，年度文件、增量和聚合均按源时间处理。`Kline.Timestamp(loc)`、`KlineMinute.Timestamp(loc)` 和 `protocol.ParseSourceTime` 已移除；存储仅校验合法的源日期时分、年份和范围，不根据来源时区或夏令时解释钟面值。

SQLite 目录标记现为 `source-time-v2`；旧 `source-time-v1` 库不能继续打开，须使用新目录，不自动迁移。源时间字段本身的格式不变。旧分钟表里的额外 `Unix` / `TimeZone` / `TradingDate` / `AmountUnknown` 列不再映射或使用，新表不再创建这些列；不会自动删列或清理旧数据。旧代码须删除这些字段的赋值和转换方法调用；行情 JSON 不再包含 `TimeZone` / `Unix` / `TradingDate`，pull 结果也不再包含 `AmountUnknown`。这不改变内部标准协议 `time.Time`、交易日历及缓存过期检查的既有时间处理。


### 重复逻辑收敛后的兼容边界

- `protocol.DecodeCode` 对已接入市场复用 `ParseInstrument`，港股指数前缀、点分隔前缀、交易所后缀和美股点代码使用同一规则。未接入统一 Client 的原始协议市场仍保留旧解析能力，不代表统一 Client 已支持该市场。
- pull 的内置代码解析也复用这套规则，`000001.SZ` 归深市、`BRK.B` 归美股；完整市场键及已注册自定义市场继续保留。`ParseCode` 不再将任意 `unknown.code` 静默当成市场；需要自定义市场时先注册 Unit。`SplitKey` 已移除，统一使用有错误返回的 `ParseCode`。
- 标准分笔解码的钟面值现带固定东八区标签，修正原有 UTC 标签；日期与时分保持不变，Unix 值会相差八小时。`Trades.Klines` 保留输入交易日的时区，不再借用主机时区恢复日期。本地日线保持 00:00，协议日线/同花顺日线保持 15:00；同花顺金额按交易日期关联。
- `pull.Trade` 的 5/15/30/60 分钟 CSV 使用 `Merge241`：09:30 竞价单独保留，后续按连续交易分钟聚合，保留 11:30/13:01 午休边界。记录数和总量额不变，首根及各周期终点修正。已有 CSV 如需统一口径，应另行重新导出。
- HTTP 基础行情直接返回统一结果；`StandardResponse()` 仅用于确实需要原始协议记录的复权、指标和存储适配。交易所参数支持混合大小写。
- `GetReportFile` 与 `GetBlockFileRaw` 共享单块校验，超长响应现在返回错误；前者仍按短块/空块结束，后者仍要求达到元数据声明的文件大小。
- `protocol.DecodeKline` 及 `lib/gbbq` 的 `GetStocks/Equity/Equities/Turnover` 旧派生模型已移除。离线文件保留原始 `GBBQ/GBBQs` 解密结果，文件单位未独立核实，不自动转换为网络股本模型。

## 股本查询与日历

股本查询使用 metadata.NewGbbq(events)，数据获取使用 Client.GetGbbq 或 pull.FetchGbbq。复权计算、股本按日生效和量额口径保持原有业务算法；缺失股本的换手率通过 Turnover 的 available=false 表达。metadata.NewWorkday(snapshot) 持有明确市场、来源和历史覆盖，Status 区分已观察交易与来源覆盖，覆盖外为未知，不预测未来节假日。

## 字符转换与均单成交额名称修正

| 旧入口 | 当前入口 |
|---|---|
| `protocol.UTF8ToGBK` | `protocol.GBKToUTF8` |
| `(*protocol.Trade).AvgPrice()` | `(*protocol.Trade).AvgAmount()` |

旧入口已移除，调用方需同步更新名称。`GBKToUTF8` 保持原来的 GBK 解码为 UTF-8、去除 NUL 字节的行为；没有反转转换方向。`AvgAmount()` 保持平均每单成交量乘成交价格、再换算百股单位的公式，返回成交额，现统一为浮点元值。扩展分时记录 `ExMinuteTick.AvgPrice` 的均价字段及其 JSON 键 `avgPrice` 保持不变。


## 命名一致性整理

以下旧名称已移除，项目外的 Go 调用方、工厂签名、自定义 Unit 和匿名嵌入字段需同步更新：

| 原入口 | 当前入口 |
|---|---|
| `tdx.IPool` | `tdx.ClientPool`，具体实现仍为 `*tdx.Pool` |
| `tdx.DialExHq` / `DialExHqDefault` / `DialExHqHosts` | `tdx.DialEx` / `DialExDefault` / `DialExHosts` |
| `httpserver.WithExHqHosts` / `httpserver.WithOptions` | `httpserver.WithExHosts` / `httpserver.WithClientOptions` |
| 元数据选项与 manage 选项 | 已移除，使用纯数据构造；pull.Fetch* 获取元数据，应用自行保存 |
| `xorms.NewSqlite` / `xorms.NewSQLite` / `codes.NewSqlite` / `workday.NewSqlite` | SQLite 引擎使用 `storage/sqlite.Open`，通过 storage 读取数据后使用 `metadata.NewCodes` / `metadata.NewWorkday` 构造查询对象 |
| `pull.NewService` | `pull.New` |
| `codes.CodesBase` / `codes.Base` / `codes.Codes` | `metadata.Codes`，构造入口为 `metadata.NewCodes` |
| `update.Updated` / `update.Tracker` / `update.UpdateModel` | 通用标记对象已删除；行情使用 `storage.BarStore`，SQLite 表模型为私有类型 |
| 标记对象的 `Update(key)` / `Updated(key)` / `Mark(key)` / `IsUpdated(key)` | 使用存储的 `SaveUpdate` / `LoadUpdate`，由调用方计算完成边界；定时任务直接传入 func() error 回调 |
| `pull.Config.Updated` / `pull.Config.Tracker` | 已删除，行情完成标记统一使用 `pull.Config.Store` |
| `QueryMin` / `QueryMinCoverage` / `SaveMin` / `SaveMinComplete` / `SaveMinFallbackComplete` / `SaveMinEmptyScan` | `QueryMinute` / `QueryMinuteCoverage` / `SaveMinute` / `SaveMinuteComplete` / `SaveMinuteFallbackComplete` / `SaveMinuteEmptyScan` |
| `MinFile` / `MinExists` / `MinYearComplete` / `MinYearRecentlyEmpty` / `LastMinUnix` | `MinuteFile` / `MinuteExists` / `MinuteYearComplete` / `MinuteYearRecentlyEmpty` / `LastMinuteTime` |
| `pull.Unit.FetchMin` | `pull.Unit.FetchMinute`，自定义 Unit 和调用方须同步改名 |
| `extend.KlinesToCsv` / `extend.TradeToCsv` | `extend.KlinesToCSV` / `extend.TradesToCSV` |
| `extend.UrlSinaFactorHfq` / `UrlSinaFactorQfq` / `UrlTHSDayKline` | `extend.URLSinaFactorHFQ` / `URLSinaFactorQFQ` / `URLTHSDayKline` |
| `extend.Sina_QFQ` / `Sina_HFQ` / `THS_BFQ` / `THS_QFQ` / `THS_HFQ` | `extend.SinaQFQ` / `SinaHFQ` / `THSBFQ` / `THSQFQ` / `THSHFQ` |
| `protocol.Quote.ReversedBytes0..9` | `protocol.Quote.Reserved0..9` |
| `protocol.Trades.Volume2()` | `protocol.Trades.SideVolumes()`，仍返回主动卖出和主动买入的股数 |

`update` 表的名称与 Key/Time 列、行情数据库表列、`updated.db`、`min/` 数据目录均不变。JSON 名称与当前 Go 字段一致：`Reserved0..9`、`Store`、`ClientPool`，不再导出 Tracker，不保留旧拼写。协议原始字段的数值与单位不变。

源码文件统一使用小写下划线：`codes_server.go`、`pull_trade.go`、`spider_sina.go`、`spider_ths.go`，第三方比例因子模型为 `extend/spider/factor.go`；扩展 HTTP handler 为 `handler_ex.go`，传输写入为 `conn_writer.go`，ZIP 工具为 `lib/zip/zip.go`，对应测试同步。跨市场示例统一使用 [GetKline](../examples/GetKline/main.go) 的证券前缀参数，原 UnifiedClient 独立示例已删除；真实扩展测试名称改为 `TestExLive`，原 `TDX_EXHQ_LIVE` 环境开关保持不变。

## 扩展功能按职责拆包

以下入口从 `extend` 根包迁入职责子包。原函数、类型和常量已从根包移除，不提供兼容转发；直接使用子包以缩小依赖范围。

| 旧入口 | 新入口（省略相同的函数或类型名） |
|---|---|
| `extend.ReadDay/ReadMinute1/ReadMinute5`、`WriteDay/WriteMinute1/WriteMinute5` | `extend/local` |
| `extend.KlinesToCSV/TradesToCSV` | `extend/export` |
| `extend.NewPullTrade/PullTrade` | `pull.NewTrade/Trade` |
| `extend.GetSinaFactor*`、`GetTHSDayKline*`、`Factor/Factors` 及新浪/同花顺常量 | `extend/spider` |
| `extend.ListenCodesHTTP/ListenCodesAndGbbqHTTP` | `extend/httpserver.CodesHandler`；监听由应用管理 |
| `extend.DialCodesHTTP/DialGbbqHTTP`、`CodesHTTP/GbbqHTTP` | `extend/metadata/httpclient.New` 和显式 FetchCodes/FetchGbbq |

完整 import 前缀为 `github.com/injoyai/tdx/v2/`，不再在 `extend` 根包重复导出类型；反射或 `%T` 获得的定义包路径会变为新子包。目录拆分后的进一步接口收敛见本文开头：CSV 直接接收统一业务数据，HTTP 刷新和监听改由应用管理。CSV 表头与 BOM、本地文件的价格/量/时间及占位规则保持不变。第三方比例因子仍不等于 `protocol.Factor` 仿射模型。

`extend` 根包继续提供 `DoIncomes`。`protocol/unit.go` 的实现按职责拆入 `encoding.go`、`code.go`、`format.go`、`math.go` 和 `time.go`，包名、公开名称和算法不变。工具基础设施仍位于 `lib`，私有辅助函数跟随所属功能包，不新增通用 `utils` 包。详见 [扩展功能说明](../extend/README.md)。

## 开发版本接口收敛

当前 v2 仍在开发，不保留旧接口兼容层：

- `pull` 的市场类型、常量与 `ToShares/FromShares` 直接使用 `protocol`；解析统一为 `pull.ParseCode`。完整市场键和简称输入都规范化为同一代码与文件名，例如 `cn/stock.600000` 与 `sh600000` 均使用 `sh600000.db`，不自动搬迁旧文件。
- `codes.NewSQLite/workday.NewSQLite` 已移除，使用 storage 后端读取快照，再通过 metadata.NewCodes/NewWorkday 构造查询；`DefaultCodes` 已移除。定时任务统一使用返回停止函数的 `update.StartTimer`。
- `MQuote.Decode` 和 `ParseBlockFile` 直接返回解码错误，不保留忽略错误的包装或 `Checked` 双入口。
- HTTP `/quote`、`/kline/*`、`/minute/*`、`/trade/*` 按证券前缀选择标准或已启用的扩展池，直接返回 Go 统一结果。`/code` 必须传 `count`，`start` 为 uint32；`/count` 返回整数。
- 指数使用 `/kline/*`；移除 `/index/*` 和重复的扩展行情路由。市场发现使用 `/markets`，区间 K 线使用 `/kline/range`。保留 `/ex/count`、`/ex/instruments` 的原始协议全市场发现能力及 `/ex/quote_list` 的批量报价。
- 数据库保留正常建表、原生/合成分钟覆盖语义、时区隔离与事务保护，不承诺旧库字段或数据口径自动迁移。

详见 [HTTP 服务](../extend/httpserver/README.md)。


## 价格统一为 float64

`protocol.Price`、`protocol.Yuan` 和价格的 `.Float64()` / `.Int64()` / `.Abs()` 方法已移除，无兼容别名。标准 `Kline` 的昨收/OHLC/成交额、`Trade.Price`、`PriceNumber.Price`、五档和集合竞价价格，以及 `Factor.Last` 均改为 `float64`，单位由整数厘改为元。价格差、成交额、均价及 MA/EMA/MACD/BOLL/ATR/VWAP 也返回浮点元值；RSI 保持整数百分比返回值，内部计算使用浮点涨跌额。

```go
// 旧：Close: protocol.Price(12340)，或 protocol.Yuan(12.34)
k := &protocol.Kline{Close: 12.34, Amount: 123400.50}
price := k.Close                // 直接使用元值
adjusted := factor.QFQPrice(price)
difference := math.Abs(k.High - k.Low)
```

- 标准报价的整数分、K 线/实时分时的整数厘和历史分时/分笔的差值仍按协议尺度解码；累计差值先用整数求和，再转换为浮点，避免累计误差。变长整数的校验和尺度转换由对应协议消息的解码器完成，原 `GetPrice`、`CutInt`、`CutInt64` 入口已移除。
- 统一业务接口及 `pull` 数据库已有的价格/金额单位保持不变。`Standard`、`StandardResponse()`、集合竞价及直接返回协议模型的 JSON 价格/金额由整数厘变为浮点元值；已有手写整数厘字面量及序列化快照须主动除以 1000 后迁移，不能按新单位直接读取旧 JSON。
- 扩展分笔 `ExTradeTick.Price` 仍为未确认尺度的原始 `uint32`，业务 `Trade.Price` 仍为 `nil`；这不是已归一化的价格，不猜测换算。
- 指标不再截断为整数厘，可能与旧版结果有小幅差异。复权 `QFQPrice/HFQPrice` 仍按现有约定四舍五入到分；`XRXD.Pre` 返回未截断的浮点参考价。
- `.day/.lc1/.lc5` 的磁盘格式不变：日线价格在写入时四舍五入到整数分，分钟价格/金额保留文件原有的 `float32` 精度，读取时不再截断为厘。写入器拒绝 NaN、Inf 和超出格式范围的值；日线价格还必须处于无符号整数分范围。成交量单位和尾盘占位规则保持不变。
- `float64` 仍有二进制表示误差；计算结果比较应按来源精度和业务容差处理，不提供精确十进制结算保证。


### 指标、复权和 CSV 的数量边界

`KlineResp.Klines()` 直接转换统一记录，不再受旧 `protocol.KlineResp.Count uint16` 的 65535 条限制；仍校验标准来源、原始记录和非负整百股。`StandardResponse()` 的旧类型限制和协议分页偏移上限均保留。

Client 的 `KlineResp.Klines()` 返回整数手成交量，指数从原始股数改为股数 ÷100，随后可交给 metadata.Gbbq.QFQ/HFQ、`VWAP()` 和 `local.Write*`。股票原始量已为手，正常输入的结果不变；不能无损换算为整数手时返回错误。直接调用 `QFQ` / `HFQ` 仍保留输入量额，调用方须先归一化输入。CSV 入口改为直接接收统一 Client 的 List（股数）；复权或本地文件数据需要显式转换，不能直接传入协议整数手。

`spider.GetTHSDayKline` 不再舍入成交量：源股数不是非负整数、溢出或不能整除 100 时返回错误；Full 系列的不复权数据也统一为整数手。

`workday` 刷新直接使用 Client 的 `Time` 源日期，不再要求 `Standard` 原始记录。非法日期导致本轮插入失败；既有固定东八区 15:00 的 Unix 缓存键、数据库结构和交易日查询方式不变。


### 历史分笔下载并入 pull

`extend/pulltrade` 包已删除，功能直接移入 `extend/pull`，不保留兼容转发或类型别名。

| 旧入口 | 当前入口 |
| --- | --- |
| `pulltrade.NewPullTrade(dir)` | `pull.NewTrade(dir)` |
| `pulltrade.PullTrade` | `pull.Trade` |
| `downloader.Pull(ctx, m, code)` / `PullYear(ctx, m, year, code)` | 设置 downloader 的 Pool、Workday、CodeName，调用 `Pull(ctx, startYear, endYear, code)` / `PullYear(ctx, year, code)` |

调用方将 import 改为 `github.com/injoyai/tdx/v2/extend/pull`，并更新构造函数和类型名。分笔请求、交易日遍历、错误传播、取消检查、CSV 路径、各周期合成及量额口径保持不变；CSV 编码仍由 `extend/export` 负责。该下载器直接使用调用方提供的连接池和交易日历，代码表仅用于导出名称，不引入 K 线 Service 的数据库和更新标记初始化。

### pull 直接接入连接池与存储

Config.Pool／ExPool 提供来源连接，Store 提供行情持久化。元数据获取仅需 Pool，不需要存储或行情市场注册。CodesStore、GbbqStore、WorkdayStore 接口及同名配置字段已删除；SQLite 的具体元数据读写方法保留，由应用显式调用。

标准证券代码发现使用 ListCodes(ctx)，每轮固定一次；行情 Unit 可以继续自定义发现，不被框架按市场名强制替换。指定证券时跳过发现。股本与日历查询对象不传入行情服务。生命周期、错误处理和快照发布见[架构说明](data-architecture.md)。

### 请求取消、市场调度和同步输出

Client 原方法集保留；tdx.WithContext(ctx, client) 建立共享连接的请求视图，Pool.DoContext 自动绑定。取消视图不结束原客户端重连生命周期，回调不得关闭视图。首次连接总预算现为 30 秒（含重试与握手），查询 context 可以设置更短期限，SetTimeout 仍只配置单次请求。

自定义行情市场需补充 Schedule() pull.Schedule 或 Config.Schedules 对应条目；内置市场默认边界保留。Schedule.Delay=0 表示不延迟。显式调度变化参与任务去重；原标记保留，不删除历史数据。空发现不写完成标记。自动证券池变化后，可更新 UniverseVersion 并重新构造服务以触发同窗口补拉。

pull 默认不创建终端进度条；需要展示时配置 OnProgress（见 examples/pull）。回调串行执行且 Completed 包含最终失败项，应及时返回且不能同步重入 Update。Retry 是每阶段最大尝试次数；仅瞬时错误重试，RetryDelay 默认五秒、倍增至最多三十秒。自定义可重试错误实现 Temporary() bool 并返回 true。


## 更新选项、阶段事件与公开验收

`Update(true)` 仍仅表示允许提前执行。新增 `UpdateWithOptions(ctx, pull.UpdateOptions{Recheck: true})`，在收盘允许窗口内忽略完成标记，再次执行原增量检查；提前复查同时设置 `AllowEarly: true`。不会重扫已覆盖历史或清空分钟空扫描缓存。复查失败不删除旧成功标记，再试继续使用 Recheck。

`OnProgress` 和 `Progress` 保留原逐证券通知契约。新增 `Config.OnEvent` 接收阶段／市场终态，状态为 skipped、no_data、success、failed。第三方 Unit 可选实现 ResultUnit，提供最后一次尝试的记录数及最新源时间；旧 Unit 仍可运行，但 Result.Known=false，不得把未知计数显示成确定的零。Config 应使用带字段名的结构字面量。详见[公开契约](public-contracts.md)。

其他存储后端导入 `extend/storage/storagetest`，以独立空测试数据集运行基础契约；设置 Fixture.Fail 可验证实际事务失败回滚，无故障钩子时三个事务用例显示 SKIP。[接入说明](../extend/storage/storagetest/README.md)。BarStore 及 Unit 原方法集不变。

SQLite 为修复大小写／Unicode 文件名碰撞，目录格式升级到 source-time-v2。路径段保留小写 ASCII、数字及 `-_.`，其他字节编码为 `~hh`（含大写及 `~`）；`cff/IF2609` 的文件名现为 `cff-~49~462609.db`。**旧 source-time-v1 目录拒绝打开，使用新目录重新构建数据；不兼容、不自动迁移或清理旧目录。** 曾混写的旧文件无法靠文件名恢复证券身份。
