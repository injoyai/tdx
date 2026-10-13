package main

import (
	"errors"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

type fakeClient struct {
	tdx.Client
	t      *testing.T
	method string
	before time.Time
	err    error
}

func (c *fakeClient) check(method string, category uint8, code string) {
	c.t.Helper()
	if method != c.method || category != protocol.TypeKlineYear || code != "sh000001" {
		c.t.Fatalf("错误调用: %s %d %s", method, category, code)
	}
}
func (c *fakeClient) GetKline(category uint8, code string, start, count uint16) (*tdx.KlineResp, error) {
	c.check("page", category, code)
	if start != 7 || count != 20 {
		c.t.Fatalf("分页参数改变: %d %d", start, count)
	}
	return nil, c.err
}
func (c *fakeClient) GetKlineAll(category uint8, code string) (*tdx.KlineResp, error) {
	c.check("all", category, code)
	return nil, c.err
}
func (c *fakeClient) GetKlineUntil(category uint8, code string, stop func(tdx.Kline) bool) (*tdx.KlineResp, error) {
	c.check("until", category, code)
	at := c.before.Format("2006-01-02 15:04")
	earlier := c.before.Add(-time.Minute).Format("2006-01-02 15:04")
	if stop(tdx.Kline{Time: at}) || !stop(tdx.Kline{Time: earlier}) {
		c.t.Fatal("停止条件应只在边界之前命中")
	}
	return nil, c.err
}

func TestQuery(t *testing.T) {
	for _, method := range []string{"page", "all", "until"} {
		t.Run(method, func(t *testing.T) {
			want := errors.New("source error")
			c := &fakeClient{t: t, method: method, err: want}
			if method == "until" {
				c.before = time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local)
			}
			_, err := query(c, protocol.TypeKlineYear, "sh000001", 7, 20, method == "all", c.before)
			if !errors.Is(err, want) {
				t.Fatalf("源错误丢失: %v", err)
			}
		})
	}
}
