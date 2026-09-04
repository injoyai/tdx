package market

import (
	"errors"
	"testing"
	"time"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/extend/pull"
	"github.com/injoyai/tdx/protocol"
)

func TestPullMinutesOneScanAndRecovery(t *testing.T) {
	s, err := pull.NewService(&pull.Config{Dir: t.TempDir(), StartAt: "20240101", Codes: []string{"sz000001"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	code := pull.Code{Market: pull.MarketAStock, Code: "sz000001"}
	now := time.Date(2026, 9, 4, 16, 0, 0, 0, time.Local)
	var data []*pull.KlineMinute
	for y := 2024; y <= 2026; y++ {
		data = append(data, &pull.KlineMinute{Unix: time.Date(y, 6, 1, 9, 31, 0, 0, time.Local).Unix(), Close: float64(y)})
	}
	// 旧历史库虽有数据，但没有完成标记，应重扫而不是从尾部继续。
	if err := s.SaveMin(code, 2024, 0, data[:1]); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("page failed")
	if err := pullMinutes(s, code, now, func(time.Time) ([]*pull.KlineMinute, error) { return data, failure }, nil); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if done, err := s.MinYearComplete(code, 2024, s.Start().Unix()); done || err != nil {
		t.Fatal("失败标记完成", done, err)
	}
	calls := 0
	fetch := func(from time.Time) ([]*pull.KlineMinute, error) {
		calls++
		if !from.Equal(s.Start()) {
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
		from := time.Date(y, 1, 1, 0, 0, 0, 0, time.Local).Unix()
		if done, err := s.MinYearComplete(code, y, from); !done || err != nil {
			t.Fatal("未完成", y, done, err)
		}
	}
	rows, err := s.QueryMin(code, time.Time{}, time.Time{})
	if err != nil || len(rows) != 3 {
		t.Fatal("分年落库失败", len(rows), err)
	}
	if err := pullMinutes(s, code, now, func(from time.Time) ([]*pull.KlineMinute, error) {
		if from.Unix() != data[2].Unix {
			t.Errorf("增量边界=%v", from)
		}
		return []*pull.KlineMinute{{Unix: data[2].Unix, Close: 99}}, nil
	}, nil); err != nil {
		t.Fatal(err)
	}
	rows, err = s.QueryMin(code, time.Time{}, time.Time{})
	if err != nil || len(rows) != 3 || rows[2].Close != 99 {
		t.Fatal("边界未覆盖", rows, err)
	}
}

func TestPullMinutesEmptyAndFailed(t *testing.T) {
	s, err := pull.NewService(&pull.Config{Dir: t.TempDir(), StartAt: "20240101", Codes: []string{"sz000001"}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := pull.Code{Market: pull.MarketAStock, Code: "sz000001"}
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.Local)
	for i := 0; i < 2; i++ {
		if err := pullMinutes(s, c, now, func(from time.Time) ([]*pull.KlineMinute, error) {
			if !from.Equal(s.Start()) {
				t.Fatal("空结果被记为已完成")
			}
			return nil, nil
		}, nil); err != nil {
			t.Fatal(err)
		}
	}
	if s.MinExists(c, 2024) || s.MinExists(c, 2026) {
		t.Fatal("空结果创建文件")
	}
	failed := errors.New("fetch failed")
	err = pullMinutes(s, c, now, func(time.Time) ([]*pull.KlineMinute, error) {
		return []*pull.KlineMinute{{Unix: now.Unix()}}, failed
	}, nil)
	if !errors.Is(err, failed) || s.MinExists(c, 2026) {
		t.Fatal("失败后仍写库", err)
	}
}

func TestReadExBarsBoundaryFailureAndLimit(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	page := make([]protocol.ExKline, 800)
	for i := range page {
		page[i].Datetime = base.Add(time.Duration(i) * time.Minute).Format("2006-01-02 15:04")
	}
	calls := 0
	rows, err := readExBars(base.Add(790*time.Minute), func(uint16, uint16) ([]protocol.ExKline, error) { calls++; return page, nil })
	if err != nil || len(rows) != 10 || calls != 1 {
		t.Fatal("未包含边界或重复请求", len(rows), calls, err)
	}
	want := errors.New("page failed")
	_, err = readExBars(base, func(uint16, uint16) ([]protocol.ExKline, error) { return nil, want })
	if !errors.Is(err, want) {
		t.Fatal("分页错误未传播", err)
	}
	lastOffset := -1
	_, err = readExBars(base.Add(-time.Minute), func(offset, count uint16) ([]protocol.ExKline, error) {
		if int(offset) <= lastOffset {
			t.Fatal("分页回绕")
		}
		lastOffset = int(offset)
		return page, nil
	})
	if err == nil || lastOffset != 64800 {
		t.Fatal("缺少偏移上限保护", lastOffset, err)
	}
}

func TestManageDoesNotInitializeGbbq(t *testing.T) {
	g := &tdx.Gbbq{}
	m := &tdx.Manage{Gbbq: g}
	s, err := pull.NewService(&pull.Config{Dir: t.TempDir(), Codes: []string{"sh000001"}, Manage: m})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, kind := range []string{"stock", "index", "etf"} {
		u := &stdUnit{kind: kind}
		if _, err := u.Manage(s); err != nil {
			t.Fatal(err)
		}
		if m.Gbbq != g || !g.IsEmpty() {
			t.Fatal("取连接源触发了股本初始化")
		}
	}
}
