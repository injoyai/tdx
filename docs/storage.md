# 可注入的持久化存储

行情、代码表、股本、交易日历分别定义业务存储接口，应用可以独立选择实现。接口不要求 SQL 或 XORM；默认仍使用 SQLite，原文件布局、表结构、源时间和量价单位保持不变，已有源时间格式数据库无需迁移。

| 数据 | 接口 | 注入入口 | 默认实现 |
|---|---|---|---|
| 日线、分钟线及其状态 | `pull.Store` | `pull.Config.Store` | `pull.SQLiteStore` |
| 代码快照及更新标记 | `codes.Store` | `codes.WithStore` | `codes.SQLStore` + SQLite |
| 股本/除权除息快照及更新标记 | `gbbq.Store` | `gbbq.WithStore` | `gbbq.SQLStore` + SQLite |
| 历史交易日 | `workday.Store` | `workday.WithStore` | `workday.SQLStore` + SQLite |
| 通用更新标记 | `update.Store` | `update.NewWithStore` | `update.SQLStore` |

## 行情存储

应用实现 `pull.Store`，然后传入 `Config.Store`：

```go
func newPull(store pull.Store, m *manage.Manage) (*pull.Service, error) {
    return pull.New(&pull.Config{
        Store: store,
        Codes: []string{"sz000001"},
        Manage: m,
    })
}
```

仍须导入 `github.com/injoyai/tdx/v2/extend/pull/market` 注册内置市场。连接源和拉取范围的配置方式不变；`Service.QueryDay/QueryMinute`、`SaveDay/SaveMinute` 及历史补全方法继续使用业务对象。`Dir`、`MaxCachedEngines` 只用于未注入 Store 时自动创建的 SQLite 存储。注入后不检查本地格式标记，也不创建 `updated.db` 或其他本地文件。

`Store` 包含日线与分钟线的最后时间、范围查询、增量写入、分钟覆盖状态、空历史扫描记录，以及 `update.Store` 的更新标记方法。不能只保存行情而丢弃状态，否则重启后会重复扫描或错误跳过历史。

实现必须遵守：

- 每次访问按 `Code.Market`、`Code.Code` 隔离证券；分钟写入只影响指定年份。共享 SQL 表可使用 `(Market, Code, Time)` 组合主键，代码比较应保留期货合约大小写语义。
- 行情时间沿用源字符串：日线 `YYYY-MM-DD`、分钟线 `YYYY-MM-DD HH:MM`，不附加时区、不推导 Unix。`Service` 保留格式、年份和范围校验。
- 范围查询按时间升序，包含 start/end；空端点不限制。无记录返回空切片或 nil，最后时间为空字符串，不能把存储错误当作无数据。
- `SaveDay`、`SaveMinute` 原子删除该证券 `Time >= from` 的尾部并插入新数据；分钟操作限于指定年。`from` 为空表示覆盖该证券的全部日线或指定年分钟线。空切片不修改已有数据或完成状态。
- `SaveMinute(..., coverage)` 的 coverage 非空时，同一事务/原子操作提交行情和覆盖标记；失败两者都不生效。`from` 是实际覆盖起点，可能晚于 `coverage.From`，不可按覆盖标记起点误删服务器已不返回的本地前缀。
- 相同 `TradeFallback` 的覆盖记录在原生模式下合并；分笔模式仅在区间相接或重叠时合并，取更早 From、更晚 Through。不同模式或不相接的分笔范围替换旧标记。`MinuteCoverage.ID` 是旧 SQLite 表的内部键，其他实现按证券、年份定位。
- 空历史扫描的 7 天有效期、更早请求范围和时钟回退判断由 `Service` 处理；存储只负责记录 `From/CheckedAt`。`CheckedAt` 和更新标记使用 Unix 秒，均不是行情源时间。
- 所有 Store 方法支持并发调用，数据库错误必须返回。更新完成标记只能在对应市场拉取全部成功后写入。

`Identity()` 必须稳定且非空，用于更新任务的摘要。不同存储数据集不能共用同一身份；相同数据集的不同连接应使用相同身份。不要使用含账号密码的 DSN；可采用不含凭证的库名和业务命名空间。SQLite 默认沿用规范化后的目录作为身份，旧任务摘要可继续使用。应用自定义实现时，由应用定义身份。

`Config.Tracker` 仍可显式覆盖任务去重器；不设置时，标记保存到同一个 Store，不暗中使用 SQLite。

## 代码表、股本与交易日历

三个模块都通过 `WithStore(store)` 注入，不能同时使用 `WithDB/WithDialDB`。未注入时保留原默认路径及初始化/定时刷新行为；构造函数仍会立即刷新，可能返回网络或存储错误。

- `codes.Store.Load` 读取代码快照；`Save` 原子提交完整新快照、保留未变化证券的主键和创建时间、删除退市记录，并返回提交后的记录供内存缓存发布。
- `gbbq.Store.Load` 读取股本记录；`Save` 原子替换完整快照。空快照表示数据源成功返回空，与行情的空增量跳过不同。
- `workday.Store.Load` 按 Unix 升序返回日期；`Save` 原子追加新交易日，失败不得留下部分批次。沿用现有固定东八区 15:00 键及 `WorkdayModel` 字段。代码中的时区处理属于日历语义，与行情源字符串不同。
- 代码表和股本的 Store 同时实现 `update.Store`。更新标记写入失败会返回错误，已提交的数据可在下次刷新时重新读取，不标记失败刷新为完成。
- 返回的数据供缓存读取，调用方应视为只读。多个缓存对象共用同一 Store 时，实现应协调并发保存。

已有 XORM 引擎也可使用 `codes.NewSQLStore(db)`、`gbbq.NewSQLStore(db)`、`workday.NewSQLStore(db)` 适配；`WithDB/WithDialDB` 保留，内部走同一实现。SQLStore 不关闭调用方的引擎。

MySQL 驱动由应用依赖并注册，再通过 `xorms.New("mysql", dsn, ...)` 创建引擎。元数据可使用上述 SQLStore；行情需要应用实现 `pull.Store` 的表结构和隔离条件。当前仓库不内置 MySQL 行情适配器，也不自动导入 SQLite 存量数据。SQLite 已完成回归验证，其他数据库须针对目标版本验证建表、字段类型、事务、排序和重复写入，不能仅凭引擎创建成功认定兼容。

## 生命周期与原有调用迁移

- `Service` 自动创建的 SQLite 存储由 `Service.Close()` 关闭。
- `Config.Store` 和各模块 `WithStore` 注入的存储始终由调用方关闭，包括构造失败时；关闭缓存/服务后再关闭共享存储。
- 元数据原有 `WithDB` 不接管数据库；`WithDialDB` 创建的数据库仍归缓存对象，初始化失败也会关闭。客户端的注入与工厂资源归属规则不变。
- `Service.Close` 拒绝后续业务存储操作和更新；调用方仍应协调正在执行的拉取任务，再关闭自己管理的共享资源。元数据缓存继续等待在途刷新后停止调度。

原 `Service.DayFile/MinuteFile/MinuteExists` 属于 SQLite 文件能力，现位于 `SQLiteStore`，自定义 Store 无须伪造文件路径。需要显式访问这些能力时：

```go
store, err := pull.NewSQLiteStore("./output/pull", 32)
if err != nil {
    return err
}
defer store.Close()
service, err := pull.New(&pull.Config{Store: store, Codes: []string{"sz000001"}})
if err != nil {
    return err
}
defer service.Close()
filename := store.DayFile(code) // code 为 pull.Code；仅用于 SQLite 文件操作
_ = filename
```

业务代码应优先使用 `QueryDay/QueryMinute` 等存储无关接口。`Code.DayFile/MinuteFile` 仍是纯路径计算辅助方法，不参与 Service 的存储选择。
