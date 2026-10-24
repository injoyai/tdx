package pull

import (
	"github.com/injoyai/tdx/v2/protocol"
	"testing"
	"time"
)

func TestMinuteFallbackSchemaAndSource(t *testing.T) {
	s := testService(t)
	c := Code{Market: protocol.MarketAStock, Code: "sz000001"}
	day := time.Date(2025, 1, 6, 0, 0, 0, 0, time.Local)
	if err := s.SaveMinuteComplete(c, 2025, day.Format("2006-01-02 15:04"), []*KlineMinute{{Time: day.Format("2006-01-02 15:04"), Open: 1, High: 1, Low: 1, Close: 1, Volume: 100, Amount: 100}}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.QueryMinute(c, day.Format("2006-01-02 15:04"), day.AddDate(0, 0, 1).Format("2006-01-02 15:04"))
	if err != nil || len(rows) != 1 || rows[0].Source != "" {
		t.Fatal("原生来源记录错误", rows, err)
	}
	coverage, err := s.QueryMinuteCoverage(c, 2025)
	if err != nil || coverage == nil || coverage.TradeFallback || coverage.Through != "" {
		t.Fatal("原生扫描误判兜底完成", coverage, err)
	}
	through := day.AddDate(0, 0, 1).Format("2006-01-02 15:04")
	ks := []*KlineMinute{{Time: day.Format("2006-01-02 15:04"), Source: MinuteSourceTrade}}
	if err := s.SaveMinuteFallbackComplete(c, 2025, day.Format("2006-01-02 15:04"), through, ks); err != nil {
		t.Fatal(err)
	}
	// 事务失败必须同时回滚数据和新覆盖范围。
	if err := s.SaveMinuteFallbackComplete(c, 2025, day.AddDate(0, 0, -1).Format("2006-01-02 15:04"), through, append(ks, ks[0])); err == nil {
		t.Fatal("重复主键应失败")
	}
	coverage, err = s.QueryMinuteCoverage(c, 2025)
	if err != nil || coverage.From != day.Format("2006-01-02 15:04") || coverage.Through != through || !coverage.TradeFallback {
		t.Fatal("覆盖标记未回滚", coverage, err)
	}
	rows, err = s.QueryMinute(c, day.Format("2006-01-02 15:04"), day.AddDate(0, 0, 1).Format("2006-01-02 15:04"))
	if err != nil || len(rows) != 1 || rows[0].Source != MinuteSourceTrade {
		t.Fatal("合成来源未持久化", rows, err)
	}
}

func TestMinuteMergedSource(t *testing.T) {
	day := time.Date(2025, 1, 6, 9, 31, 0, 0, time.Local)
	rows := []*KlineMinute{
		{Time: day.Format("2006-01-02 15:04"), Open: 1, High: 1, Low: 1, Close: 1},
		{Time: day.Add(time.Minute).Format("2006-01-02 15:04"), Open: 2, High: 2, Low: 2, Close: 2, Source: MinuteSourceTrade},
	}
	merged := MinuteToPeriod(rows, 2)
	if len(merged) != 1 || merged[0].Source != MinuteSourceTrade {
		t.Fatal("固定周期丢失合成来源", merged)
	}
	merged, err := MinuteToSessions(rows, 5, []TradingSession{{StartMinute: 570, EndMinute: 690}})
	if err != nil || len(merged) != 1 || merged[0].Source != MinuteSourceTrade {
		t.Fatal("交易时段聚合丢失合成来源", merged, err)
	}
}
