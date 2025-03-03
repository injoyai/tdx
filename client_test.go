package tdx

import (
	"github.com/injoyai/logs"
	"testing"
)

var (
	c  *Client
	do func(f func(c *Client))
)

func init() {
	var err error
	c, err = Dial("124.71.187.122:7709")
	logs.PanicErr(err)
	do = func(f func(c *Client)) {
		f(c)
		<-c.Done()
	}
}

func TestClient_GetStockHistoryMinuteTrade(t *testing.T) {
	do(func(c *Client) {
		resp, err := c.GetHistoryMinuteTrade("20241028", "sz000001", 0, 100)
		if err != nil {
			t.Error(err)
			return
		}
		for _, v := range resp.List {
			t.Log(v)
		}
		t.Log("总数：", resp.Count)
	})

}
