package market

import (
	"github.com/injoyai/tdx/v2/protocol"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/pull"
)

func TestSaveDaysPreservesExpiredHistoryAndRefreshesBoundary(t *testing.T) {
	s, err := pull.New(&pull.Config{Dir: t.TempDir(), StartAt: "20200101", Codes: []string{"sz000001"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	code := pull.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	old := &pull.KlineDay{Time: s.Start().Add(15 * time.Hour).Format("2006-01-02"), Close: 10}
	if err := s.SaveDay(code, "", []*pull.KlineDay{old}); err != nil {
		t.Fatal(err)
	}
	newer := &pull.KlineDay{Time: "2020-01-02", Close: 11}
	newest := &pull.KlineDay{Time: "2020-01-03", Close: 12}
	if err := saveDays(s, code, old.Time, []*pull.KlineDay{newest, newer}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.QueryDay(code, "", "")
	if err != nil || len(rows) != 3 || rows[0].Time != old.Time {
		t.Fatal("服务器未返回的旧记录丢失", rows, err)
	}
	// Until 可能返回界外停止记录；只能覆盖返回的增量边界，不能重复插入旧前缀。
	if err := saveDays(s, code, newest.Time, []*pull.KlineDay{newer, {Time: newest.Time, Close: 99}}); err != nil {
		t.Fatal(err)
	}
	rows, err = s.QueryDay(code, "", "")
	if err != nil || len(rows) != 3 || rows[2].Close != 99 {
		t.Fatal("边界未更新或重复插入旧记录", rows, err)
	}
}

func TestSaveDaysFiltersStartAndEmptyResult(t *testing.T) {
	s, err := pull.New(&pull.Config{Dir: t.TempDir(), StartAt: "20260101", Codes: []string{"sz000001"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	from := s.Start().Format("2006-01-02")
	code := pull.Code{Market: protocol.MarketIndex, Code: "sh000001"}
	if err := saveDays(s, code, from, []*pull.KlineDay{{Time: "2025-12-31"}, {Time: from, Close: 10}}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.QueryDay(code, "", "")
	if err != nil || len(rows) != 1 || rows[0].Time != from {
		t.Fatal("起始边界错误", rows, err)
	}
	if err := saveDays(s, code, from, []*pull.KlineDay{{Time: "2025-12-31"}}); err != nil {
		t.Fatal(err)
	}
	rows, err = s.QueryDay(code, "", "")
	if err != nil || len(rows) != 1 || rows[0].Close != 10 {
		t.Fatal("空结果侵蚀旧记录", rows, err)
	}
	empty := pull.Code{Market: protocol.MarketHK, Code: "00700"}
	if err := saveDays(s, empty, "2026-01-02", []*pull.KlineDay{{Time: from}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LastDayTime(empty); err != nil {
		t.Fatal(err)
	}
	if rows, err := s.QueryDay(empty, "", ""); err != nil || len(rows) != 0 {
		t.Fatal("未来起始日期写入已有行情", rows, err)
	}
}
