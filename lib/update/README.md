# 更新标记与定时更新

`github.com/injoyai/tdx/v2/lib/update` 提供共享的更新基础设施，不依赖根包。`mark.go` 负责通过 Store 读写完成标记，`timer.go` 负责可停止的定时执行。

## 持久化完成标记

```go
import "github.com/injoyai/tdx/v2/lib/update"

marks, err := update.NewWithStore(store, 15, 1)
if err != nil {
    return err
}
done, err := marks.IsUpdated("daily-task")
if err != nil {
    return err
}
if !done {
    if err := runTask(); err != nil {
        return err
    }
    return marks.Mark("daily-task")
}
```

`store` 必须实现本包 Store，由调用方创建和关闭；不提供默认数据库；可用 `sqlite.NewUpdate(db)` 复用调用方的 SQLite 原生 `*xorm.Engine`。每天的更新边界按固定东八区计算，例中的 15:01 表示当天边界。

`IsUpdated(key)` 只查询，不创建占位行；`Mark(key)` 可以直接写入首次标记。标记表示任务已完成，不是执行锁，调用方仍需按业务要求安排并发。

`IsUpdatedSince(key, since)` 查询在指定时刻（含）之后是否已完成，忽略 `New` 的固定东八区每日边界。`IsUpdated` 的既有行为不变。

## 可停止的定时执行

```go
stop, err := update.StartTimer("0 1 9 * * *", 3, updater)
if err != nil {
    return err
}
defer stop()
```

`updater` 实现 `update.Updater`（`Update() error`）。`StartTimer` 先校验表达式并立即执行一次更新，成功后才启动调度；初次更新失败直接返回错误。定时执行最多尝试指定次数，至少一次，失败后的重试间隔仍为五分钟，同一任务不重叠执行。cron 默认使用带秒表达式和固定东八区。

`stop` 可重复或并发调用，取消重试等待并等待在途更新退出；`Update()` 本身应有有限超时。应在更新回调外调用 `stop`，避免等待自身退出。

只提供 `StartTimer`，调用方保存并调用 `stop` 释放任务。拆包后的对应关系见 [迁移指南](../../docs/v2-migration.md#交易日历与更新工具独立子包)。

## 自定义标记存储

`Store` 提供 `LoadUpdate(key)` 和 `SaveUpdate(key, unix)`，缺少标记返回 `has=false`。`NewWithStore(store,hour,minute)` 复用原有每日边界计算，不打开数据库、不接管存储生命周期。`sqlite.NewUpdate(db, prefix...)` 创建缺失的标记表并可加键前缀，已有表不自动迁移；引擎仍由调用方关闭。SQLite 沿用 update 表；其他数据库由调用方实现 update.Store。标记时间是 Unix 秒，与行情源时间无关。
