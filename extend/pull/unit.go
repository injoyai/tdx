package pull

import (
	"fmt"
	"strings"
	"sync"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

// Unit 代表一个可拉取的独立市场（可插拔）。
//
// 内置市场由 market 包按协议定义注册。自定义市场需实现并 Register，且须通过 Config.Schedules
// 或可选的 Schedule() Schedule 方法声明更新边界。
type Unit interface {
	// Name 唯一标识（protocol.MarketID 枚举值，如 protocol.MarketSH、protocol.MarketHK、protocol.MarketCFF），用于日志/进度条/去重。
	Name() string

	// Codes 返回该市场需要拉取的代码列表（动态获取，可被 Config.Codes 覆盖）。
	// s 提供连接池（Pool 标准行情 / ExPool 扩展行情）和已持久化代码快照／代码列表函数访问。
	Codes(s *Service) ([]model.Code, error)

	// FetchDay 拉取并存储某代码日线；实现需读取最后源日期、控制增量边界并调用 SaveDay。
	FetchDay(s *Service, code model.Code) error

	// FetchMinute 拉取并存储某代码1分钟线；实现需管理分页、年度覆盖状态并调用存储方法。
	FetchMinute(s *Service, code model.Code) error
}

// registry 市场注册表（可插拔核心）。
var registry struct {
	sync.RWMutex
	units map[string]Unit
}

func init() {
	registry.units = map[string]Unit{}
}

// Register 注册一个市场 Unit；忽略大小写后重名 panic。
func Register(u Unit) { register(u, false) }

// RegisterMarket installs the pull adapter for an exact built-in market key.
func RegisterMarket(u Unit) { register(u, true) }

func register(u Unit, builtin bool) {
	if u == nil {
		panic("pull: 注册的市场 Unit 不能为 nil")
	}
	name := u.Name()
	if name == "" {
		panic("pull: 注册的市场 Unit 缺少 Name")
	}
	if builtin {
		if _, ok := protocol.MarketID(name).Exchange(); !ok {
			panic(fmt.Sprintf("pull: 未定义的内置市场 %q", name))
		}
	} else {
		for _, m := range protocol.Markets() {
			ex, _ := m.Exchange()
			if strings.EqualFold(name, m.String()) || strings.EqualFold(name, ex.String()) {
				panic(fmt.Sprintf("pull: 市场 Unit %q 占用内置市场名称", name))
			}
		}
	}
	registry.Lock()
	defer registry.Unlock()
	for registered := range registry.units {
		if strings.EqualFold(registered, name) {
			panic(fmt.Sprintf("pull: 市场 Unit %q 重复注册", name))
		}
	}
	registry.units[name] = u
}

// Get 按名称获取已注册的市场 Unit，不区分大小写。
func Get(name string) (Unit, bool) {
	registry.RLock()
	defer registry.RUnlock()
	u, ok := registry.units[name]
	if ok {
		return u, true
	}
	for registered, unit := range registry.units {
		if strings.EqualFold(registered, name) {
			return unit, true
		}
	}
	return nil, false
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
