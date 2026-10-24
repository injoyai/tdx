package main

import (
	"errors"
	"testing"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

type fakeClient struct {
	tdx.Client
	t      *testing.T
	all    bool
	called bool
	err    error
}

func (c *fakeClient) GetCode(exchange protocol.Exchange, start uint32, count uint16) (*tdx.CodeResp, error) {
	if c.all || exchange != protocol.ExchangeBJ || start != 7 || count != 20 {
		c.t.Fatal("分页请求或市场参数错误")
	}
	c.called = true
	return nil, c.err
}
func (c *fakeClient) GetCodeAll(exchange protocol.Exchange) (*tdx.CodeResp, error) {
	if !c.all || exchange != protocol.ExchangeBJ {
		c.t.Fatal("全量请求或市场参数错误")
	}
	c.called = true
	return nil, c.err
}
func TestQuery(t *testing.T) {
	for _, all := range []bool{false, true} {
		want := errors.New("source error")
		c := &fakeClient{t: t, all: all, err: want}
		_, err := query(c, protocol.ExchangeBJ, 7, 20, all)
		if !c.called || !errors.Is(err, want) {
			t.Fatalf("调用缺失或错误未传递: %v", err)
		}
	}
}
