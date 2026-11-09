package pull

import (
	"errors"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/extend/model"
)

type updateTestUnit struct {
	dayCalls    int
	minCalls    int
	failDay     bool
	dayFailures int
	minFailures int
	err         error
}

func (*updateTestUnit) Name() string { return "update-test" }
func (*updateTestUnit) Schedule() Schedule {
	return Schedule{TimeZone: "Asia/Shanghai", CloseTime: "15:30", Delay: 5 * time.Minute}
}

type temporaryTestError struct{ error }

func (temporaryTestError) Temporary() bool { return true }
func (*updateTestUnit) Codes(*Service) ([]model.Code, error) {
	return []model.Code{{Code: "one"}}, nil
}
func (u *updateTestUnit) FetchDay(*Service, model.Code) error {
	u.dayCalls++
	if u.failDay || u.dayCalls <= u.dayFailures {
		return u.err
	}
	return nil
}
func (u *updateTestUnit) FetchMinute(*Service, model.Code) error {
	u.minCalls++
	if u.minCalls <= u.minFailures {
		return u.err
	}
	return nil
}

func TestUpdateFailureAndMustSemantics(t *testing.T) {
	s := testService(t)
	s.goro, s.retry = 1, 2
	want := temporaryTestError{errors.New("fetch failed")}
	u := &updateTestUnit{failDay: true, err: want}
	s.units = []Unit{u}
	if err := s.Update(true); !errors.Is(err, want) {
		t.Fatal(err)
	}
	if u.dayCalls != 2 || u.minCalls != 0 {
		t.Fatal("重试次数或执行顺序错误", u.dayCalls, u.minCalls)
	}
	key := s.updateKey("pull:update-test")
	if _, done, err := s.store.LoadUpdate(key); done || err != nil {
		t.Fatal("失败后标记完成", key, done, err)
	}
	u.failDay = false
	if err := s.Update(true); err != nil {
		t.Fatal(err)
	}
	if u.dayCalls != 3 || u.minCalls != 1 {
		t.Fatal("失败市场未恢复", u.dayCalls, u.minCalls)
	}
	if _, done, err := s.store.LoadUpdate(key); !done || err != nil {
		t.Fatal("成功后未标记", key, done, err)
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
	u := &updateTestUnit{minFailures: 1, err: temporaryTestError{errors.New("temporary failure")}}
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
	u := &updateTestUnit{minFailures: 2, err: temporaryTestError{errors.New("temporary failure")}}
	if err := s.updateUnit(u); err != nil {
		t.Fatal(err)
	}
	if u.dayCalls != 1 || u.minCalls != 3 {
		t.Fatal("分钟重试重复了日线", u.dayCalls, u.minCalls)
	}
	u = &updateTestUnit{minFailures: 3, err: temporaryTestError{errors.New("minute failed")}}
	if err := s.updateUnit(u); !errors.Is(err, u.err) {
		t.Fatal("重试耗尽未传递错误", err)
	}
	if u.dayCalls != 1 || u.minCalls != 3 {
		t.Fatal("最大尝试次数错误", u.dayCalls, u.minCalls)
	}
	u = &updateTestUnit{dayFailures: 2, minFailures: 2, err: temporaryTestError{errors.New("temporary failure")}}
	if err := s.updateUnit(u); err != nil {
		t.Fatal("两个阶段未获得独立的尝试次数", err)
	}
	if u.dayCalls != 3 || u.minCalls != 3 {
		t.Fatal("两个阶段的重试次数错误", u.dayCalls, u.minCalls)
	}
}
