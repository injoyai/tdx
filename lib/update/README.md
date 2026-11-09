# 定时更新

`github.com/injoyai/tdx/v2/lib/update` 只提供可停止的定时执行工具，不依赖根包，不读写完成标记。行情同步由 pull 计算市场更新边界，通过 `storage.BarStore` 保存完成状态，见[存储接口说明](../../docs/storage.md)。

## 可停止的定时执行

```go
import "github.com/injoyai/tdx/v2/lib/update"

stop, err := update.StartTimer("0 1 9 * * *", 3, func() error { return service.Update() })
if err != nil {
    return err
}
defer stop()
```

第三个参数为 `func() error`，可直接传普通函数或在闭包中调用 service.Update()；不再要求 update.Updater 对象接口。nil 回调返回错误。`StartTimer` 先校验表达式并立即执行一次更新，成功后才启动调度；初次更新失败直接返回错误。定时执行最多尝试指定次数，至少一次，失败后的重试间隔仍为五分钟，同一任务不重叠执行。cron 默认使用带秒表达式和固定东八区。

`stop` 可重复或并发调用，取消重试等待并等待在途更新退出；`Update()` 本身应有有限超时。应在更新回调外调用 `stop`，避免等待自身退出。

只提供 `StartTimer`，调用方保存并调用 `stop` 释放任务。拆包后的对应关系见 [迁移指南](../../docs/v2-migration.md#交易日历与更新工具独立子包)。
