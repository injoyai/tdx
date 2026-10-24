# 中国市场历史交易日历

`github.com/injoyai/tdx/v2/extend/workday` 根据上证指数日线维护历史交易日缓存，实现根包的 `tdx.TradingCalendar` 接口。

```go
import "github.com/injoyai/tdx/v2/extend/workday"

calendar, err := workday.New(workday.WithSpec("0 3 9 * * *"))
if err != nil {
    return err
}
defer calendar.Close()

isTradingDay := calendar.TodayIs()
```

`New` 默认打开 `./output/data/database/workday.db`、建连并立即刷新，再启动定时更新，因此可能返回数据库或网络错误。默认按固定东八区每天 09:03 刷新；`Is`、`TodayIs`、`Range`、`RangeYear`、`Iter` 和 `IterYear` 用于日期判断与遍历。

`Range` 与 `Iter` 使用 `[start,end)`，从 start 起每隔 24 小时判断交易日。`Iter(...,true)` 只改变顺序，返回相同的日期集合，支持重复遍历；旧倒序曾错误地包含 end 并排除 start，升级后会修正这两个边界。`IterYear` 的倒序同样与正序互为逆序。

它只知道数据源已返回的交易日，不预测未来节假日，也不自动适用于港美股或期货。历史分笔查询按证券所属市场接收 `tdx.TradingCalendar`；本日历可用于中国市场历史查询。

- `WithClient` / `WithDB` 直接注入资源，由调用方负责关闭。
- `WithDialClient` / `WithDialDB` 工厂创建的资源归日历所有，初始化失败时也清理。
- `WithSpec` / `WithRetry` 配置刷新计划与定时重试次数，`WithOptions` 组合配置。
- `Close()` 可重复或并发调用，等待在途更新并停止调度，释放自建资源。关闭后 `Update()` 返回 `net.ErrClosed`，已加载日期仍可查询。

`manage.WithWorkday` 和 `pull.Config.Workday` 使用 `*workday.Workday`；`manage.WithDialWorkday` 使用 `workday.DialFunc`。数据表仍为 `workday`，`WorkdayModel` 的字段保持原样；无需因拆包转换已有数据库。

`tdx.IntegerDay` 继续保留在根包，它返回输入时区的当日零点。迁移对应关系见 [迁移指南](../../docs/v2-migration.md#交易日历与更新工具独立子包)。

默认定时规则公开为 `workday.DefaultSpec`；数据库工厂类型为 `xorms.DialFunc`（`github.com/injoyai/tdx/v2/lib/xorms`）。
