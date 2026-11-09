package pull

import (
	"context"
	"reflect"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/storage"
	"github.com/injoyai/tdx/v2/protocol"
)

// Config 拉取配置，通过代码参数传入（本库作为第三方引用，配置不落文件）。
type Config struct {
	Store         storage.BarStore // 自定义存储；由调用方关闭。行情同步必须显式提供
	Codes         []string         // 拉取代码列表（自动路由市场，见 ParseCode）；空 = 全部注册市场自动发现
	Day           bool             // 是否拉日线（默认 true，见 New 归一化）
	Minute        bool             // 是否拉1分钟线（默认 true）
	TradeFallback bool             // 是否用历史分笔补充缺失分钟线，仅股票/ETF/LOF；默认 false

	Goroutines int            // 并发数，默认 8
	StartAt    string         // 起始日期 YYYYMMDD，首次全量的最早日期；空 = 只拉最近两年
	Retry      int            // 每个代码的日线/分钟线各自最大尝试次数，默认 3
	Pool       tdx.ClientPool // 标准行情(7709)连接池，由调用方关闭
	ExPool     tdx.ClientPool // 扩展行情(7727)连接池（可选）。期货/港股/美股 Unit 使用
	// ListCodes supplies standard-market discovery once per Update; explicit Codes bypass it.
	ListCodes func(context.Context) ([]model.Code, error) `json:"-"`
	// UniverseVersion changes when an automatically discovered universe changes.
	// Explicit Codes already participate in the completion key.
	UniverseVersion string
	// Schedules overrides update boundaries. Custom markets must provide an entry
	// here or implement Schedule() Schedule on their Unit.
	Schedules  map[protocol.Market]Schedule
	RetryDelay time.Duration // initial retry backoff; zero defaults to five seconds
	// OnProgress receives serialized per-code completion events. Nil means no terminal output.
	// The callback must return promptly and must not synchronously reenter Service.Update.
	OnProgress func(Progress) `json:"-"`
	// OnEvent receives serialized market and stage results, independently of OnProgress.
	// It must return promptly and must not synchronously reenter an update method.
	OnEvent func(Event) `json:"-"`
}

// Progress reports completed securities, including failures after retry exhaustion.
type Progress struct {
	Market           string
	Code             model.Code
	Completed, Total int
	Err              error
}

func isNil(v any) bool {
	if v == nil {
		return true
	}
	switch value := reflect.ValueOf(v); value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	}
	return false
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
