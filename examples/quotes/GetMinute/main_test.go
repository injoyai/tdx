package main

import (
	"errors"
	"testing"

	"github.com/injoyai/tdx/v2"
)

type fakeClient struct {
	tdx.Client
	t      *testing.T
	date   string
	called bool
	err    error
}

func (c *fakeClient) GetMinute(code string) (*tdx.MinuteResp, error) {
	if c.date != "" || code != "sz000001" {
		c.t.Fatal("当日分时请求参数错误")
	}
	c.called = true
	return nil, c.err
}
func (c *fakeClient) GetHistoryMinute(date, code string) (*tdx.MinuteResp, error) {
	if date != c.date || code != "sz000001" {
		c.t.Fatal("历史分时请求参数错误")
	}
	c.called = true
	return nil, c.err
}
func TestQuery(t *testing.T) {
	for _, date := range []string{"", "20261012"} {
		want := errors.New("source error")
		c := &fakeClient{t: t, date: date, err: want}
		_, err := query(c, "sz000001", date)
		if !c.called || !errors.Is(err, want) {
			t.Fatalf("调用缺失或源错误丢失: %v", err)
		}
	}
}
