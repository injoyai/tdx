package pull

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// 临时服务：构造最小 Service 用于存储测试。
var registerMockOnce sync.Once

func testService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	// 测试环境未注册内置市场，注册一个仅供测试的市场（多个测试共用，只注册一次）；
	// 用完整键 "test.xxx" 通过 ParseCode 路由到该市场
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	s, err := NewService(&Config{
		Dir:   dir,
		Codes: []string{"test.sh600000"},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// mockUnit 仅用于满足 NewService 的 Unit 非空校验。
type mockUnit struct{ name string }

func (u *mockUnit) Name() string { return u.name }
func (u *mockUnit) Codes(s *Service) ([]Code, error) {
	return nil, nil
}
func (u *mockUnit) FetchDay(s *Service, code Code) error {
	return nil
}
func (u *mockUnit) FetchMin(s *Service, code Code) error {
	return nil
}

func TestStoreDay(t *testing.T) {
	s := testService(t)
	code := Code{Market: MarketAStock, Code: "sh600000"}

	// 首次插入 3 根
	base := time.Date(2026, 1, 5, 15, 0, 0, 0, time.Local)
	ks := []*KlineDay{
		{Unix: base.Unix(), Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 100, Amount: 150},
		{Unix: base.AddDate(0, 0, 1).Unix(), Open: 1.5, High: 2.5, Low: 1, Close: 2, Volume: 200, Amount: 400},
		{Unix: base.AddDate(0, 0, 2).Unix(), Open: 2, High: 3, Low: 1.5, Close: 2.5, Volume: 300, Amount: 750},
	}
	if err := s.SaveDay(code, 0, ks); err != nil {
		t.Fatalf("SaveDay: %v", err)
	}

	last, err := s.LastDayUnix(code)
	if err != nil {
		t.Fatalf("LastDayUnix: %v", err)
	}
	if last != ks[2].Unix {
		t.Errorf("LastDayUnix = %d, want %d", last, ks[2].Unix)
	}

	// 增量：从 last 之后追加一根，模拟当天更新的最新 K 线
	next := &KlineDay{Unix: base.AddDate(0, 0, 3).Unix(), Open: 2.5, High: 3.5, Low: 2, Close: 3, Volume: 400, Amount: 1200}
	if err := s.SaveDay(code, last, []*KlineDay{next}); err != nil {
		t.Fatalf("SaveDay 增量: %v", err)
	}
	last2, err := s.LastDayUnix(code)
	if err != nil {
		t.Fatalf("LastDayUnix2: %v", err)
	}
	if last2 != next.Unix {
		t.Errorf("LastDayUnix 增量后 = %d, want %d", last2, next.Unix)
	}

	// 查询区间（第 3 根被增量替换为 base+3，库里共 3 根）
	dayFile := code.DayFile(s.Config().Dir)
	db, err := openDB(dayFile, new(KlineDay))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()
	ls, err := queryDay(db, base, base.AddDate(0, 0, 3))
	if err != nil {
		t.Fatalf("queryDay: %v", err)
	}
	if len(ls) != 3 {
		t.Fatalf("queryDay = %d 根, want 3", len(ls))
	}
	if ls[0].Unix != ks[0].Unix {
		t.Errorf("queryDay 升序: first = %d, want %d", ls[0].Unix, ks[0].Unix)
	}
}

func TestStoreDayUpsert(t *testing.T) {
	s := testService(t)
	code := Code{Market: MarketIndex, Code: "sh000001"}

	base := time.Date(2026, 1, 5, 15, 0, 0, 0, time.Local)
	ks := []*KlineDay{
		{Unix: base.Unix(), Open: 1, Close: 2, Volume: 100},
		{Unix: base.AddDate(0, 0, 1).Unix(), Open: 2, Close: 3, Volume: 200},
	}
	if err := s.SaveDay(code, 0, ks); err != nil {
		t.Fatalf("SaveDay: %v", err)
	}

	// 用 from=第一根 重新写入（幂等覆盖），数据量多一根且改动第一根
	ks2 := []*KlineDay{
		{Unix: base.Unix(), Open: 9, Close: 99, Volume: 999},
		{Unix: base.AddDate(0, 0, 1).Unix(), Open: 2, Close: 3, Volume: 200},
		{Unix: base.AddDate(0, 0, 2).Unix(), Open: 3, Close: 4, Volume: 300},
	}
	if err := s.SaveDay(code, ks[0].Unix, ks2); err != nil {
		t.Fatalf("SaveDay upsert: %v", err)
	}

	last, _ := s.LastDayUnix(code)
	if last != ks2[2].Unix {
		t.Errorf("upsert 后 LastDayUnix = %d, want %d", last, ks2[2].Unix)
	}
	dayFile := code.DayFile(s.Config().Dir)
	db, err := openDB(dayFile, new(KlineDay))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()
	ls, _ := queryDay(db, time.Time{}, time.Now().AddDate(0, 0, 10))
	if len(ls) != 3 {
		t.Fatalf("upsert 后 = %d 根, want 3", len(ls))
	}
	if ls[0].Open != 9 || ls[0].Close != 99 {
		t.Errorf("第一根未被覆盖: %+v", ls[0])
	}
}

func TestStoreMin(t *testing.T) {
	s := testService(t)
	code := Code{Market: MarketAStock, Code: "sz000001"}

	year := 2026
	base := time.Date(year, 1, 5, 9, 31, 0, 0, time.Local)
	ks := make([]*KlineMinute, 0, 3)
	for i := 0; i < 3; i++ {
		ks = append(ks, &KlineMinute{
			Unix:   base.Add(time.Duration(i) * time.Minute).Unix(),
			Open:   float64(i + 1),
			Close:  float64(i + 1),
			Volume: int64(i+1) * 100,
		})
	}
	if err := s.SaveMin(code, year, 0, ks); err != nil {
		t.Fatalf("SaveMin: %v", err)
	}

	last, err := s.LastMinUnix(code, year)
	if err != nil {
		t.Fatalf("LastMinUnix: %v", err)
	}
	if last != ks[2].Unix {
		t.Errorf("LastMinUnix = %d, want %d", last, ks[2].Unix)
	}

	if !s.MinExists(code, year) {
		t.Errorf("MinExists(%d) = false, want true", year)
	}
	if s.MinExists(code, year+1) {
		t.Errorf("MinExists(%d) = true, want false", year+1)
	}

	// 查询
	minFile := code.MinFile(s.Config().Dir, year)
	db, err := openDB(minFile, new(KlineMinute))
	if err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()
	ls, err := queryMin(db, base, base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("queryMin: %v", err)
	}
	if len(ls) != 3 {
		t.Fatalf("queryMin = %d 根, want 3", len(ls))
	}
}

func TestCodeFileLayout(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		code    Code
		dayFile string
		minFile string
	}{
		{Code{Market: MarketAStock, Code: "sh600000"}, "cn/stock/day/sh600000.db", "cn/stock/min/sh600000/sh600000-2026.db"},
		{Code{Market: MarketIndex, Code: "sh000001"}, "cn/index/day/sh000001.db", "cn/index/min/sh000001/sh000001-2026.db"},
		{Code{Market: MarketEtfLof, Code: "sh510300"}, "cn/etf/day/sh510300.db", "cn/etf/min/sh510300/sh510300-2026.db"},
		{Code{Market: MarketBlock, Code: "sh880001"}, "cn/block/day/sh880001.db", "cn/block/min/sh880001/sh880001-2026.db"},
		{Code{Market: MarketHK, Code: "00700"}, "hk/stock/day/00700.db", "hk/stock/min/00700/00700-2026.db"},
		{Code{Market: MarketHKIndex, Code: "HSI"}, "hk/index/day/HSI.db", "hk/index/min/HSI/HSI-2026.db"},
		{Code{Market: MarketUS, Code: "AAPL"}, "us/stock/day/AAPL.db", "us/stock/min/AAPL/AAPL-2026.db"},
		{Code{Market: MarketFuture, Code: "cff/IF2609"}, "cn/future/day/cff-IF2609.db", "cn/future/min/cff-IF2609/cff-IF2609-2026.db"},
	}
	for _, c := range cases {
		if got := c.code.DayFile(dir); got != filepath.Join(dir, c.dayFile) {
			t.Errorf("%s DayFile = %s, want %s", c.code.Key(), got, filepath.Join(dir, c.dayFile))
		}
		if got := c.code.MinFile(dir, 2026); got != filepath.Join(dir, c.minFile) {
			t.Errorf("%s MinFile = %s, want %s", c.code.Key(), got, filepath.Join(dir, c.minFile))
		}
	}
}

func TestSplitKey(t *testing.T) {
	cases := []struct {
		key    string
		market Market
		code   string
	}{
		{"US.AAPL", MarketUS, "AAPL"},
		{"HK00700", MarketHK, "00700"},
		{"cn/future.cff/IF2609", MarketFuture, "cff/IF2609"},
		{"cn/stock.sh600000", MarketAStock, "sh600000"},
		{"hk/index.HSI", MarketHKIndex, "HSI"},
		{"sh600000", "", "sh600000"}, // 无法识别市场，保留原样
	}
	for _, c := range cases {
		got := SplitKey(c.key)
		if got.Market != c.market || got.Code != c.code {
			t.Errorf("SplitKey(%q) = %+v, want market=%q code=%q", c.key, got, c.market, c.code)
		}
	}
}

func TestParseCode(t *testing.T) {
	cases := []struct {
		in     string
		market Market
		code   string
	}{
		// 沪深：带前缀
		{"sh600000", MarketAStock, "sh600000"},
		{"sz000001", MarketAStock, "sz000001"},
		{"sz300750", MarketAStock, "sz300750"},
		{"bj920001", MarketAStock, "bj920001"},
		{"SH600000", MarketAStock, "sh600000"}, // 大小写归一
		// 沪深：6 位裸数字（自动补前缀）
		{"600000", MarketAStock, "sh600000"},
		{"000001", MarketAStock, "sz000001"},
		{"300750", MarketAStock, "sz300750"},
		{"920001", MarketAStock, "bj920001"},
		// ETF/LOF
		{"sh510300", MarketEtfLof, "sh510300"},
		{"sz159915", MarketEtfLof, "sz159915"},
		{"510300", MarketEtfLof, "sh510300"},
		// 指数/板块
		{"sh000001", MarketIndex, "sh000001"},
		{"sz399001", MarketIndex, "sz399001"},
		{"399001", MarketIndex, "sz399001"},
		{"sh880001", MarketBlock, "sh880001"},
		{"880001", MarketBlock, "sh880001"},
		{"899050", MarketIndex, "bj899050"}, // 北交所指数，补不上交易所前缀，特判
		// 港股
		{"HK00700", MarketHK, "00700"},
		{"00700", MarketHK, "00700"},
		// 港股指数（白名单）
		{"HSI", MarketHKIndex, "HSI"},
		{"VHSI", MarketHKIndex, "VHSI"},
		// 美股
		{"AAPL", MarketUS, "AAPL"},
		// 期货
		{"cff/IF2609", MarketFuture, "cff/IF2609"},
		{"shf/cu2609", MarketFuture, "shf/cu2609"},
		// 完整键
		{"cn/future.cff/IF2609", MarketFuture, "cff/IF2609"},
		{"hk/stock.00700", MarketHK, "00700"},
		{"US.AAPL", MarketUS, "AAPL"},
		{"hk/index.HSI", MarketHKIndex, "HSI"},
		{"hk/index.CES120", MarketHKIndex, "CES120"},
	}
	for _, c := range cases {
		got, err := ParseCode(c.in)
		if err != nil {
			t.Errorf("ParseCode(%q) err: %v", c.in, err)
			continue
		}
		if got.Market != c.market || got.Code != c.code {
			t.Errorf("ParseCode(%q) = {%s, %s}, want {%s, %s}", c.in, got.Market, got.Code, c.market, c.code)
		}
	}
	// 无法识别
	for _, s := range []string{"", "1A0001", "abcdefg1", "XX600000", "12345678901"} {
		if _, err := ParseCode(s); err == nil {
			t.Errorf("ParseCode(%q) 应返回错误", s)
		}
	}
}

func TestDirCreation(t *testing.T) {
	dir := filepath.Join(os.TempDir(), "pull-test-"+t.Name())
	defer os.RemoveAll(dir)
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	s, err := NewService(&Config{
		Dir:   dir,
		Codes: []string{"test.sh600000"},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	defer s.Close()
	code := Code{Market: MarketAStock, Code: "sh600000"}
	if err := s.SaveDay(code, 0, []*KlineDay{{Unix: 1, Open: 1, Close: 1, Volume: 100}}); err != nil {
		t.Fatalf("SaveDay: %v", err)
	}
	// 地区/资产两级目录应自动创建
	if _, err := os.Stat(filepath.Join(dir, "cn", "stock", "day", "sh600000.db")); err != nil {
		t.Errorf("地区/资产/day 目录/文件未创建: %v", err)
	}
}

func TestQueryDay(t *testing.T) {
	s := testService(t)
	code := Code{Market: MarketAStock, Code: "sh600000"}

	// 未建库：返回空且不创建文件
	ls, err := s.QueryDay(code, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("QueryDay 空库: %v", err)
	}
	if len(ls) != 0 {
		t.Fatalf("QueryDay 空库 = %d 根, want 0", len(ls))
	}
	if _, err := os.Stat(code.DayFile(s.Config().Dir)); !os.IsNotExist(err) {
		t.Fatalf("QueryDay 空库不应创建文件")
	}

	// 写入后按范围查询
	base := time.Date(2026, 1, 5, 15, 0, 0, 0, time.Local)
	ks := []*KlineDay{
		{Unix: base.Unix(), Open: 1, Close: 1, Volume: 100},
		{Unix: base.AddDate(0, 0, 1).Unix(), Open: 2, Close: 2, Volume: 200},
		{Unix: base.AddDate(0, 0, 2).Unix(), Open: 3, Close: 3, Volume: 300},
	}
	if err := s.SaveDay(code, 0, ks); err != nil {
		t.Fatalf("SaveDay: %v", err)
	}

	// 全范围（零值端点不限制）
	ls, err = s.QueryDay(code, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("QueryDay: %v", err)
	}
	if len(ls) != 3 {
		t.Fatalf("QueryDay 全范围 = %d 根, want 3", len(ls))
	}
	// 半开区间
	ls, err = s.QueryDay(code, base.AddDate(0, 0, 1), time.Time{})
	if err != nil {
		t.Fatalf("QueryDay 半开: %v", err)
	}
	if len(ls) != 2 {
		t.Fatalf("QueryDay 半开 = %d 根, want 2", len(ls))
	}
}

func TestQueryMin(t *testing.T) {
	s := testService(t)
	code := Code{Market: MarketAStock, Code: "sz000001"}

	year := 2025
	base := time.Date(year, 1, 5, 9, 31, 0, 0, time.Local)
	ks := []*KlineMinute{}
	for i := 0; i < 3; i++ {
		ks = append(ks, &KlineMinute{
			Unix:   base.Add(time.Duration(i) * time.Minute).Unix(),
			Open:   float64(i + 1),
			Close:  float64(i + 1),
			Volume: int64(i+1) * 100,
		})
	}
	if err := s.SaveMin(code, year, 0, ks); err != nil {
		t.Fatalf("SaveMin: %v", err)
	}

	// 跨零值端点查询（自动定位年份文件）
	ls, err := s.QueryMin(code, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("QueryMin: %v", err)
	}
	if len(ls) != 3 {
		t.Fatalf("QueryMin 全范围 = %d 根, want 3", len(ls))
	}
	// 范围过滤
	ls, err = s.QueryMin(code, base, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("QueryMin 范围: %v", err)
	}
	if len(ls) != 2 {
		t.Fatalf("QueryMin 范围 = %d 根, want 2", len(ls))
	}
}

func TestEngineCache(t *testing.T) {
	s := testService(t)
	code := Code{Market: MarketAStock, Code: "sh600000"}

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
	if err := s.SaveDay(code, 0, []*KlineDay{{Unix: 1, Open: 1, Close: 1, Volume: 100}}); err != nil {
		t.Fatalf("SaveDay: %v", err)
	}
	last, err := s.LastDayUnix(code)
	if err != nil {
		t.Fatalf("LastDayUnix: %v", err)
	}
	if last != 1 {
		t.Fatalf("LastDayUnix = %d, want 1", last)
	}
}

func TestSaveEmptySkipsWrite(t *testing.T) {
	s := testService(t)
	code := Code{Market: MarketAStock, Code: "sh600000"}

	// 空数据写入应跳过，不创建库文件
	if err := s.SaveDay(code, 0, nil); err != nil {
		t.Fatalf("SaveDay 空数据: %v", err)
	}
	if _, err := os.Stat(code.DayFile(s.Config().Dir)); !os.IsNotExist(err) {
		t.Fatalf("空数据不应创建文件")
	}
}

// TestInsertBatchOverLimit 回归测试：批量插入超过 sqlite 占位符上限
// （999 变量）时自动分批，不报 "too many SQL variables"。
func TestInsertBatchOverLimit(t *testing.T) {
	s := testService(t)
	code := Code{Market: MarketAStock, Code: "sh600000"}

	// 200 根日线 × 10 字段 = 2000 变量 > 999，必然触发分批
	base := time.Date(2026, 1, 5, 15, 0, 0, 0, time.Local)
	ks := make([]*KlineDay, 0, 200)
	for i := 0; i < 200; i++ {
		ks = append(ks, &KlineDay{
			Unix:   base.AddDate(0, 0, i).Unix(),
			Open:   float64(i),
			Close:  float64(i),
			Volume: int64(i),
		})
	}
	if err := s.SaveDay(code, 0, ks); err != nil {
		t.Fatalf("SaveDay 超限批量: %v", err)
	}

	// 数据完整：200 根全部落库
	got, err := s.QueryDay(code, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("QueryDay: %v", err)
	}
	if len(got) != 200 {
		t.Fatalf("落库数量 = %d, want 200", len(got))
	}
	if got[0].Unix != ks[0].Unix || got[199].Unix != ks[199].Unix {
		t.Errorf("首尾时间戳不符: got %d..%d, want %d..%d",
			got[0].Unix, got[199].Unix, ks[0].Unix, ks[199].Unix)
	}
}
