package pull

import (
	"errors"
	"fmt"
	"github.com/injoyai/tdx/v2/protocol"
	"sync"
	"testing"
)

func TestMinuteCoverageRollback(t *testing.T) {
	s := testService(t)
	c := Code{Market: protocol.MarketAStock, Code: "sz000001"}
	if last, err := s.LastMinuteTime(c, 2025); err != nil || last != "" || s.MinuteExists(c, 2025) {
		t.Fatalf("读取空库创建了文件或返回异常: %s %v", last, err)
	}
	if last, err := s.LastDayTime(c); err != nil || last != "" || exists(s.DayFile(c)) {
		t.Fatal("读取空日线库创建了文件", err)
	}
	ks := []*KlineMinute{{Time: "2025-01-05 09:31", Close: 2}}
	if err := s.SaveMinute(c, 2025, "", ks); err != nil {
		t.Fatal(err)
	}
	if done, err := s.MinuteYearComplete(c, 2025, "2025-01-01 00:00"); done || err != nil {
		t.Fatal("旧库不应算完成", done, err)
	}
	bad := []*KlineMinute{{Time: "2025-01-05 09:31", Close: 5}, {Time: "2025-01-05 09:31", Close: 6}}
	if err := s.SaveMinuteComplete(c, 2025, "2025-01-01 00:00", bad); err == nil {
		t.Fatal("重复主键应回滚")
	}
	if done, err := s.MinuteYearComplete(c, 2025, "2025-01-01 00:00"); done || err != nil {
		t.Fatal("失败后标记完成", done, err)
	}
	// 测试记录使用合成时间戳；直接检查表中原值验证删除也被回滚。
	db, release, err := s.openMinute(c, 2025)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := queryMin(db, "", "")
	release()
	if err != nil || len(rows) != 1 || rows[0].Close != 2 {
		t.Fatalf("事务未回滚: %+v %v", rows, err)
	}
	if err := s.SaveMinuteComplete(c, 2025, "2025-01-01 00:00", ks); err != nil {
		t.Fatal(err)
	}
	if done, err := s.MinuteYearComplete(c, 2025, "2025-01-01 00:00"); !done || err != nil {
		t.Fatal("提交后未完成", done, err)
	}
	if done, err := s.MinuteYearComplete(c, 2025, "2024-12-31 00:00"); done || err != nil {
		t.Fatal("更早区间应补拉", done, err)
	}
	// 服务器仅保留较新的数据时，重新认证历史区间不能删除旧本地前缀。
	if err := s.SaveMinuteComplete(c, 2025, "2024-12-31 00:00", []*KlineMinute{{Time: "2025-01-05 09:32", Close: 3}}); err != nil {
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
	if err := s.SaveMinuteComplete(c, 2024, "", nil); err != nil || s.MinuteExists(c, 2024) {
		t.Fatal("空年份不应建库", err)
	}
}

func TestEngineLimitAndBorrow(t *testing.T) {
	s := testService(t)
	s.engineLimit = 2
	c := Code{Market: protocol.MarketAStock, Code: "sz000001"}
	db, release, err := s.openDay(c)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for i := 0; i < 8; i++ {
		if err := s.SaveDay(Code{Market: protocol.MarketAStock, Code: fmt.Sprintf("sz%06d", i+10)}, "", []*KlineDay{{Time: "2026-01-05"}}); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.engines) > 2 {
		t.Fatal("缓存未限制", len(s.engines))
	}
	if _, err := LastTime(db, new(KlineDay)); err != nil {
		t.Fatal("使用中的引擎被关闭", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := LastTime(db, new(KlineDay)); err != nil {
		t.Fatal("Close 中断了借用", err)
	}
	release()
	if len(s.engines) != 0 {
		t.Fatal("释放后仍有引擎")
	}
	if err := s.SaveDay(c, "", []*KlineDay{{Time: "2026-01-05"}}); err == nil {
		t.Fatal("关闭后重新打开引擎")
	}
}

func TestEngineConcurrentEviction(t *testing.T) {
	s := testService(t)
	s.engineLimit = 2
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := Code{Market: protocol.MarketAStock, Code: fmt.Sprintf("sz%06d", i)}
			for j := 0; j < 3; j++ {
				if err := s.SaveDay(c, "", []*KlineDay{{Time: "2026-01-05", Close: float64(i)}}); err != nil {
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

type updateTestUnit struct {
	dayCalls    int
	minCalls    int
	failDay     bool
	dayFailures int
	minFailures int
	err         error
}

func (*updateTestUnit) Name() string { return "update-test" }
func (*updateTestUnit) Codes(*Service) ([]Code, error) {
	return []Code{{Code: "one"}}, nil
}
func (u *updateTestUnit) FetchDay(*Service, Code) error {
	u.dayCalls++
	if u.failDay || u.dayCalls <= u.dayFailures {
		return u.err
	}
	return nil
}
func (u *updateTestUnit) FetchMinute(*Service, Code) error {
	u.minCalls++
	if u.minCalls <= u.minFailures {
		return u.err
	}
	return nil
}

func TestUpdateFailureAndMustSemantics(t *testing.T) {
	s := testService(t)
	s.goro, s.retry = 1, 2
	want := errors.New("fetch failed")
	u := &updateTestUnit{failDay: true, err: want}
	s.units = []Unit{u}
	if err := s.Update(true); !errors.Is(err, want) {
		t.Fatal(err)
	}
	if u.dayCalls != 2 || u.minCalls != 0 {
		t.Fatal("重试次数或执行顺序错误", u.dayCalls, u.minCalls)
	}
	for _, key := range []string{s.updateKey("pull:update-test"), s.updateKey("pull")} {
		if done, err := s.updated.IsUpdated(key); done || err != nil {
			t.Fatal("失败后标记完成", key, done, err)
		}
	}
	u.failDay = false
	if err := s.Update(true); err != nil {
		t.Fatal(err)
	}
	if u.dayCalls != 3 || u.minCalls != 1 {
		t.Fatal("失败市场未恢复", u.dayCalls, u.minCalls)
	}
	for _, key := range []string{s.updateKey("pull:update-test"), s.updateKey("pull")} {
		if done, err := s.updated.IsUpdated(key); !done || err != nil {
			t.Fatal("成功后未标记", key, done, err)
		}
	}
	// must=true 仍保留市场级去重。
	if err := s.Update(true); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(); err != nil {
		t.Fatal(err)
	}
	if u.dayCalls != 3 || u.minCalls != 1 {
		t.Fatal("去重语义发生变化")
	}
}

func TestUpdateMinuteOnlyRetry(t *testing.T) {
	s := testService(t)
	s.day, s.minute, s.retry = false, true, 3
	u := &updateTestUnit{minFailures: 1, err: errors.New("temporary failure")}
	s.units = []Unit{u}
	if err := s.Update(true); err != nil {
		t.Fatal(err)
	}
	if u.dayCalls != 0 || u.minCalls != 2 {
		t.Fatal("分钟重试未执行", u.dayCalls, u.minCalls)
	}
}

func TestUpdateMinuteRetryKeepsSuccessfulDay(t *testing.T) {
	s := testService(t)
	s.goro, s.retry = 1, 3
	u := &updateTestUnit{minFailures: 2, err: errors.New("temporary failure")}
	if err := s.updateUnit(u); err != nil {
		t.Fatal(err)
	}
	if u.dayCalls != 1 || u.minCalls != 3 {
		t.Fatal("分钟重试重复了日线", u.dayCalls, u.minCalls)
	}
	u = &updateTestUnit{minFailures: 3, err: errors.New("minute failed")}
	if err := s.updateUnit(u); !errors.Is(err, u.err) {
		t.Fatal("重试耗尽未传递错误", err)
	}
	if u.dayCalls != 1 || u.minCalls != 3 {
		t.Fatal("最大尝试次数错误", u.dayCalls, u.minCalls)
	}
	u = &updateTestUnit{dayFailures: 2, minFailures: 2, err: errors.New("temporary failure")}
	if err := s.updateUnit(u); err != nil {
		t.Fatal("两个阶段未获得独立的尝试次数", err)
	}
	if u.dayCalls != 3 || u.minCalls != 3 {
		t.Fatal("两个阶段的重试次数错误", u.dayCalls, u.minCalls)
	}
}
