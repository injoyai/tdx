package pull

import (
	"github.com/injoyai/tdx/v2/protocol"
	"testing"
)

type discoveryTestUnit struct {
	mockUnit
	list []Code
}

func (u *discoveryTestUnit) Codes(*Service) ([]Code, error) { return u.list, nil }

func TestConfiguredCodesDeduplicateTasksAndIdentity(t *testing.T) {
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	cfg := Config{Dir: t.TempDir(), Codes: []string{"test.one", " test.one "}, Goroutines: 1}
	s, err := New(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	u := &scopeTestUnit{mockUnit: mockUnit{name: "test"}}
	s.units = []Unit{u}
	if err := s.updateUnit(u); err != nil {
		t.Fatal(err)
	}
	if len(u.days) != 1 || len(u.minutes) != 1 {
		t.Fatal("重复代码执行了多次", u.days, u.minutes)
	}
	cfg.Codes = []string{"test.one"}
	single, err := New(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer single.Close()
	if s.updateKey("pull") != single.updateKey("pull") {
		t.Fatal("重复代码改变了任务身份")
	}
}

func TestDiscoveredCodesDeduplicateByMarketAndCode(t *testing.T) {
	s := testService(t)
	first, err := ParseCode("600000")
	if err != nil {
		t.Fatal(err)
	}
	alias, err := ParseCode("sh600000")
	if err != nil {
		t.Fatal(err)
	}
	first.Name, alias.Name = "first", "other name"
	u := &discoveryTestUnit{mockUnit: mockUnit{name: "discovery"}, list: []Code{
		first, alias, {Market: protocol.MarketUS, Code: first.Code},
	}}
	codes, err := s.codes(u)
	if err != nil || len(codes) != 2 || codes[0] != first || codes[1].Market != protocol.MarketUS {
		t.Fatal("重复代码未去重、名称丢失或不同市场被合并", codes, err)
	}
	if u.list[1] != alias {
		t.Fatal("修改了 Unit 返回的列表")
	}
}

func TestUSCodeCaseDedup(t *testing.T) {
	var codes []Code
	for _, input := range []string{"aapl", "AAPL", "usaapl", "us.AAPL", "us/stock.aapl"} {
		code, err := ParseCode(input)
		if err != nil || code.Code != "AAPL" || code.Market != protocol.MarketUS {
			t.Fatal(input, code, err)
		}
		codes = append(codes, code)
	}
	if got := uniqueCodes(codes); len(got) != 1 {
		t.Fatal("same security scheduled more than once", got)
	}
}
