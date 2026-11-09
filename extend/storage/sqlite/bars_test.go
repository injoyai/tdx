package sqlite

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

func testAmount(v float64) *float64 { return &v }

func TestStoreDay(t *testing.T) {
	s := testStore(t)
	code := model.Code{Market: protocol.MarketAStock, Code: "sh600000"}

	// 首次插入 3 根
	base := time.Date(2026, 1, 5, 15, 0, 0, 0, time.Local)
	ks := []*model.KlineDay{
		{Time: base.Format("2006-01-02"), Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100, Amount: testAmount(150)},
		{Time: base.AddDate(0, 0, 1).Format("2006-01-02"), Open: 1.5, High: 2.5, Low: 1, Close: 2, Volume: 200, Amount: testAmount(400)},
		{Time: base.AddDate(0, 0, 2).Format("2006-01-02"), Open: 2, High: 3, Low: 1.5, Close: 2.5, Volume: 300, Amount: testAmount(750)},
	}
	if err := s.SaveDay(code, "", ks); err != nil {
		t.Fatalf("SaveDay: %v", err)
	}

	last, err := s.LastDayTime(code)
	if err != nil {
		t.Fatalf("LastDayTime: %v", err)
	}
	if last != ks[2].Time {
		t.Errorf("LastDayTime = %s, want %s", last, ks[2].Time)
	}

	// 增量：从 last 之后追加一根，模拟当天更新的最新 K 线
	next := &model.KlineDay{Time: base.AddDate(0, 0, 3).Format("2006-01-02"), Open: 2.5, High: 3.5, Low: 2, Close: 3, Volume: 400, Amount: testAmount(1200)}
	if err := s.SaveDay(code, last, []*model.KlineDay{next}); err != nil {
		t.Fatalf("SaveDay 增量: %v", err)
	}
	last2, err := s.LastDayTime(code)
	if err != nil {
		t.Fatalf("LastDayTime2: %v", err)
	}
	if last2 != next.Time {
		t.Errorf("LastDayTime 增量后 = %s, want %s", last2, next.Time)
	}

	// 查询区间（第 3 根被增量替换为 base+3，库里共 3 根）
	dayFile := dayFile(code, s.dir)
	db, err := openDB(dayFile, new(model.KlineDay))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()
	ls, err := queryDay(db, base.Format("2006-01-02"), base.AddDate(0, 0, 3).Format("2006-01-02"))
	if err != nil {
		t.Fatalf("queryDay: %v", err)
	}
	if len(ls) != 3 {
		t.Fatalf("queryDay = %d 根, want 3", len(ls))
	}
	if ls[0].Time != ks[0].Time {
		t.Errorf("queryDay 升序: first = %s, want %s", ls[0].Time, ks[0].Time)
	}
}

func TestStoreDayUpsert(t *testing.T) {
	s := testStore(t)
	code := model.Code{Market: protocol.MarketIndex, Code: "sh000001"}

	base := time.Date(2026, 1, 5, 15, 0, 0, 0, time.Local)
	ks := []*model.KlineDay{
		{Time: base.Format("2006-01-02"), Open: 1, Close: 2, Volume: 100},
		{Time: base.AddDate(0, 0, 1).Format("2006-01-02"), Open: 2, Close: 3, Volume: 200},
	}
	if err := s.SaveDay(code, "", ks); err != nil {
		t.Fatalf("SaveDay: %v", err)
	}

	// 用 from=第一根 重新写入（幂等覆盖），数据量多一根且改动第一根
	ks2 := []*model.KlineDay{
		{Time: base.Format("2006-01-02"), Open: 9, Close: 99, Volume: 999},
		{Time: base.AddDate(0, 0, 1).Format("2006-01-02"), Open: 2, Close: 3, Volume: 200},
		{Time: base.AddDate(0, 0, 2).Format("2006-01-02"), Open: 3, Close: 4, Volume: 300},
	}
	if err := s.SaveDay(code, ks[0].Time, ks2); err != nil {
		t.Fatalf("SaveDay upsert: %v", err)
	}

	last, _ := s.LastDayTime(code)
	if last != ks2[2].Time {
		t.Errorf("upsert 后 LastDayTime = %s, want %s", last, ks2[2].Time)
	}
	dayFile := dayFile(code, s.dir)
	db, err := openDB(dayFile, new(model.KlineDay))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()
	ls, _ := queryDay(db, "", time.Now().AddDate(0, 0, 10).Format(time.DateOnly))
	if len(ls) != 3 {
		t.Fatalf("upsert 后 = %d 根, want 3", len(ls))
	}
	if ls[0].Open != 9 || ls[0].Close != 99 {
		t.Errorf("第一根未被覆盖: %+v", ls[0])
	}
}

func TestStoreMinute(t *testing.T) {
	s := testStore(t)
	code := model.Code{Market: protocol.MarketAStock, Code: "sz000001"}

	year := 2026
	base := time.Date(year, 1, 5, 9, 31, 0, 0, time.Local)
	ks := make([]*model.KlineMinute, 0, 3)
	for i := 0; i < 3; i++ {
		ks = append(ks, &model.KlineMinute{
			Time:   base.Add(time.Duration(i) * time.Minute).Format("2006-01-02 15:04"),
			Open:   float64(i + 1),
			Close:  float64(i + 1),
			Volume: int64(i+1) * 100,
		})
	}
	if err := s.SaveMinute(code, year, "", ks, nil); err != nil {
		t.Fatalf("SaveMinute: %v", err)
	}

	last, err := s.LastMinuteTime(code, year)
	if err != nil {
		t.Fatalf("LastMinuteTime: %v", err)
	}
	if last != ks[2].Time {
		t.Errorf("LastMinuteTime = %s, want %s", last, ks[2].Time)
	}

	if !s.MinuteExists(code, year) {
		t.Errorf("MinExists(%d) = false, want true", year)
	}
	if s.MinuteExists(code, year+1) {
		t.Errorf("MinExists(%d) = true, want false", year+1)
	}

	// 查询
	minFile := minuteFile(code, s.dir, year)
	db, err := openDB(minFile, new(model.KlineMinute))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()
	ls, err := queryMin(db, base.Format("2006-01-02 15:04"), base.Add(2*time.Minute).Format("2006-01-02 15:04"))
	if err != nil {
		t.Fatalf("queryMin: %v", err)
	}
	if len(ls) != 3 {
		t.Fatalf("queryMin = %d 根, want 3", len(ls))
	}
}

func TestDirCreation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")

	s, err := NewPull(dir, 2)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()
	code := model.Code{Market: protocol.MarketAStock, Code: "sh600000"}
	if err := s.SaveDay(code, "", []*model.KlineDay{{Time: "2026-01-05", Open: 1, Close: 1, Volume: 100}}); err != nil {
		t.Fatalf("SaveDay: %v", err)
	}
	// 地区/资产两级目录应自动创建
	if _, err := os.Stat(filepath.Join(dir, "cn", "stock", "day", "sh600000.db")); err != nil {
		t.Errorf("地区/资产/day 目录/文件未创建: %v", err)
	}
}

func TestQueryDay(t *testing.T) {
	s := testStore(t)
	code := model.Code{Market: protocol.MarketAStock, Code: "sh600000"}

	// 未建库：返回空且不创建文件
	ls, err := s.QueryDay(code, "", "")
	if err != nil {
		t.Fatalf("QueryDay 空库: %v", err)
	}
	if len(ls) != 0 {
		t.Fatalf("QueryDay 空库 = %d 根, want 0", len(ls))
	}
	if _, err := os.Stat(dayFile(code, s.dir)); !os.IsNotExist(err) {
		t.Fatalf("QueryDay 空库不应创建文件")
	}

	// 写入后按范围查询
	base := time.Date(2026, 1, 5, 15, 0, 0, 0, time.Local)
	ks := []*model.KlineDay{
		{Time: base.Format("2006-01-02"), Open: 1, Close: 1, Volume: 100},
		{Time: base.AddDate(0, 0, 1).Format("2006-01-02"), Open: 2, Close: 2, Volume: 200},
		{Time: base.AddDate(0, 0, 2).Format("2006-01-02"), Open: 3, Close: 3, Volume: 300},
	}
	if err := s.SaveDay(code, "", ks); err != nil {
		t.Fatalf("SaveDay: %v", err)
	}

	// 全范围（零值端点不限制）
	ls, err = s.QueryDay(code, "", "")
	if err != nil {
		t.Fatalf("QueryDay: %v", err)
	}
	if len(ls) != 3 {
		t.Fatalf("QueryDay 全范围 = %d 根, want 3", len(ls))
	}
	// 半开区间
	ls, err = s.QueryDay(code, base.AddDate(0, 0, 1).Format("2006-01-02"), "")
	if err != nil {
		t.Fatalf("QueryDay 半开: %v", err)
	}
	if len(ls) != 2 {
		t.Fatalf("QueryDay 半开 = %d 根, want 2", len(ls))
	}
}

func TestQueryMinute(t *testing.T) {
	s := testStore(t)
	code := model.Code{Market: protocol.MarketAStock, Code: "sz000001"}

	year := 2025
	base := time.Date(year, 1, 5, 9, 31, 0, 0, time.Local)
	ks := []*model.KlineMinute{}
	for i := 0; i < 3; i++ {
		ks = append(ks, &model.KlineMinute{
			Time:   base.Add(time.Duration(i) * time.Minute).Format("2006-01-02 15:04"),
			Open:   float64(i + 1),
			Close:  float64(i + 1),
			Volume: int64(i+1) * 100,
		})
	}
	if err := s.SaveMinute(code, year, "", ks, nil); err != nil {
		t.Fatalf("SaveMinute: %v", err)
	}

	// 跨零值端点查询（自动定位年份文件）
	ls, err := s.QueryMinute(code, "", "")
	if err != nil {
		t.Fatalf("QueryMinute: %v", err)
	}
	if len(ls) != 3 {
		t.Fatalf("QueryMinute 全范围 = %d 根, want 3", len(ls))
	}
	// 范围过滤
	ls, err = s.QueryMinute(code, base.Format("2006-01-02 15:04"), base.Add(time.Minute).Format("2006-01-02 15:04"))
	if err != nil {
		t.Fatalf("QueryMinute 范围: %v", err)
	}
	if len(ls) != 2 {
		t.Fatalf("QueryMinute 范围 = %d 根, want 2", len(ls))
	}
}

func TestEngineCache(t *testing.T) {
	s := testStore(t)
	code := model.Code{Market: protocol.MarketAStock, Code: "sh600000"}

	db1, release1, err := s.openDay(code)
	if err != nil {
		t.Fatalf("openDay: %v", err)
	}
	defer release1()
	db2, release2, err := s.openDay(code)
	if err != nil {
		t.Fatalf("openDay 2: %v", err)
	}
	if db1 != db2 {
		t.Fatalf("引擎缓存未生效：两次打开返回不同实例")
	}
	defer release2()
	// 写入后查询走缓存引擎
	if err := s.SaveDay(code, "", []*model.KlineDay{{Time: "2026-01-05", Open: 1, Close: 1, Volume: 100}}); err != nil {
		t.Fatalf("SaveDay: %v", err)
	}
	last, err := s.LastDayTime(code)
	if err != nil {
		t.Fatalf("LastDayTime: %v", err)
	}
	if last != "2026-01-05" {
		t.Fatalf("LastDayTime = %s, want 1", last)
	}
}

func TestSaveEmptySkipsWrite(t *testing.T) {
	s := testStore(t)
	code := model.Code{Market: protocol.MarketAStock, Code: "sh600000"}

	// 空数据写入应跳过，不创建库文件
	if err := s.SaveDay(code, "", nil); err != nil {
		t.Fatalf("SaveDay 空数据: %v", err)
	}
	if _, err := os.Stat(dayFile(code, s.dir)); !os.IsNotExist(err) {
		t.Fatalf("空数据不应创建文件")
	}
}

func TestInsertBatchOverLimit(t *testing.T) {
	s := testStore(t)
	code := model.Code{Market: protocol.MarketAStock, Code: "sh600000"}

	// 200 根日线 × 10 字段 = 2000 变量 > 999，必然触发分批
	base := time.Date(2026, 1, 5, 15, 0, 0, 0, time.Local)
	ks := make([]*model.KlineDay, 0, 200)
	for i := 0; i < 200; i++ {
		ks = append(ks, &model.KlineDay{
			Time:   base.AddDate(0, 0, i).Format("2006-01-02"),
			Open:   float64(i),
			Close:  float64(i),
			Volume: int64(i),
		})
	}
	if err := s.SaveDay(code, "", ks); err != nil {
		t.Fatalf("SaveDay 超限批量: %v", err)
	}

	// 数据完整：200 根全部落库
	got, err := s.QueryDay(code, "", "")
	if err != nil {
		t.Fatalf("QueryDay: %v", err)
	}
	if len(got) != 200 {
		t.Fatalf("落库数量 = %d, want 200", len(got))
	}
	if got[0].Time != ks[0].Time || got[199].Time != ks[199].Time {
		t.Errorf("首尾源时间不符: got %s..%s, want %s..%s",
			got[0].Time, got[199].Time, ks[0].Time, ks[199].Time)
	}
}
