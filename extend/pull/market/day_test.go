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
	old := &pull.KlineDay{Unix: s.Start().Add(15 * time.Hour).Unix(), Close: 10}
	if err := s.SaveDay(code, 0, []*pull.KlineDay{old}); err != nil {
		t.Fatal(err)
	}
	newer := &pull.KlineDay{Unix: old.Unix + 86400, Close: 11}
	newest := &pull.KlineDay{Unix: newer.Unix + 86400, Close: 12}
	if err := saveDays(s, code, old.Unix, []*pull.KlineDay{newest, newer}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.QueryDay(code, time.Time{}, time.Time{})
	if err != nil || len(rows) != 3 || rows[0].Unix != old.Unix {
		t.Fatal("服务器未返回的旧记录丢失", rows, err)
	}
	// Until 可能返回界外停止记录；只能覆盖返回的增量边界，不能重复插入旧前缀。
	if err := saveDays(s, code, newest.Unix, []*pull.KlineDay{newer, {Unix: newest.Unix, Close: 99}}); err != nil {
		t.Fatal(err)
	}
	rows, err = s.QueryDay(code, time.Time{}, time.Time{})
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
	from := s.Start().Unix()
	code := pull.Code{Market: protocol.MarketIndex, Code: "sh000001"}
	if err := saveDays(s, code, from, []*pull.KlineDay{{Unix: from - 1}, {Unix: from, Close: 10}}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.QueryDay(code, time.Time{}, time.Time{})
	if err != nil || len(rows) != 1 || rows[0].Unix != from {
		t.Fatal("起始边界错误", rows, err)
	}
	if err := saveDays(s, code, from, []*pull.KlineDay{{Unix: from - 1}}); err != nil {
		t.Fatal(err)
	}
	rows, err = s.QueryDay(code, time.Time{}, time.Time{})
	if err != nil || len(rows) != 1 || rows[0].Close != 10 {
		t.Fatal("空结果侵蚀旧记录", rows, err)
	}
	empty := pull.Code{Market: protocol.MarketHK, Code: "00700"}
	if err := saveDays(s, empty, from+86400, []*pull.KlineDay{{Unix: from}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.LastDayUnix(empty); err != nil {
		t.Fatal(err)
	}
	if rows, err := s.QueryDay(empty, time.Time{}, time.Time{}); err != nil || len(rows) != 0 {
		t.Fatal("未来起始日期写入已有行情", rows, err)
	}
}
