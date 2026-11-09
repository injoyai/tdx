# 存储接口与实现

本包仅定义行情增量同步所需的 BarStore，业务模型来自 model。元数据获取返回快照，由应用决定是否以及如何持久化，不要求实现 codes、gbbq 或 workday 的存储接口。SQLite/XORM 集中在 [sqlite](sqlite/README.md)，其中保留可直接调用的元数据读写方法；其他行情后端由应用实现 BarStore，不要求使用 ORM。

行情实现可复用 model.ValidateDays、ValidateMinutes、ValidateTimeRange 校验源日期、分钟年份和查询范围；空数据不删行情的契约仍由实现入口保证。pull 与 SQLite 均直接使用 model 类型，不提供同名别名。

接入示例、事务和范围语义见[存储契约](../../docs/storage.md)；整体职责见[架构说明](../../docs/data-architecture.md)。

第三方后端可直接导入 [storagetest](storagetest/README.md) 运行范围、替换、空批次、金额、隔离和事务故障契约测试。公开兼容边界见[公开契约](../../docs/public-contracts.md)。
