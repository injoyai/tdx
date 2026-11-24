package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestCodeFileLayout(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		code    model.Code
		dayFile string
		minFile string
	}{
		{model.Code{Market: protocol.MarketSH, Code: "sh600000"}, "sh/day/sh600000.db", "sh/min/sh600000/sh600000-2026.db"},
		{model.Code{Market: protocol.MarketSH, Code: "sh000001"}, "sh/day/sh000001.db", "sh/min/sh000001/sh000001-2026.db"},
		{model.Code{Market: protocol.MarketSH, Code: "sh510300"}, "sh/day/sh510300.db", "sh/min/sh510300/sh510300-2026.db"},
		{model.Code{Market: protocol.MarketSH, Code: "sh880001"}, "sh/day/sh880001.db", "sh/min/sh880001/sh880001-2026.db"},
		{model.Code{Market: protocol.MarketSZ, Code: "sz000001"}, "sz/day/sz000001.db", "sz/min/sz000001/sz000001-2026.db"},
		{model.Code{Market: protocol.MarketBJ, Code: "bj899050"}, "bj/day/bj899050.db", "bj/min/bj899050/bj899050-2026.db"},
		{model.Code{Market: protocol.MarketHK, Code: "00700"}, "hk/stock/day/00700.db", "hk/stock/min/00700/00700-2026.db"},
		{model.Code{Market: protocol.MarketHKIndex, Code: "HSI"}, "hk/index/day/~48~53~49.db", "hk/index/min/~48~53~49/~48~53~49-2026.db"},
		{model.Code{Market: protocol.MarketUS, Code: "AAPL"}, "us/stock/day/~41~41~50~4c.db", "us/stock/min/~41~41~50~4c/~41~41~50~4c-2026.db"},
		{model.Code{Market: protocol.MarketCFF, Code: "IF2609"}, "cff/day/~49~462609.db", "cff/min/~49~462609/~49~462609-2026.db"},
	}
	for _, c := range cases {
		if got := dayFile(c.code, dir); got != filepath.Join(dir, c.dayFile) {
			t.Errorf("%s DayFile = %s, want %s", c.code.Key(), got, filepath.Join(dir, c.dayFile))
		}
		if got := minuteFile(c.code, dir, 2026); got != filepath.Join(dir, c.minFile) {
			t.Errorf("%s MinuteFile = %s, want %s", c.code.Key(), got, filepath.Join(dir, c.minFile))
		}
	}
}

func TestQueryMinuteWithLiteralBracketsInDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "archive [old]")
	s, err := NewPull(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	code := model.Code{Market: protocol.MarketSZ, Code: "sz000001"}
	old := "2025-12-31 14:59"
	newer := "2026-01-05 09:31"
	if err := s.SaveMinute(code, 2026, newer, []*model.KlineMinute{{Time: newer}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMinute(code, 2025, old, []*model.KlineMinute{{Time: old}}, nil); err != nil {
		t.Fatal(err)
	}
	got, err := s.QueryMinute(code, "", "")
	if err != nil || len(got) != 2 || got[0].Time != old || got[1].Time != newer {
		t.Fatalf("query returned %+v, %v, want 2 rows in year order", got, err)
	}
}
