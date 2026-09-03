package pull

import (
	"context"
	"fmt"
	"sync"
)

// Unit 代表一个可拉取的独立市场/品种类别（可插拔）。
//
// 新增一个市场 = 新增一个实现 + Register，无需修改框架代码。
type Unit interface {
	// Name 唯一标识（Market 枚举值，如 MarketAStock、MarketHK、MarketFuture），用于日志/进度条/去重。
	Name() string

	// Codes 返回该市场需要拉取的代码列表（动态获取，可被 Config.Codes 覆盖）。
	// s 提供连接源（Manage 标准行情 / ExPool 扩展行情）访问。
	Codes(ctx context.Context, s *Service) ([]Code, error)

	// FetchDay 拉取并存储某代码的日线（增量逻辑由框架统一处理）。
	FetchDay(ctx context.Context, s *Service, code Code) error

	// FetchMin 拉取并存储某代码的1分钟线（增量逻辑由框架统一处理）。
	FetchMin(ctx context.Context, s *Service, code Code) error
}

// registry 市场注册表（可插拔核心）。
var registry struct {
	sync.RWMutex
	units map[string]Unit
}

func init() {
	registry.units = map[string]Unit{}
}

// Register 注册一个市场 Unit；重名 panic。
func Register(u Unit) {
	if u == nil {
		panic("pull: 注册的市场 Unit 不能为 nil")
	}
	name := u.Name()
	if name == "" {
		panic("pull: 注册的市场 Unit 缺少 Name")
	}
	registry.Lock()
	defer registry.Unlock()
	if _, ok := registry.units[name]; ok {
		panic(fmt.Sprintf("pull: 市场 Unit %q 重复注册", name))
	}
	registry.units[name] = u
}

// Get 按名称获取已注册的市场 Unit。
func Get(name string) (Unit, bool) {
	registry.RLock()
	defer registry.RUnlock()
	u, ok := registry.units[name]
	return u, ok
}

// Units 返回全部已注册的市场 Unit。
func Units() []Unit {
	registry.RLock()
	defer registry.RUnlock()
	out := make([]Unit, 0, len(registry.units))
	for _, u := range registry.units {
		out = append(out, u)
	}
	return out
}
