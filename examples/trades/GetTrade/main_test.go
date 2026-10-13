package main

import (
	"errors"
	"testing"

	"github.com/injoyai/tdx/v2"
)

type fakeClient struct {
	tdx.Client
	t      *testing.T
	method string
	called bool
	err    error
}

func (c *fakeClient) check(method, code, date string) {
	c.t.Helper()
	if method != c.method || code != "sz000001" || (date != "" && date != "20261012") {
		c.t.Fatalf("错误调用: %s %s %s", method, code, date)
	}
	c.called = true
}
func (c *fakeClient) GetTrade(code string, start, count uint16) (*tdx.TradeResp, error) {
	c.check("page", code, "")
	if start != 7 || count != 20 {
		c.t.Fatal("分页参数改变")
	}
	return nil, c.err
}
func (c *fakeClient) GetTradeAll(code string) (*tdx.TradeResp, error) {
	c.check("all", code, "")
	return nil, c.err
}
func (c *fakeClient) GetHistoryTrade(date, code string, start, count uint16) (*tdx.TradeResp, error) {
	c.check("history", code, date)
	if start != 7 || count != 20 {
		c.t.Fatal("历史分页参数改变")
	}
	return nil, c.err
}
func (c *fakeClient) GetHistoryTradeDay(date, code string) (*tdx.TradeResp, error) {
	c.check("history-all", code, date)
	return nil, c.err
}
func TestQuery(t *testing.T) {
	for _, tc := range []struct {
		method, date string
		all          bool
	}{
		{"page", "", false}, {"all", "", true},
		{"history", "20261012", false}, {"history-all", "20261012", true},
	} {
		t.Run(tc.method, func(t *testing.T) {
			want := errors.New("source error")
			c := &fakeClient{t: t, method: tc.method, err: want}
			_, err := query(c, "sz000001", tc.date, 7, 20, tc.all)
			if !c.called || !errors.Is(err, want) {
				t.Fatalf("调用缺失或源错误丢失: %v", err)
			}
		})
	}
}
