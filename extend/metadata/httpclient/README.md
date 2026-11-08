# 元数据 HTTP 客户端

从独立 HTTP 元数据服务获取代码表和股本事件，使用 metadata 查询对象保存内存快照。调用 Dial 会立即访问服务并安排定时刷新，使用完毕必须 Close；父包 metadata 的内存查询对象不包含这些网络和后台任务。

```go
import "github.com/injoyai/tdx/v2/extend/metadata/httpclient"

index, err := httpclient.DialCodesHTTP("http://127.0.0.1:8080")
if err != nil { return err }
defer index.Close()
name := index.GetName("sh600000")

equity, err := httpclient.DialGbbqHTTP("http://127.0.0.1:8080")
if err != nil { return err }
defer equity.Close()
turnover, available := equity.Turnover("sh600000", at, volumeInShares)
```

默认请求总超时 30 秒，可选参数 spec 指定定时表达式。Close 停止定时任务并取消在途请求；Update 可显式刷新。

客户端读取 `/codes` 的代码记录列表及 `/gbbqs` 的证券事件映射，不使用行情服务的 code/msg/data 包装。股本事件必须包含有效日期、证券代码，且与所在分组一致；经 metadata.NewGbbq 校验、复制和排序后整体发布。刷新失败保留旧快照，空对象或 null 股本响应仍表示空快照。

本包从 extend/metadatahttp 移入，包名改为 httpclient，DialCodesHTTP、DialGbbqHTTP、CodesHTTP、GbbqHTTP 方法和类型名称保留，不提供旧路径转发。服务端见 [HTTP 服务](../../httpserver/README.md)。
