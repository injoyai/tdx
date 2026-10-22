# 代码表接口与缓存

`github.com/injoyai/tdx/v2/extend/codes` 提供标准沪深北市场的代码表读取接口、纯内存代码表和 SQLite 自动更新缓存。

## 内存代码表

```go
cache := codes.NewBase()
cache.Update(codes.CodeModels{
    {Exchange: "sh", Code: "600000", Name: "浦发银行"},
})
var reader codes.Interface = cache
name := reader.GetName("sh600000")
```

上述示例需导入 `github.com/injoyai/tdx/v2/extend/codes`。`NewBase` 不联网、不打开数据库，也不启动定时任务。

`Interface` 保留 `Iter`、`Get`、`GetName`，以及股票、ETF、指数的记录列表和完整代码列表查询方法。代码使用 `sh600000` 等带市场前缀的形式；查询只读取当前缓存，不触发刷新。记录指针沿用共享读取语义，调用方应将其视为只读数据。

## 持久化缓存

```go
cache, err := codes.New(codes.WithSpec("0 1 9 * * *"))
if err != nil {
    return err
}
defer cache.Close()
```

`New` 默认立即建连并刷新代码表，然后启动定时更新；可能返回网络或数据库错误。`NewSQLite` 提供同一构造行为。默认路径仍为 `./output/data/database/codes.db`，刷新时间仍为固定东八区每天 09:01。

- `WithClient` / `WithDB` 直接注入资源，资源由调用方关闭。
- `WithDialClient` / `WithDialDB` 工厂创建的资源归缓存对象所有，初始化失败时也会清理。
- `WithSpec` / `WithRetry` 配置更新计划与重试次数，`WithOptions` 组合选项。
- `Update()` 串行刷新；完整拉取成功后事务提交数据库，再发布内存快照。失败不发布部分结果。
- `Close()` 可重复或并发调用，等待在途更新并停止调度，释放自建资源；关闭后 `Update()` 返回 `net.ErrClosed`，已有内存数据仍可查询。

管理器使用 `manage.WithCodes(reader)` 或 `manage.WithDialCodes(factory)` 注入代码表。`extend.CodesHTTP` 同样实现 `codes.Interface`；HTTP 服务入口保留在 `extend`，配置使用 `codes.Option`。

`DefaultCodes` 保留为显式共享缓存入口，不由构造函数自动赋值，行情查询不依赖它。根包的标准连接使用内部纯内存元数据校准价格，不会初始化持久化代码缓存。接口和模型通过内部共享实现保持一致，应用只需引用本包。

原根包入口的对应关系见 [迁移指南](../../docs/v2-migration.md#代码表接口与缓存独立子包)。本次不改变已有 SQLite 表结构、模型 JSON 字段或数据单位。

默认定时规则公开为 `codes.DefaultSpec`；数据库工厂类型为 `xorms.DialFunc`（`github.com/injoyai/tdx/v2/lib/xorms`）。
