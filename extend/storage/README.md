# 存储接口与实现

本包集中定义 BarStore、CodesStore、GbbqStore、WorkdayStore。各小接口可由同一个后端实现，业务模型来自 model。SQLite/XORM 集中在 [sqlite](sqlite/README.md)，其他数据库由应用实现接口，不要求使用 ORM。

行情实现可复用 model.ValidateDays、ValidateMinutes、ValidateTimeRange 校验源日期、分钟年份和查询范围；空数据不删行情的契约仍由实现入口保证。pull 与 SQLite 均直接使用 model 类型，不提供同名别名。

接入示例、事务和范围语义见[存储契约](../../docs/storage.md)；整体职责见[架构说明](../../docs/data-architecture.md)。
