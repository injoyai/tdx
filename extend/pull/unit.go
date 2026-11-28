package pull

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

// Unit 代表一个可拉取的独立市场。协议市场自动选用标准或扩展实现。
// 自定义拉取流程使用 Register，并通过 Config.Schedules 或 Schedule() Schedule 声明更新边界。
// 可选 Identity() string 用稳定的非空标识绑定持久数据定义。
type Unit interface {
	// Name 是市场的唯一标识，用于存储、日志和去重。
	Name() string

	// Codes 返回市场的代码列表（动态获取，可被 Config.Codes 覆盖）。
	Codes(s *Service) ([]model.Code, error)

	// FetchDay 拉取并存储某代码日线；实现需控制增量边界并调用 SaveDay。
	FetchDay(s *Service, code model.Code) error

	// FetchMinute 拉取并存储某代码1分钟线；实现需管理分页和年度覆盖状态。
	FetchMinute(s *Service, code model.Code) error
}

var registry = struct {
	sync.RWMutex
	units map[string]Unit
}{units: make(map[string]Unit)}

// Register 注册自定义 Unit。名称与协议路由、市场别名或其他 Unit 冲突时 panic。
// 在应用启动时先完成 protocol.Register，再注册自定义 Unit。
func Register(u Unit) {
	if isNil(u) {
		panic("pull: 注册的市场 Unit 不能为 nil")
	}
	name := u.Name()
	if name == "" || name != strings.TrimSpace(name) {
		panic("pull: 注册的市场 Unit 的 Name 无效")
	}
	if protocolName(name) {
		panic(fmt.Sprintf("pull: 市场 Unit %q 占用协议市场名称", name))
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

func protocolName(name string) bool {
	if _, ok := protocol.MarketID(strings.ToLower(name)).Exchange(); ok {
		return true
	}
	ex, err := protocol.ParseExchange(name)
	if err != nil {
		return false
	}
	_, ok := ex.Route()
	return ok
}

func protocolUnit(id protocol.MarketID) (Unit, bool) {
	ex, ok := id.Exchange()
	if !ok {
		return nil, false
	}
	route, _ := ex.Route()
	switch route.Protocol {
	case protocol.ProtocolStandard:
		return &stdUnit{name: id}, true
	case protocol.ProtocolExtended:
		return &exUnit{name: id}, true
	default:
		return nil, false
	}
}

// Get 按路由键或自定义 Unit 名称查询，不区分大小写。
func Get(name string) (Unit, bool) {
	if u, ok := protocolUnit(protocol.MarketID(strings.ToLower(name))); ok {
		return u, true
	}
	if protocolName(name) {
		return nil, false
	}
	registry.RLock()
	defer registry.RUnlock()
	for registered, u := range registry.units {
		if strings.EqualFold(registered, name) {
			return u, true
		}
	}
	return nil, false
}

func customUnits() []Unit {
	registry.RLock()
	defer registry.RUnlock()
	out := make([]Unit, 0, len(registry.units))
	for name, u := range registry.units {
		if !protocolName(name) {
			out = append(out, u)
		}
	}
	return out
}

// Units 返回协议市场的每条路由以及已注册的自定义 Unit。
func Units() []Unit {
	ids := protocol.Markets()
	out := customUnits()
	for _, id := range ids {
		if u, ok := protocolUnit(id); ok {
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
