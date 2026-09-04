package pull

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestMinuteCoverageRollback(t *testing.T) {
	s := testService(t)
	c := Code{Market: MarketAStock, Code: "sz000001"}
	if last, err := s.LastMinUnix(c, 2025); err != nil || last != 0 || s.MinExists(c, 2025) {
		t.Fatalf("读取空库创建了文件或返回异常: %d %v", last, err)
	}
	if last, err := s.LastDayUnix(c); err != nil || last != 0 || exists(s.DayFile(c)) {
		t.Fatal("读取空日线库创建了文件", err)
	}
	ks := []*KlineMinute{{Unix: 10, Close: 2}}
	if err := s.SaveMin(c, 2025, 0, ks); err != nil {
		t.Fatal(err)
	}
	if done, err := s.MinYearComplete(c, 2025, 1); done || err != nil {
		t.Fatal("旧库不应算完成", done, err)
	}
	bad := []*KlineMinute{{Unix: 10, Close: 5}, {Unix: 10, Close: 6}}
	if err := s.SaveMinComplete(c, 2025, 1, bad); err == nil {
		t.Fatal("重复主键应回滚")
	}
	if done, err := s.MinYearComplete(c, 2025, 1); done || err != nil {
		t.Fatal("失败后标记完成", done, err)
	}
	// 测试记录使用合成时间戳；直接检查表中原值验证删除也被回滚。
	db, release, err := s.openMin(c, 2025)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := queryMin(db, time.Time{}, time.Time{})
	release()
	if err != nil || len(rows) != 1 || rows[0].Close != 2 {
		t.Fatalf("事务未回滚: %+v %v", rows, err)
	}
	if err := s.SaveMinComplete(c, 2025, 1, ks); err != nil {
		t.Fatal(err)
	}
	if done, err := s.MinYearComplete(c, 2025, 1); !done || err != nil {
		t.Fatal("提交后未完成", done, err)
	}
	if done, err := s.MinYearComplete(c, 2025, 0); done || err != nil {
		t.Fatal("更早区间应补拉", done, err)
	}
	// 服务器仅保留较新的数据时，重新认证历史区间不能删除旧本地前缀。
	if err := s.SaveMinComplete(c, 2025, 0, []*KlineMinute{{Unix: 20, Close: 3}}); err != nil {
		t.Fatal(err)
	}
	db, release, err = s.openMin(c, 2025)
	if err != nil {
		t.Fatal(err)
	}
	rows, err = queryMin(db, time.Time{}, time.Time{})
	release()
	if err != nil || len(rows) != 2 || rows[0].Unix != 10 {
		t.Fatal("本地历史前缀丢失", rows, err)
	}
	if err := s.SaveMinComplete(c, 2024, 0, nil); err != nil || s.MinExists(c, 2024) {
		t.Fatal("空年份不应建库", err)
	}
}

func TestEngineLimitAndBorrow(t *testing.T) {
	s := testService(t)
	s.engineLimit = 2
	c := Code{Market: MarketAStock, Code: "sz000001"}
	db, release, err := s.openDay(c)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for i := 0; i < 8; i++ {
		if err := s.SaveDay(Code{Market: MarketAStock, Code: fmt.Sprintf("sz%06d", i+10)}, 0, []*KlineDay{{Unix: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.engines) > 2 {
		t.Fatal("缓存未限制", len(s.engines))
	}
	if _, err := LastUnix(db, new(KlineDay)); err != nil {
		t.Fatal("使用中的引擎被关闭", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := LastUnix(db, new(KlineDay)); err != nil {
		t.Fatal("Close 中断了借用", err)
	}
	release()
	if len(s.engines) != 0 {
		t.Fatal("释放后仍有引擎")
	}
	if err := s.SaveDay(c, 0, []*KlineDay{{Unix: 1}}); err == nil {
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
			c := Code{Market: MarketAStock, Code: fmt.Sprintf("sz%06d", i)}
			for j := 0; j < 3; j++ {
				if err := s.SaveDay(c, 0, []*KlineDay{{Unix: 1, Close: float64(i)}}); err != nil {
					t.Error(err)
					return
				}
				rows, err := s.QueryDay(c, time.Time{}, time.Time{})
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
	minFailures int
	err         error
}

func (*updateTestUnit) Name() string { return "update-test" }
func (*updateTestUnit) Codes(*Service) ([]Code, error) {
	return []Code{{Code: "one"}}, nil
}
func (u *updateTestUnit) FetchDay(*Service, Code) error {
	u.dayCalls++
	if u.failDay {
		return u.err
	}
	return nil
}
func (u *updateTestUnit) FetchMin(*Service, Code) error {
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
	for _, key := range []string{"pull:update-test", "pull"} {
		if done, err := s.updated.Updated(key); done || err != nil {
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
	for _, key := range []string{"pull:update-test", "pull"} {
		if done, err := s.updated.Updated(key); !done || err != nil {
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
