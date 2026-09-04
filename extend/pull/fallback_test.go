package pull

import (
	"testing"
	"time"

	"github.com/injoyai/tdx/lib/xorms"
)

func TestMinuteFallbackSchemaAndSource(t *testing.T) {
	s := testService(t)
	c := Code{Market: MarketAStock, Code: "sz000001"}
	day := time.Date(2025, 1, 6, 0, 0, 0, 0, time.Local)
	db, err := xorms.NewSqlite(s.MinFile(c, 2025))
	if err != nil {
		t.Fatal(err)
	}
	// 模拟升级前七列分钟表及只有 From 的覆盖表。
	_, err = db.Exec("CREATE TABLE KlineMinute (Unix INTEGER PRIMARY KEY, Open REAL, High REAL, Low REAL, Close REAL, Volume INTEGER, Amount REAL)")
	if err == nil {
		_, err = db.Exec("INSERT INTO KlineMinute VALUES (?,1,1,1,1,100,100)", day.Unix())
	}
	if err == nil {
		_, err = db.Exec("CREATE TABLE minuteCoverage (ID INTEGER PRIMARY KEY, [From] INTEGER)")
	}
	if err == nil {
		_, err = db.Exec("INSERT INTO minuteCoverage VALUES (1,?)", day.Unix())
	}
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.QueryMin(c, day, day.AddDate(0, 0, 1))
	if err != nil || len(rows) != 1 || rows[0].Source != "" {
		t.Fatal("旧库原生来源迁移失败", rows, err)
	}
	coverage, err := s.QueryMinCoverage(c, 2025)
	if err != nil || coverage == nil || coverage.TradeFallback || coverage.Through != 0 {
		t.Fatal("旧标记误判兜底完成", coverage, err)
	}
	through := day.AddDate(0, 0, 1).Unix()
	ks := []*KlineMinute{{Unix: day.Unix(), Source: MinuteSourceTrade}}
	if err := s.SaveMinFallbackComplete(c, 2025, day.Unix(), through, ks); err != nil {
		t.Fatal(err)
	}
	// 事务失败必须同时回滚数据和新覆盖范围。
	if err := s.SaveMinFallbackComplete(c, 2025, day.AddDate(0, 0, -1).Unix(), through, append(ks, ks[0])); err == nil {
		t.Fatal("重复主键应失败")
	}
	coverage, err = s.QueryMinCoverage(c, 2025)
	if err != nil || coverage.From != day.Unix() || coverage.Through != through || !coverage.TradeFallback {
		t.Fatal("覆盖标记未回滚", coverage, err)
	}
	rows, err = s.QueryMin(c, day, day.AddDate(0, 0, 1))
	if err != nil || len(rows) != 1 || rows[0].Source != MinuteSourceTrade {
		t.Fatal("合成来源未持久化", rows, err)
	}
}

func TestMinuteMergedSource(t *testing.T) {
	day := time.Date(2025, 1, 6, 9, 31, 0, 0, time.Local)
	rows := []*KlineMinute{
		{Unix: day.Unix(), Open: 1, High: 1, Low: 1, Close: 1},
		{Unix: day.Add(time.Minute).Unix(), Open: 2, High: 2, Low: 2, Close: 2, Source: MinuteSourceTrade},
	}
	merged := MinuteToPeriod(rows, 2)
	if len(merged) != 1 || merged[0].Source != MinuteSourceTrade {
		t.Fatal("固定周期丢失合成来源", merged)
	}
	merged, err := MinuteToSessions(rows, 5, time.Local, []TradingSession{{StartMinute: 570, EndMinute: 690}})
	if err != nil || len(merged) != 1 || merged[0].Source != MinuteSourceTrade {
		t.Fatal("交易时段聚合丢失合成来源", merged, err)
	}
}
