package pull

import (
	"sync"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

// 临时服务：构造最小 Service 用于存储测试。
var registerMockOnce sync.Once

func testService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	// 测试环境未注册内置市场，注册一个仅供测试的市场（多个测试共用，只注册一次）；
	// 用完整键 "test.xxx" 通过 ParseCode 路由到该市场
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	s, err := New(&Config{
		Store:      testStore(dir),
		Codes:      []string{"test.sh600000"},
		RetryDelay: time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// mockUnit 仅用于满足 New 的 Unit 非空校验。
type mockUnit struct{ name string }

func (*mockUnit) Schedule() Schedule {
	return Schedule{TimeZone: "Asia/Shanghai", CloseTime: "15:30", Delay: 5 * time.Minute}
}

func (u *mockUnit) Name() string { return u.name }
func (*mockUnit) Codes(s *Service) ([]model.Code, error) {
	return nil, nil
}
func (*mockUnit) FetchDay(s *Service, code model.Code) error {
	return nil
}
func (*mockUnit) FetchMinute(s *Service, code model.Code) error {
	return nil
}

func TestParseCode(t *testing.T) {
	cases := []struct {
		in     string
		market protocol.Market
		code   string
	}{
		// 沪深：带前缀
		{"sh600000", protocol.MarketAStock, "sh600000"},
		{"sz000001", protocol.MarketAStock, "sz000001"},
		{"sz300750", protocol.MarketAStock, "sz300750"},
		{"bj920001", protocol.MarketAStock, "bj920001"},
		{"SH600000", protocol.MarketAStock, "sh600000"}, // 大小写归一
		// 沪深：6 位裸数字（自动补前缀）
		{"600000", protocol.MarketAStock, "sh600000"},
		{"000001", protocol.MarketAStock, "sz000001"},
		{"300750", protocol.MarketAStock, "sz300750"},
		{"920001", protocol.MarketAStock, "bj920001"},
		// ETF/LOF
		{"sh510300", protocol.MarketEtfLof, "sh510300"},
		{"sz159915", protocol.MarketEtfLof, "sz159915"},
		{"510300", protocol.MarketEtfLof, "sh510300"},
		// 指数/板块
		{"sh000001", protocol.MarketIndex, "sh000001"},
		{"sz399001", protocol.MarketIndex, "sz399001"},
		{"399001", protocol.MarketIndex, "sz399001"},
		{"sh880001", protocol.MarketBlock, "sh880001"},
		{"880001", protocol.MarketBlock, "sh880001"},
		{"899050", protocol.MarketIndex, "bj899050"}, // 北交所指数，补不上交易所前缀，特判
		// 港股
		{"HK00700", protocol.MarketHK, "00700"},
		{"00700", protocol.MarketHK, "00700"},
		// 港股指数（白名单）
		{"HSI", protocol.MarketHKIndex, "HSI"},
		{"VHSI", protocol.MarketHKIndex, "VHSI"},
		// 美股
		{"AAPL", protocol.MarketUS, "AAPL"},
		// 期货
		{"cff/IF2609", protocol.MarketFuture, "cff/IF2609"},
		{"shf/cu2609", protocol.MarketFuture, "shf/cu2609"},
		// 完整键
		{"cn/future.cff/IF2609", protocol.MarketFuture, "cff/IF2609"},
		{"hk/stock.00700", protocol.MarketHK, "00700"},
		{"US.AAPL", protocol.MarketUS, "AAPL"},
		{"hk/index.HSI", protocol.MarketHKIndex, "HSI"},
		{"hk/index.CES120", protocol.MarketHKIndex, "CES120"},
	}
	for _, c := range cases {
		got, err := ParseCode(c.in)
		if err != nil {
			t.Errorf("ParseCode(%q) err: %v", c.in, err)
			continue
		}
		if got.Market != c.market || got.Code != c.code {
			t.Errorf("ParseCode(%q) = {%s, %s}, want {%s, %s}", c.in, got.Market, got.Code, c.market, c.code)
		}
	}
	// 无法识别
	for _, s := range []string{"", "1A0001", "abcdefg1", "XX600000", "12345678901"} {
		if _, err := ParseCode(s); err == nil {
			t.Errorf("ParseCode(%q) 应返回错误", s)
		}
	}
}
