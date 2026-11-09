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
index := metadata.NewCodes(snapshot.Rows)
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

- 按 Code.Market、Code.Code 隔离证券；分钟写入限于指定年份。代码比较保留期货合约大小写。
- 日线时间为 YYYY-MM-DD；分钟为 YYYY-MM-DD HH:MM。不推导 Unix 或来源时区。查询升序、包含 start/end，空端点不限范围。无记录返回空、最后时间返回空字符串。
- SaveDay、SaveMinute 原子删除指定证券 Time>=from 的尾部后插入；from 为空表示覆盖全部日线或指定年分钟。空切片不删除行情或写覆盖状态。
- SaveMinute 的 coverage 非空时，行情和覆盖状态必须一起提交。实际写入起点 from 可能晚于 coverage.From，不能误删服务器未返回的本地前缀。
- 原生覆盖区间按现有语义合并；分笔补全仅合并相接／重叠的区间。空历史扫描的 7 天复查、更早请求范围和时钟回退检查由 pull 处理，后端只保存 From、CheckedAt。
- Identity 稳定且非空，区分数据集，用于任务去重；不得含凭证。相同数据集的连接使用同一身份。
- 市场拉取全部成功才可写完成标记；某市场失败后重试不应影响其他已成功市场。

## 构造与所有权

`sqlite.New(dir)` 是合并后端；`sqlite.NewPull(dir, limit)` 仅打开行情后端。`sqlite.Open(filename)` 只打开原生 SQLite 引擎，使用 SameMapper、单连接。`sqlite.NewMetadata(db)`、`sqlite.NewUpdate(db, prefix...)` 复用调用方引擎，不关闭引擎、不更改 mapper 或时区，只创建缺失表。其他数据库不能传给这些 SQLite 构造器。

创建方关闭连接。先停止所有使用者，再关闭共享存储。pull 不创建或关闭 Store，不导入 SQLite。纯内存查询包不提供 WithDB、WithStore 或 Close。

## 文件与旧库

行情保留 source-time-v1 文件布局；原日线额外派生列不自动删除，新业务模型不映射它们。新合并后端的 metadata.db 保存元数据，刷新状态与记录原子提交。旧独立 codes.db、gbbq.db 和 workday.db 不会自动搬迁或转换；显式 NewMetadata 可借用原 SQLite 引擎读取既有代码／股本表，但旧完成标记不作为新快照刷新证明。旧日历表缺少覆盖信息，不自动当作已覆盖日历。

详见[完整架构与数据语义](data-architecture.md)、[迁移指南](v2-migration.md)。
