package market

import "testing"

func TestTradingSessionClock(t *testing.T) {
	for _, tc := range []struct {
		name    string
		session TradingSession
		index   int
		want    string
	}{
		{"domestic first", DomesticSession, 0, "09:31"},
		{"domestic last morning", DomesticSession, 119, "11:30"},
		{"domestic first afternoon", DomesticSession, 120, "13:01"},
		{"domestic last", DomesticSession, 239, "15:00"},
		{"continuous 121st", TradingSession{Open: 570}, 120, "11:31"},
		{"continuous first", TradingSession{Open: 570}, 0, "09:31"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.session.HourMinute(tc.index)
			if err != nil || got != tc.want {
				t.Fatal(got, tc.want, err)
			}
		})
	}
}

func TestTradingSessionCapabilities(t *testing.T) {
	if !DomesticSession.Valid() || !DomesticSession.HasLunchBreak() {
		t.Fatal("domestic session should be valid with a lunch break")
	}
	if !DomesticSession.AuctionMinute() || !DomesticSession.LunchTimeFix() {
		t.Fatal("domestic session should support the 241 layout and lunch time fix")
	}
	continuous := TradingSession{Open: 570}
	if !continuous.Valid() || continuous.HasLunchBreak() {
		t.Fatal("continuous session should be valid without a lunch break")
	}
	if continuous.AuctionMinute() || continuous.LunchTimeFix() {
		t.Fatal("continuous session must not claim domestic-only layouts")
	}
	if _, err := (TradingSession{}).HourMinute(0); err == nil {
		t.Fatal("zero session accepted")
	}
	if _, err := DomesticSession.HourMinute(-1); err == nil {
		t.Fatal("negative index accepted")
	}
	if _, err := DomesticSession.HourMinute(24 * 60); err == nil {
		t.Fatal("clock beyond day accepted")
	}
}

// TestRouteSessionAndTradeFallback 固定路由时段与“分笔补线”能力的独立声明。
func TestRouteSessionAndTradeFallback(t *testing.T) {
	for _, ex := range []Exchange{ExchangeSH, ExchangeSZ, ExchangeBJ} {
		route, ok := ex.Route()
		if !ok || route.Session != DomesticSession {
			t.Fatalf("%s route session: %+v", ex, route.Session)
		}
	}
	route, _ := ExchangeHK.Route()
	if route.Session.Valid() {
		t.Fatal("扩展协议路由不应声明按序号推导的时段")
	}
	for _, tc := range []struct {
		code     string
		fallback bool
	}{
		{"sh600000", true},
		{"sz000001", true},
		{"bj920001", true},
		{"sh510300", true},
		{"sh000001", false},
		{"sh110075", false},
		{"sh880001", false},
		{"00700.31", false},
	} {
		i, err := ParseInstrument(tc.code)
		if err != nil {
			t.Fatal(err)
		}
		rules, ok := i.Rules()
		if !ok || rules.TradeFallback != tc.fallback {
			t.Fatalf("%s fallback: %v (want %v)", tc.code, rules.TradeFallback, tc.fallback)
		}
	}
}
