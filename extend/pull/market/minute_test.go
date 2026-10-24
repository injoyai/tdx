package market

import (
	"errors"
	"fmt"
	"github.com/injoyai/tdx/v2/extend/gbbq"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/manage"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

type plainMinuteClient struct {
	tdx.Client
	category uint8
}

func (c *plainMinuteClient) GetKlineUntil(category uint8, _ string, _ func(tdx.Kline) bool) (*tdx.KlineResp, error) {
	c.category = category
	return &tdx.KlineResp{List: []tdx.Kline{{Standard: &protocol.Kline{Volume: 100}}}}, nil
}

func TestBondMinuteUsesPlainPaging(t *testing.T) {
	for _, market := range []protocol.Market{protocol.MarketBond, protocol.MarketOther} {
		if _, ok := pull.Get(market.String()); !ok {
			t.Fatal("new domestic unit not registered", market)
		}
		c := &plainMinuteClient{}
		u := stdUnit{name: market}
		rows, err := u.minUntil(c, "sh110075", func(*protocol.Kline) bool { return false })
		if err != nil || len(rows) != 1 || c.category != protocol.TypeKlineMinute {
			t.Fatal(market, rows, c.category, err)
		}
	}
}

func TestPullMinutesOneScanAndRecovery(t *testing.T) {
	s, err := pull.New(&pull.Config{Dir: t.TempDir(), StartAt: "20240101", Codes: []string{"sz000001"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	code := pull.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	now := time.Date(2026, 9, 4, 16, 0, 0, 0, time.Local)
	var data []*pull.KlineMinute
	for y := 2024; y <= 2026; y++ {
		data = append(data, &pull.KlineMinute{Time: time.Date(y, 6, 1, 9, 31, 0, 0, time.Local).Format("2006-01-02 15:04"), Close: float64(y)})
	}
	// 旧历史库虽有数据，但没有完成标记，应重扫而不是从尾部继续。
	if err := s.SaveMinute(code, 2024, "", data[:1]); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("page failed")
	if err := pullMinutes(s, code, now, func(string) ([]*pull.KlineMinute, error) { return data, failure }, nil); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if done, err := s.MinuteYearComplete(code, 2024, s.Start().Format("2006-01-02 15:04")); done || err != nil {
		t.Fatal("失败标记完成", done, err)
	}
	calls := 0
	fetch := func(from string) ([]*pull.KlineMinute, error) {
		calls++
		if from != s.Start().Format(protocol.TimeLayout) {
			t.Errorf("首次边界=%v", from)
		}
		return data, nil
	}
	if err := pullMinutes(s, code, now, fetch, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("按年份重复扫描", calls)
	}
	for y := 2024; y < 2026; y++ {
		from := time.Date(y, 1, 1, 0, 0, 0, 0, time.Local).Format("2006-01-02 15:04")
		if done, err := s.MinuteYearComplete(code, y, from); !done || err != nil {
			t.Fatal("未完成", y, done, err)
		}
	}
	rows, err := s.QueryMinute(code, "", "")
	if err != nil || len(rows) != 3 {
		t.Fatal("分年落库失败", len(rows), err)
	}
	if err := pullMinutes(s, code, now, func(from string) ([]*pull.KlineMinute, error) {
		if from != data[2].Time {
			t.Errorf("增量边界=%v", from)
		}
		return []*pull.KlineMinute{{Time: data[2].Time, Close: 99}}, nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	rows, err = s.QueryMinute(code, "", "")
	if err != nil || len(rows) != 3 || rows[2].Close != 99 {
		t.Fatal("边界未覆盖", rows, err)
	}
}

func TestPullMinutesEmptyAndFailed(t *testing.T) {
	s, err := pull.New(&pull.Config{Dir: t.TempDir(), StartAt: "20240101", Codes: []string{"sz000001"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := pull.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.Local)
	for i := 0; i < 2; i++ {
		if err := pullMinutes(s, c, now, func(from string) ([]*pull.KlineMinute, error) {
			if from != s.Start().Format(protocol.TimeLayout) {
				t.Fatal("空结果被记为已完成")
			}
			return nil, nil
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if s.MinuteExists(c, 2024) || s.MinuteExists(c, 2026) {
		t.Fatal("空结果创建文件")
	}
	failed := errors.New("fetch failed")
	err = pullMinutes(s, c, now, func(string) ([]*pull.KlineMinute, error) {
		return []*pull.KlineMinute{{Time: now.Format("2006-01-02 15:04")}}, failed
	}, nil)
	if !errors.Is(err, failed) || s.MinuteExists(c, 2026) {
		t.Fatal("失败后仍写库", err)
	}
}

func TestPullMinutesRetainsExpiredLocalBar(t *testing.T) {
	s, err := pull.New(&pull.Config{Dir: t.TempDir(), StartAt: "20260101", Codes: []string{"sz000001"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := pull.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	old := &pull.KlineMinute{Time: time.Date(2026, 3, 2, 15, 0, 0, 0, time.Local).Format("2006-01-02 15:04"), Close: 10}
	if err := s.SaveMinute(c, 2026, "", []*pull.KlineMinute{old}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 4, 16, 0, 0, 0, time.Local)
	newer := &pull.KlineMinute{Time: now.Add(-time.Hour - time.Minute).Format("2006-01-02 15:04"), Close: 11}
	newest := &pull.KlineMinute{Time: now.Add(-time.Hour).Format("2006-01-02 15:04"), Close: 12}
	if err := pullMinutes(s, c, now, func(from string) ([]*pull.KlineMinute, error) {
		if from != old.Time {
			t.Fatalf("unexpected incremental boundary %v", from)
		}
		// 服务器已不保留旧记录，且返回顺序是从新到旧。
		return []*pull.KlineMinute{newest, newer}, nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	rows, err := s.QueryMinute(c, "", "")
	if err != nil || len(rows) != 3 || rows[0].Time != old.Time || rows[1].Time != newer.Time {
		t.Fatalf("local history lost: rows=%v, err=%v", rows, err)
	}
	if err := pullMinutes(s, c, now, func(string) ([]*pull.KlineMinute, error) {
		return []*pull.KlineMinute{{Time: newest.Time, Close: 99}}, nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	rows, err = s.QueryMinute(c, "", "")
	if err != nil || len(rows) != 3 || rows[2].Close != 99 {
		t.Fatal("incremental boundary was not refreshed", rows, err)
	}
}

type pagingClient struct {
	tdx.Client
	rows  []tdx.Kline
	err   error
	calls int
}

func TestPullMinutesKeepsReturnedSourceYear(t *testing.T) {
	s, err := pull.New(&pull.Config{Dir: t.TempDir(), StartAt: "20261231", Codes: []string{"usAAPL"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	code := pull.Code{Market: protocol.MarketUS, Code: "AAPL"}
	now := time.Date(2026, 12, 31, 23, 0, 0, 0, protocol.Location())
	row := &pull.KlineMinute{Time: "2027-01-01 00:01", Close: 10}
	if err := pullMinutes(s, code, now, func(string) ([]*pull.KlineMinute, error) { return []*pull.KlineMinute{row}, nil }, nil); err != nil {
		t.Fatal(err)
	}
	rows, err := s.QueryMinute(code, "", "")
	if err != nil || len(rows) != 1 || rows[0].Time != row.Time || !s.MinuteExists(code, 2027) || rows[0].Unix != nil {
		t.Fatal("host date displaced source year", rows, err)
	}
}

func (*pagingClient) Close() error { return nil }
func (c *pagingClient) GetKlineUntil(category uint8, code string, stop func(tdx.Kline) bool) (*tdx.KlineResp, error) {
	c.calls++
	if category != protocol.TypeKlineDay || code != "us.AAPL" {
		return nil, fmt.Errorf("wrong paging request %d %s", category, code)
	}
	if len(c.rows) > 1 && (stop(c.rows[1]) != true || stop(c.rows[2]) != false) {
		return nil, errors.New("wrong boundary predicate")
	}
	return &tdx.KlineResp{List: c.rows}, c.err
}

func TestExtendedBarsUsesClientPaging(t *testing.T) {
	base := time.Date(2026, 1, 2, 15, 0, 0, 0, protocol.Location())
	c := &pagingClient{}
	for _, at := range []time.Time{base.AddDate(0, 0, -1), base, base.AddDate(0, 0, 1)} {
		k := &protocol.ExKline{Datetime: at.Format(time.DateOnly), Close: 10}
		c.rows = append(c.rows, tdx.Kline{Time: k.Datetime, Extended: k})
	}
	p, err := tdx.NewPool(func() (tdx.Client, error) { return c, nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	s, err := pull.New(&pull.Config{Dir: t.TempDir(), Codes: []string{"usAAPL"}, ExPool: p})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	u := &exUnit{name: protocol.MarketUS, markets: []uint8{marketUS}}
	rows, err := u.bars(s, pull.Code{Market: protocol.MarketUS, Code: "AAPL"}, protocol.TypeKlineDay, base.Format(time.DateOnly))
	if err != nil || len(rows) != 2 || rows[0].Datetime != c.rows[1].Time || c.calls != 1 {
		t.Fatal(rows, c.calls, err)
	}
	c.err = errors.New("page failed")
	if _, err = u.bars(s, pull.Code{Market: protocol.MarketUS, Code: "AAPL"}, protocol.TypeKlineDay, base.Format(time.DateOnly)); !errors.Is(err, c.err) {
		t.Fatal("lost paging error", err)
	}
	c.err = nil
	c.rows = []tdx.Kline{{Time: "invalid"}}
	if _, err = u.bars(s, pull.Code{Market: protocol.MarketUS, Code: "AAPL"}, protocol.TypeKlineDay, base.Format(time.DateOnly)); err == nil {
		t.Fatal("accepted invalid source time")
	}
}

func TestManageDoesNotInitializeGbbq(t *testing.T) {
	g := &gbbq.Gbbq{}
	m := &manage.Manage{Gbbq: g}
	s, err := pull.New(&pull.Config{Dir: t.TempDir(), Codes: []string{"sh000001"}, Manage: m})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, name := range []protocol.Market{protocol.MarketAStock, protocol.MarketIndex, protocol.MarketEtfLof} {
		u := &stdUnit{name: name}
		if _, err := u.Manage(s); err != nil {
			t.Fatal(err)
		}
		if m.Gbbq != g || !g.IsEmpty() {
			t.Fatal("取连接源触发了股本初始化")
		}
	}
}

func TestPullMinutesEmptyHistoryCacheAndRecheck(t *testing.T) {
	cfg := pull.Config{Dir: t.TempDir(), StartAt: "20240101", Codes: []string{"sz000001"}}
	s, err := pull.New(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	code := pull.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	now := time.Date(2026, 9, 4, 16, 0, 0, 0, time.Local)
	row := &pull.KlineMinute{Time: now.Add(-time.Hour).Format("2006-01-02 15:04"), Close: 10}
	failed := errors.New("scan failed")
	if err := pullMinutes(s, code, now, func(string) ([]*pull.KlineMinute, error) {
		return []*pull.KlineMinute{row}, failed
	}, nil); !errors.Is(err, failed) {
		t.Fatal(err)
	}
	if empty, err := s.MinuteYearRecentlyEmpty(code, 2024, s.Start().Format("2006-01-02 15:04"), now); err != nil || empty {
		t.Fatal("请求失败被缓存为空历史", empty, err)
	}
	if err := pullMinutes(s, code, now, func(from string) ([]*pull.KlineMinute, error) {
		if from != s.Start().Format(protocol.TimeLayout) {
			t.Fatal("首次未扫描完整范围", from)
		}
		return []*pull.KlineMinute{row}, nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	if s.MinuteExists(code, 2024) || s.MinuteExists(code, 2025) {
		t.Fatal("空历史创建了行情库")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = pull.New(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := pullMinutes(s, code, now.AddDate(0, 0, 1), func(from string) ([]*pull.KlineMinute, error) {
		if from != row.Time {
			t.Fatal("重启后仍重复扫描空年份", from)
		}
		return []*pull.KlineMinute{{Time: row.Time, Close: 99}}, nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	old := &pull.KlineMinute{Time: s.Start().Add(9*time.Hour + 31*time.Minute).Format("2006-01-02 15:04"), Close: 11}
	if err := pullMinutes(s, code, now.AddDate(0, 0, 7), func(from string) ([]*pull.KlineMinute, error) {
		if from != s.Start().Format(protocol.TimeLayout) {
			t.Fatal("到期未复查历史", from)
		}
		return []*pull.KlineMinute{row, old}, nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	rows, err := s.QueryMinute(code, "", "")
	if err != nil || len(rows) != 2 || rows[0].Time != old.Time {
		t.Fatal("重新出现的历史数据未补入", rows, err)
	}
}
