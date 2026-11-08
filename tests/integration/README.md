# 跨包集成测试

这里的离线测试仅使用公开接口，检查多个包共同遵守的契约，不访问真实行情服务器。

| 文件 | 范围 |
| --- | --- |
| `api_contract_test.go` | JSON 公开字段名 |
| `adjusted_export_test.go` | 纯计算复权与本地导出的成交量口径 |
| 元数据同步与事务 | 位于 pull/metadata_test.go 和 storage/sqlite/metadata_test.go，验证失败回滚、快照状态与资源归属 |
| `consistency_test.go` | 代码格式、交易日历、时区、成交量和存储的一致性 |

从项目根目录运行：

```sh
go test ./tests/...
go test -race . ./tests/...
```

`go test ./...` 会自动发现这些测试；`go test .` 仅运行根包内部测试。需要访问 `tdx` 私有实现的测试留在根目录，各子包自身的单元测试留在对应源码目录。
