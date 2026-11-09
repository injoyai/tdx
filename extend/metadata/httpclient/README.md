# 元数据 HTTP 客户端

通过显式请求获取代码表和股本查询快照。New 不访问网络、不启动后台任务；客户端不持有查询快照，也不需要 Close。

```go
import "github.com/injoyai/tdx/v2/extend/metadata/httpclient"

source := httpclient.New("http://127.0.0.1:8080", nil)
index, err := source.FetchCodes(ctx)
if err != nil { return err }
name := index.GetName("sh600000")

equity, err := source.FetchGbbq(ctx)
if err != nil { return err }
turnover, available := equity.Turnover("sh600000", at, volumeInShares)
```

New 的第二个参数可注入调用方的 *http.Client；nil 使用 30 秒总超时。每次 Fetch 都接收 context，超时和取消随请求传播。刷新频率、快照发布和资源关闭由应用安排；只有成功时才发布新快照，失败继续使用之前的查询对象。可通过 atomic.Pointer 发布快照。

客户端读取 `/codes` 的记录列表及 `/gbbqs` 的证券事件映射，保留元数据服务的原始 JSON 格式。股本事件须包含有效日期和代码，且与分组一致；通过 metadata.NewGbbq 完整校验、复制和排序后返回。空对象或 null 股本响应表示空快照。

FetchCodes 在构造查询对象前校验整份记录：exchange 必须为小写 sh/sz/bj，code 为不含前缀的六位数字，证券键不得重复，不能含 null 行；名称最多 128 字符、代码最多 32 字符。非法响应返回 nil 和错误，不返回部分索引。`[]` 或 `null` 代码响应仍表示有效空快照。

原 DialCodesHTTP、DialGbbqHTTP、CodesHTTP、GbbqHTTP 及内部定时缓存已删除。服务端通过 [CodesHandler](../../httpserver/README.md) 挂载到应用管理的 HTTP 服务。
