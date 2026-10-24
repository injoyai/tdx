# 更新标记与定时更新

`github.com/injoyai/tdx/v2/lib/update` 提供共享的更新基础设施，不依赖根包。`mark.go` 负责数据库完成标记，`timer.go` 负责可停止的定时执行。

## 持久化完成标记

```go
import "github.com/injoyai/tdx/v2/lib/update"

marks, err := update.New(db, 15, 1)
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

`db` 为调用方提供的 `*xorms.Engine`，关闭仍由调用方负责。`New` 初始化 `update` 表；记录模型为 `Record`，数据库字段和表名保持原样。每天的更新边界按固定东八区计算，例中的 15:01 表示当天边界。

`IsUpdated(key)` 只查询，不创建占位行；`Mark(key)` 可以直接写入首次标记。标记表示任务已完成，不是执行锁，调用方仍需按业务要求安排并发。`pull.Config.Tracker` 接受 `*update.Tracker`。

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
