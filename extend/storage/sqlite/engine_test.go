package sqlite

import (
	"fmt"
	"sync"
	"testing"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestMinuteCoverageRollback(t *testing.T) {
	s := testStore(t)
	registerMockOnce.Do(func() { pull.Register(&mockUnit{}) })
	service, err := pull.New(&pull.Config{Store: s, Codes: []string{"sqlite-test.one"}})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	c := model.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	if last, err := s.LastMinuteTime(c, 2025); err != nil || last != "" || s.MinuteExists(c, 2025) {
		t.Fatalf("读取空库创建了文件或返回异常: %s %v", last, err)
	}
	if last, err := s.LastDayTime(c); err != nil || last != "" || exists(s.DayFile(c)) {
		t.Fatal("读取空日线库创建了文件", err)
	}
	ks := []*model.KlineMinute{{Time: "2025-01-05 09:31", Close: 2}}
	if err := s.SaveMinute(c, 2025, "", ks, nil); err != nil {
		t.Fatal(err)
	}
	if done, err := service.MinuteYearComplete(c, 2025, "2025-01-01 00:00"); done || err != nil {
		t.Fatal("旧库不应算完成", done, err)
	}
	bad := []*model.KlineMinute{{Time: "2025-01-05 09:31", Close: 5}, {Time: "2025-01-05 09:31", Close: 6}}
	if err := service.SaveMinuteComplete(c, 2025, "2025-01-01 00:00", bad); err == nil {
		t.Fatal("重复主键应回滚")
	}
	if done, err := service.MinuteYearComplete(c, 2025, "2025-01-01 00:00"); done || err != nil {
		t.Fatal("失败后标记完成", done, err)
	}
	// 直接检查源时间记录，验证删尾操作也被事务回滚。
	db, release, err := s.openMinute(c, 2025)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := queryMin(db, "", "")
	release()
	if err != nil || len(rows) != 1 || rows[0].Close != 2 {
		t.Fatalf("事务未回滚: %+v %v", rows, err)
	}
	if err := service.SaveMinuteComplete(c, 2025, "2025-01-01 00:00", ks); err != nil {
		t.Fatal(err)
	}
	if done, err := service.MinuteYearComplete(c, 2025, "2025-01-01 00:00"); !done || err != nil {
		t.Fatal("提交后未完成", done, err)
	}
	if done, err := service.MinuteYearComplete(c, 2025, "2024-12-31 00:00"); done || err != nil {
		t.Fatal("更早区间应补拉", done, err)
	}
	// 服务器仅保留较新的数据时，重新认证历史区间不能删除旧本地前缀。
	if err := service.SaveMinuteComplete(c, 2025, "2024-12-31 00:00", []*model.KlineMinute{{Time: "2025-01-05 09:32", Close: 3}}); err != nil {
		t.Fatal(err)
	}
	db, release, err = s.openMinute(c, 2025)
	if err != nil {
		t.Fatal(err)
	}
	rows, err = queryMin(db, "", "")
	release()
	if err != nil || len(rows) != 2 || rows[0].Time != "2025-01-05 09:31" {
		t.Fatal("本地历史前缀丢失", rows, err)
	}
	if err := service.SaveMinuteComplete(c, 2024, "", nil); err != nil || s.MinuteExists(c, 2024) {
		t.Fatal("空年份不应建库", err)
	}
}

func TestEngineLimitAndBorrow(t *testing.T) {
	s := testStore(t)
	s.engineLimit = 2
	c := model.Code{Market: protocol.MarketAStock, Code: "sz000001"}
	db, release, err := s.openDay(c)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for i := 0; i < 8; i++ {
		if err := s.SaveDay(model.Code{Market: protocol.MarketAStock, Code: fmt.Sprintf("sz%06d", i+10)}, "", []*model.KlineDay{{Time: "2026-01-05"}}); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.engines) > 2 {
		t.Fatal("缓存未限制", len(s.engines))
	}
	if _, err := LastTime(db, new(model.KlineDay)); err != nil {
		t.Fatal("使用中的引擎被关闭", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := LastTime(db, new(model.KlineDay)); err != nil {
		t.Fatal("Close 中断了借用", err)
	}
	release()
	if len(s.engines) != 0 {
		t.Fatal("释放后仍有引擎")
	}
	if err := s.SaveDay(c, "", []*model.KlineDay{{Time: "2026-01-05"}}); err == nil {
		t.Fatal("关闭后重新打开引擎")
	}
}

func TestEngineConcurrentEviction(t *testing.T) {
	s := testStore(t)
	s.engineLimit = 2
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := model.Code{Market: protocol.MarketAStock, Code: fmt.Sprintf("sz%06d", i)}
			for j := 0; j < 3; j++ {
				if err := s.SaveDay(c, "", []*model.KlineDay{{Time: "2026-01-05", Close: float64(i)}}); err != nil {
					t.Error(err)
					return
				}
				rows, err := s.QueryDay(c, "", "")
				if err != nil || len(rows) != 1 || rows[0].Close != float64(i) {
					t.Errorf("淘汰后读写异常 %v", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()
	if len(s.engines) > 2 {
		t.Fatal("缓存未回落", len(s.engines))
	}
}
