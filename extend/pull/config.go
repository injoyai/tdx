package pull

import (
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/manage"
	"github.com/injoyai/tdx/v2/lib/update"
	"github.com/injoyai/tdx/v2/protocol"
)

// Config 拉取配置，通过代码参数传入（本库作为第三方引用，配置不落文件）。
type Config struct {
	Store         Store    // 自定义存储；由调用方关闭。非空时不创建本地数据库
	Dir           string   // 默认 SQLite 数据根目录；Store 为空时必填
	Codes         []string // 拉取代码列表（自动路由市场，见 ParseCode）；空 = 全部注册市场自动发现
	Day           bool     // 是否拉日线（默认 true，见 New 归一化）
	Minute        bool     // 是否拉1分钟线（默认 true）
	TradeFallback bool     // 是否用历史分笔补充缺失分钟线，仅股票/ETF/LOF；默认 false

	Goroutines       int             // 并发数，默认 8
	MaxCachedEngines int             // 默认 SQLite 存储空闲时最多缓存的行情数据库引擎数，默认 32；使用中的引擎不淘汰
	StartAt          string          // 起始日期 YYYYMMDD，首次全量的最早日期；空 = 只拉最近两年
	Retry            int             // 每个代码的日线/分钟线各自最大尝试次数，默认 3
	Tracker          *update.Tracker // 增量去重库（可选，不传则使用 Store 保存更新标记）
	Manage           *manage.Manage  // 标准行情(7709)连接源（可选）。沪深股票/指数/ETF/板块 Unit 使用
	ExPool           tdx.ClientPool  // 扩展行情(7727)连接池（可选）。期货/港股/美股 Unit 使用
}

// Start 解析起始日期；time.Time 仅作为东八区日历日期容器，不定义扩展行情的来源时区。
// 未设置或非法时返回零值。New 会拒绝非法日期。
func (c Config) Start() time.Time {
	t, _ := c.parseStart()
	return t
}

func (c Config) parseStart() (time.Time, error) {
	if c.StartAt == "" {
		return time.Time{}, nil
	}
	return time.ParseInLocation("20060102", c.StartAt, protocol.Location())
}
