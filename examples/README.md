# 示例

保留 6 个典型用法，直接放在 `examples/<示例名>/` 下。每个示例独立运行，客户端或服务在退出时关闭。

| 示例 | 内容 |
| --- | --- |
| [UnifiedClient](UnifiedClient/main.go) | 统一客户端查询沪深、港股、美股日线 |
| [GetQuote](GetQuote/main.go) | 批量实时报价，证券代码作为位置参数传入 |
| [GetCodeAll](GetCodeAll/main.go) | 市场数量、分页或完整代码列表、沪深北股票代码 |
| [GetKline](GetKline/main.go) | 各周期 K 线，支持分页、全部历史和按日期停止 |
| [GetQFQKline](GetQFQKline/main.go) | 前后复权、复权因子、股本与换手率 |
| [PullV2](PullV2/main.go) | 日线/分钟线拉取与落库查询，默认单证券、最近一个月、串行执行 |

## 运行

使用 Go 1.25+，从项目根目录运行；生成的数据统一写入 `./output/`。

```sh
# 跨市场查询与批量报价
go run ./examples/UnifiedClient
go run ./examples/GetQuote sz000001 sh600000

# 北交所完整代码；沪深北股票代码
go run ./examples/GetCodeAll -exchange 2 -all
go run ./examples/GetCodeAll -stocks

# 指数日线；最近 20 根 1 分钟线；向前查询到指定日期
go run ./examples/GetKline -code sh000001
go run ./examples/GetKline -type 7 -count 20
go run ./examples/GetKline -until 20261001

# 标准股票复权
go run ./examples/GetQFQKline -code sh600519

# 指定证券，只拉日线并写入 ./output/pullv2/
go run ./examples/PullV2 -minute=false sz000001 sh000001

# 查看参数，不连接服务器
go run ./examples/GetKline -h

# 编译全部示例并执行离线测试，不执行 main
go test ./examples/...
```

`GetKline` 的 `-type` 使用 `protocol.TypeKline*` 数值，默认 9（日线）；`-all` 查询全部可用历史。`-until` 保留首次早于指定日期的停止记录，不能与 `-all` 同用。

`PullV2` 的日线和分钟线至少开启一种；全市场需显式传入 `-all`，`-force` 可忽略当日完成标记。拉取会更新同名数据库，配置与存储单位见 [pull 文档](../extend/pull/README.md)。

联网结果取决于证券、交易日和服务器保留范围；真实验证遵循 [项目规则](../PROJECT_RULES.md) 的预算与限流要求。更多接口见 [统一 Client](../docs/unified-client.md)、[项目 README](../README.md) 和 [HTTP 服务](../extend/httpserver/README.md)。
