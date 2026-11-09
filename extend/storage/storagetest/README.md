# BarStore 契约测试

此包可被第三方模块直接导入，不依赖 SQLite、MySQL 驱动或 ORM。

```go
func TestBarStore(t *testing.T) {
    storagetest.Run(t, func(t *testing.T) storagetest.Fixture {
        store := newIsolatedTestStore(t)
        t.Cleanup(func() { store.Close() })
        return storagetest.Fixture{Store: store}
    })
}
```

`newIsolatedTestStore` 由适配器实现；每次调用必须创建独立、空的数据集，不能连接生产数据集。工厂负责检查构造错误及清理。运行 `go test`；并发检查使用 `go test -race`。套件不调用 `t.Parallel`，但并发子测试会同时调用同一 Store。

基础用例包括：

- 空查询、最后时间、升序及闭区间查询、单端无限范围、跨年分钟查询。
- 包含 `from` 的尾部替换、整年／全日线替换、重复保存幂等。
- nil 和空切片保留既有行情、覆盖状态和完成标记，不认证空年份。
- `Amount=nil`、真实零和非零金额往返，以及分钟 `Source`。
- 市场、证券、分钟年份、大小写和 Unicode 拼写隔离；覆盖和空扫描状态同样隔离。
- 原生覆盖合并、分笔相接区间合并、不相接区间不冒充连续覆盖。
- 完成标记缺失、首次写入、覆盖和键隔离；空扫描首次保存及覆盖。
- 无效批次拒绝及旧数据保留；并发读写。

## 实际事务故障

工厂可设置 `Fixture.Fail(t, failure)`，在已写入基线后，对 `failure.Code/Year` 安装实际持久化故障：

| `failure.Point` | 注入时机 |
| --- | --- |
| `DayInsert` | 日线尾部删除后，插入失败 |
| `MinuteInsert` | 分钟尾部删除后，插入失败 |
| `CoverageWrite` | 分钟写入成功后，覆盖标记保存失败 |

钩子必须让下一次保存返回错误，保持读取可用，并用 `t.Cleanup` 清除故障。套件随后检查旧行情和旧覆盖状态完整保留。SQL 后端可用测试库触发器，其他后端可注入写入故障；不要在生产实现中增加故障开关。SQLite 的接入见 `../sqlite/contract_test.go`。

`Fail=nil` 时，上述三个事务子测试明确显示 `SKIP`，其余用例仍执行。无效参数校验不等价于事务回滚；缺少故障注入不能宣称事务契约已验证。本套件也不代替后端自己的连接中断、提交失败、重开持久性和部署配置测试。
