# 跨包集成测试

这里的离线测试仅使用公开接口，检查多个包共同遵守的契约，不访问真实行情服务器。

| 文件 | 范围 |
| --- | --- |
| `api_contract_test.go` | JSON 公开字段名 |
| `metadata_test.go` | 元数据初始化失败时的资源释放 |
| `metadata_ownership_test.go` | 注入资源与工厂资源的关闭归属 |
| `consistency_test.go` | 代码格式、交易日历、时区、成交量和存储的一致性 |

从项目根目录运行：

```sh
go test ./tests/...
go test -race . ./tests/...
```

`go test ./...` 会自动发现这些测试；`go test .` 仅运行根包内部测试。需要访问 `tdx` 私有实现的测试留在根目录，各子包自身的单元测试留在对应源码目录。
