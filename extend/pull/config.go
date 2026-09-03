package pull

import (
	"time"

	"github.com/injoyai/tdx"
)

// PullConfig 拉取配置，通过代码参数传入（本库作为第三方引用，配置不落文件）。
type PullConfig struct {
	Dir    string   // 数据根目录（必填）
	Codes  []string // 拉取代码列表（自动路由市场，见 ParseCode）；空 = 全部注册市场自动发现
	Day    bool     // 是否拉日线（默认 true，见 NewService 归一化）
	Minute bool     // 是否拉1分钟线（默认 true）

	Goroutines int          // 并发数，默认 8
	StartAt    string       // 起始日期 YYYYMMDD，首次全量的最早日期；空 = 只拉最近两年
	Retry      int          // 单条失败重试次数，默认 3
	Updated    *tdx.Updated // 增量去重库（可选，不传则自动创建于 Dir/updated.db）
	Manage     *tdx.Manage  // 标准行情(7709)连接源（可选）。沪深股票/指数/ETF/板块 Unit 使用
	ExPool     tdx.IPool    // 扩展行情(7727)连接池（可选）。期货/港股/美股 Unit 使用
	Workday    *tdx.Workday // 交易日历（可选，不传则不判断交易日）
}

// Start 解析起始日期；未设置或非法时返回零值（Service 会归一化为最近两年）。
func (c PullConfig) Start() time.Time {
	if c.StartAt == "" {
		return time.Time{}
	}
	t, err := time.ParseInLocation("20060102", c.StartAt, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}
