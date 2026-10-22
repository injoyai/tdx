package manage

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/gbbq"
	"github.com/injoyai/tdx/v2/extend/workday"
	"github.com/injoyai/tdx/v2/lib/xorms"
	"github.com/injoyai/tdx/v2/protocol"
)

// 使用公开接口构造离线资源，不依赖根包的连接或缓存内部字段。
type testClient struct {
	tdx.Client
	closed *atomic.Int32
}

func (c *testClient) Close() error { c.closed.Add(1); return nil }

func (*testClient) GetKlineDayAll(string) (*tdx.KlineResp, error) {
	k := &protocol.Kline{Time: tdx.IntegerDay(time.Now().In(protocol.Location())).Add(15 * time.Hour)}
	return &tdx.KlineResp{Count: 1, List: []tdx.Kline{{Standard: k}}}, nil
}

func (*testClient) GetGbbqAll() (map[string][]*protocol.Gbbq, error) {
	return map[string][]*protocol.Gbbq{}, nil
}

func poolTestClient(closed *atomic.Int32) *testClient { return &testClient{closed: closed} }

func testWorkday(t *testing.T, closed *atomic.Int32) *workday.Workday {
	t.Helper()
	w, err := workday.New(
		workday.WithDialClient(func() (tdx.Client, error) { return poolTestClient(closed), nil }),
		workday.WithDialDB(func() (*xorms.Engine, error) { return xorms.NewSQLite(":memory:") }),
		workday.WithSpec("@every 87600h"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	})
	return w
}

func testGbbq(t *testing.T, closed *atomic.Int32) *gbbq.Gbbq {
	t.Helper()
	g, err := gbbq.New(
		gbbq.WithDialClient(func() (tdx.Client, error) { return poolTestClient(closed), nil }),
		gbbq.WithDialDB(func() (*xorms.Engine, error) { return xorms.NewSQLite(":memory:") }),
		gbbq.WithSpec("@every 87600h"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := g.Close(); err != nil {
			t.Error(err)
		}
	})
	return g
}
