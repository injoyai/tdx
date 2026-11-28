package pull

import (
	"context"
	"errors"
	"testing"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/storage/sqlite"
	"github.com/injoyai/tdx/v2/protocol"
)

type zzqPullClient struct {
	tdx.Client
	days, minutes, trades int
}

func (c *zzqPullClient) GetKlineMinute241Until(string, func(tdx.Kline) bool) (*tdx.KlineResp, error) {
	c.minutes++
	return &tdx.KlineResp{List: []tdx.Kline{
		{Time: "2026-11-20 09:30", Volume: 10},
		{Time: "2026-11-20 09:32", Volume: 10},
	}}, nil
}

func (c *zzqPullClient) GetKlineUntil(uint8, string, func(tdx.Kline) bool) (*tdx.KlineResp, error) {
	c.days++
	return &tdx.KlineResp{List: []tdx.Kline{{Time: "2026-11-20", Close: 10, Volume: 10}}}, nil
}

func (c *zzqPullClient) GetHistoryTradeDay(string, string) (*tdx.TradeResp, error) {
	c.trades++
	return nil, errors.New("只开竞价能力时不应请求分笔")
}

type fixedPool struct{ c tdx.Client }

func (p fixedPool) DoContext(ctx context.Context, f func(tdx.Client) error) error {
	return f(tdx.WithContext(ctx, p.c))
}

// TestZZQAuctionWithoutTradeFallback 验证拆竞价与分笔补线相互独立：
// 市场只声明 SplitAuction 时，即使显式开启 TradeFallback 也不会请求分笔。
func TestZZQAuctionWithoutTradeFallback(t *testing.T) {
	if _, ok := protocol.MarketID("zzq").Exchange(); !ok {
		t.Skip("zzq fixture 未挂载，跳过验收用例")
	}
	store, err := sqlite.NewPull(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	c := &zzqPullClient{}
	s, err := New(&Config{
		Store: store, Pool: fixedPool{c}, StartAt: "20260101", Minute: true, TradeFallback: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	u := &stdUnit{name: protocol.MarketID("zzq")}
	code := model.Code{Market: protocol.MarketID("zzq"), Code: "zzq123456"}

	if _, err := u.FetchMinuteResult(s, code); err != nil {
		t.Fatal(err)
	}
	if c.minutes == 0 {
		t.Fatal("未走拆竞价分钟入口")
	}
	if c.trades != 0 {
		t.Fatalf("只声明拆竞价时请求了 %d 次分笔", c.trades)
	}
	rows, err := s.QueryMinute(code, "", "")
	if err != nil || len(rows) != 2 {
		t.Fatal(rows, err)
	}
	for _, row := range rows {
		if row.Volume != 10 {
			t.Fatalf("SQLite 分钟量=%d, want 10", row.Volume)
		}
	}

	if _, err := u.FetchDayResult(s, code); err != nil {
		t.Fatal(err)
	}
	days, err := s.QueryDay(code, "", "")
	if err != nil || len(days) != 1 || days[0].Close != 10 || days[0].Volume != 10 {
		t.Fatalf("SQLite 日线=%+v, err=%v", days, err)
	}
}
