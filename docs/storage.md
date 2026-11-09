# 持久化存储

行情存储契约集中在 extend/storage 的 BarStore。业务模型来自 model；SQLite 和 XORM 实现仅在 extend/storage/sqlite。元数据获取不要求存储接口；SQLite 提供可选的具体读写方法。实现支持并发调用，错误必须返回，不可把数据库错误当作无数据。其他行情后端由应用实现，不内置 MySQL 驱动或适配器。

## SQLite 接入

```go
backend, err := sqlite.New("./output/data")
if err != nil { return err }
defer backend.Close()
syncer, err := pull.New(&pull.Config{
    Pool: pool,
})
if err != nil { return err }
defer syncer.Close()
snapshot, err := syncer.FetchCodes(ctx)
if err != nil { return err }
// 应用选择持久化；FetchCodes 本身不写入 backend。
if err := backend.ReplaceCodes(ctx, snapshot); err != nil { return err }
index, err := metadata.NewCodes(snapshot.Rows)
if err != nil { return err }
_ = index.GetName("sh600000")
```

pool 与 ctx 由应用提供。只获取元数据无需存储或注册行情市场；仅在内存查询时可省略 backend 和 ReplaceCodes。加入 `Store: backend` 后可同步行情，需导入 pull/market。标准证券自动发现通过 ListCodes 提供列表；显式指定证券时可省略。

## 行情接口与元数据具体方法

| 契约／具体类型 | 读取 | 写入 |
| --- | --- | --- |
| sqlite.Metadata 代码 | LoadCodes(ctx, exchanges) | ReplaceCodes(ctx, snapshot) |
| sqlite.Metadata 股本 | LoadGbbq(ctx, codes) | ReplaceGbbq(ctx, snapshot) |
| sqlite.Metadata 日历 | LoadWorkdays(ctx, market) | SaveWorkdays(ctx, snapshot) |
| BarStore | 行情范围、最后时间和状态查询 | 增量行情、分钟覆盖与任务状态 |

CodeSnapshot 的 Exchanges、GbbqSnapshot 的 Codes 明确替换范围。写入必须同时提交记录和 FetchedAt，失败保留旧记录与旧刷新状态。有效空快照仅清除指定范围。Load 返回调用方拥有的记录；缺少刷新状态时 FetchedAt 为零。查询多个范围时，FetchedAt 是这些范围中最早的刷新时间；任一范围未同步时返回零。

日历只接受同市场、同来源、相交或相接的覆盖区间。保存时修正本次扫描范围，保留范围之外已知的日期，并与刷新时间原子提交。拒绝不同来源和不相接的区间。Status 的覆盖语义见[架构说明](data-architecture.md)。

存储不负责代码分类、股本计算、分页、网络重试或调度。格式和范围校验由 model／pull 提供，存储实现仍应在修改数据前拒绝无效批次。事务边界是每次保存调用，不提供跨任务或跨进程调度锁。Fetch 成功仅代表获取完成；持久化成功后才可视为已提交。

## 行情契约

KlineDay／KlineMinute.Amount 与统一 Client 一样使用 *float64，nil 为未知、非 nil 的 0 为真实零。SQLite 在既有 Amount 列保存 SQL NULL，不新增状态列、不批量改写旧库；历史零值仍读为非 nil 的 0，无法从数值自动恢复其原有效性。周期合并只有全部记录金额已知时才求和。

- 按 Code.Market、Code.Code 隔离证券；分钟写入限于指定年份。代码比较保留期货合约大小写。
- 日线时间为 YYYY-MM-DD；分钟为 YYYY-MM-DD HH:MM。不推导 Unix 或来源时区。查询升序、包含 start/end，空端点不限范围。无记录返回空、最后时间返回空字符串。
- SaveDay、SaveMinute 原子删除指定证券 Time>=from 的尾部后插入；from 为空表示覆盖全部日线或指定年分钟。空切片不删除行情或写覆盖状态。
- SaveMinute 的 coverage 非空时，行情和覆盖状态必须一起提交。实际写入起点 from 可能晚于 coverage.From，不能误删服务器未返回的本地前缀。
- 原生覆盖区间按现有语义合并；分笔补全仅合并相接／重叠的区间。空历史扫描的 7 天复查、更早请求范围和时钟回退检查由 pull 处理，后端只保存 From、CheckedAt。
- Identity 稳定且非空，区分数据集，用于任务去重；不得含凭证。相同数据集的连接使用同一身份。
- 市场拉取全部成功才可写完成标记；某市场失败后重试不应影响其他已成功市场。

`BarStore` 直接定义 `LoadUpdate(key)` 和 `SaveUpdate(key, unix)`，完成状态与行情共用同一个存储入口。缺少标记返回 `has=false`，读取不创建占位行；写入支持首次保存和覆盖已有时间。标记使用 Unix 秒，独立于行情源时间。pull 在全部任务成功后保存本轮开始时选定的更新窗口边界，并据此去重，不能用任务结束时间认证后来出现的窗口。既有 Unix 秒标记继续按原比较读取，不自动重写历史状态，无需迁移表结构。pull 负责生成任务键和计算市场边界，存储只负责持久化。完成标记不提供跨进程执行锁。

## 构造与所有权

`sqlite.New(dir)` 是合并后端；`sqlite.NewPull(dir, limit)` 仅打开行情后端。`sqlite.Open(filename)` 只打开原生 SQLite 引擎，使用 SameMapper、单连接。`sqlite.NewMetadata(db)` 复用调用方引擎，不关闭引擎、不更改 mapper 或时区，只创建缺失表。其他数据库不能传给这些 SQLite 构造器。完成标记为 PullStore 内部实现，不再单独创建标记存储或配置键前缀。

创建方关闭连接。先停止所有使用者，再关闭共享存储。pull 不创建或关闭 Store，不导入 SQLite。纯内存查询包不提供 WithDB、WithStore 或 Close。

New／NewPull 将根目录转换为绝对路径并解析符号链接，Identity 返回该规范路径。同一目录的相对、绝对和符号链接入口使用同一任务身份。过去使用相对路径生成的 pull 完成键可能不再命中，首次更新会重新执行增量检查；旧标记保留，不自动删除。

## 文件与旧库

行情使用 `source-time-v2` 文件布局，源时间与量价语义保持不变；为避免大小写和 Unicode 文件名碰撞，旧 `source-time-v1` 数据目录拒绝打开，须使用新目录。不会迁移、覆盖或清理旧文件。新合并后端的 metadata.db 保存元数据，刷新状态与记录原子提交。旧独立 codes.db、gbbq.db 和 workday.db 不会自动搬迁或转换；显式 NewMetadata 可借用原 SQLite 引擎读取既有代码／股本表，但旧完成标记不作为新快照刷新证明。旧日历表缺少覆盖信息，不自动当作已覆盖日历。

SQLite 在读写前校验路径：市场为不含点段的本地相对层级，代码须能安全作为单个文件名；拒绝路径分隔符、通配符、Windows 保留名和尾随点／空格。期货仅接受 `cff/IF2609` 等规范交易所／合约形式；同一期货市场中的字面 `cff-IF2609` 被拒绝。自定义代码 `A/B` 返回错误。市场各段和代码路径仅保留小写 ASCII、数字、`-_.`，其余 UTF-8 字节（包括大写和 `~`）编码为 `~hh`；期货分隔符先替换为 `-`。例如 `cff/IF2609` 映射为 `cff-~49~462609.db`，不与 `cff/if2609` 共用文件；`A`、`a`、字面 `~41` 也彼此隔离。分钟年份限 1..9999；DayFile／MinuteFile 对非法输入返回空字符串。曾因碰撞混写的旧库需另行核对，不能从文件名自动恢复归属。

详见[完整架构与数据语义](data-architecture.md)、[迁移指南](v2-migration.md)。

其他后端可直接使用[公开契约测试包](../extend/storage/storagetest/README.md)。发布兼容边界及 Fetch、Unit、事件口径见[公开契约](public-contracts.md)。
