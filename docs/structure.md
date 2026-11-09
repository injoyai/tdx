# 源码目录

本项目以 tdx 行情 SDK 为主体。根包保留客户端、连接、拨号、连接池及共享查询；基于 SDK 的同步、查询工具、存储与 HTTP 服务集中在 extend。

```text
tdx/
├── *.go                 Client、连接、拨号、连接池与共享查询
├── protocol/            协议帧、编解码、市场规则及行情计算
├── extend/              基于 tdx SDK 的扩展能力
│   ├── model/           扩展共享业务记录、快照与输入校验
│   ├── metadata/        证券、股本、历史日历的内存查询
│   │   └── httpclient/  可选的元数据 HTTP 获取与刷新
│   ├── pull/            元数据获取与行情同步、历史分笔下载
│   │   └── market/      内置市场适配器及注册
│   ├── storage/         行情 BarStore 和事务契约
│   │   └── sqlite/      SQLite/XORM 实现、表模型及文件布局
│   ├── export/          CSV 导出
│   ├── local/           通达信本地文件读写
│   ├── spider/          第三方数据源
│   └── httpserver/      行情与元数据 HTTP 服务
├── lib/                 文件解密、定时器、ZIP 工具
├── internal/testutil/   仅供测试复用的存储替身
├── tests/integration/   跨包契约和数据一致性测试
├── examples/            独立可运行示例
├── docs/                接口、架构、迁移和维护文档
│   └── images/          文档图片
└── output/              运行与验证产物，不提交
```

依赖方向为扩展使用 SDK；根包和 protocol 不导入 extend。标准连接仅在连接内保存报价 Decimal，用于协议价格校准，不引用扩展证券索引、存储接口或业务模型。

扩展内部，pull 通过 Client 获取数据，元数据直接返回快照，行情通过 storage.BarStore 提交，model 提供共享记录与校验。metadata 直接查询内存数据，不依赖 pull、storage 或 HTTP 客户端。metadata/httpclient 是独立子包，显式请求后返回完整查询快照；调度和快照发布由应用负责，客户端不持有后台任务，不需要 Close。导入 metadata 不会引入该子包。

单元测试与实现同目录；公开跨包行为放在 tests/integration，测试替身放在 internal/testutil。示例独立运行，生成数据统一写入 output。

storage/sqlite 的 bars.go、coverage.go、empty_scan.go 分别负责行情、覆盖和空扫描记录，format.go 校验磁盘源时间格式。源码文件名不改变数据库表名、数据目录、旧格式隔离或资源归属。

lib/gbbq 保留原始股本文件下载、解密和解析，与 metadata.Gbbq 的事件查询职责不同。lib/update 只提供定时工具；行情完成标记由 storage.BarStore 定义、pull 使用，不承担统一管理或隐式启动任务。

目录变更后的导入对应关系见 [迁移指南](v2-migration.md)，完整数据职责和生命周期见 [数据架构](data-architecture.md)。
