# 标准行情管理器

`github.com/injoyai/tdx/v2/extend/manage` 组合标准行情连接池、代码缓存、交易日历、股本缓存和工作日任务。构造入口为 `manage.New`，管理器类型为 `*manage.Manage`；基础行情查询仍使用根包的 `tdx.Client`。

```go
package main

import (
    "log"

    "github.com/injoyai/tdx/v2"
    "github.com/injoyai/tdx/v2/extend/manage"
)

func main() {
    m, err := manage.New(manage.WithClients(2))
    if err != nil {
        log.Fatal(err)
    }
    defer m.Close()

    err = m.Do(func(c tdx.Client) error {
        _, err := c.GetKlineDay("sh000001", 0, 20)
        return err
    })
    if err != nil {
        log.Print(err)
    }
}
```

`New` 默认立即建连并初始化代码缓存与交易日历，因此可能返回网络或数据库错误。缓存默认位于 `./output/data/database`，使用各缓存子包的默认刷新配置。股本缓存默认为空占位对象，调用 `InitGbbq()` 时才初始化；pull 的股票日线任务会调用该方法。

## 资源与选项

- `WithClients` 设置连接池大小，默认 `manage.DefaultClients`（1）。
- `WithPool` / `WithCodes` / `WithWorkday` / `WithGbbq` 注入资源，资源由调用方关闭。
- 对应 `WithDial*` 工厂返回的资源归管理器所有，初始化失败或 `Close()` 时自动释放。工厂返回 nil 或接口中的空指针会导致构造失败。
- `WithDialGbbqDefault()` 在构造时初始化默认股本缓存；`WithOptions` 组合多个选项。
- `InitGbbq()` 只替换管理器自己创建的默认占位对象；显式提供的股本对象保持原样。使用 `WithGbbq(&gbbq.Gbbq{})` 可明确禁用股本回填。

组件已全部提供时，构造不会从池中借用连接。管理器的代码表使用 [`codes.Interface`](../codes/README.md)，持久化代码缓存通过 `codes.New` 和 `codes.With*` 配置；交易日历使用 [`*workday.Workday`](../workday/README.md)，通过 `workday.New` 和 `workday.With*` 配置；股本使用 [`gbbq.Interface`](../gbbq/README.md)，通过 `gbbq.New` 和 `gbbq.With*` 配置；连接源仍使用根包的 `tdx.ClientPool` / `tdx.Client`。

## 工作日任务与关闭

`AddWorkdayTask` 使用带秒的 cron 表达式，默认按固定东八区调度；仅在交易日历确认当天为工作日时执行回调。任务可能重叠执行，调用方需按任务性质控制并发。

`Close()` 可重复或并发调用：停止调度，按创建顺序的逆序释放自建资源，然后等待在途任务退出。它没有强制截止时间，任务自身须能有界退出；外部共享连接池的阻塞由调用方解除。

任务回调不能同步调用 `Close()`，否则会等待自身退出。任务需要结束管理器时，发送信号通知外部调用方关闭，示例见 [迁移指南](../../docs/v2-migration.md#管理器独立子包)。

原 `tdx.NewManage`、`tdx.Manage`、管理器选项迁移说明也见该指南。
