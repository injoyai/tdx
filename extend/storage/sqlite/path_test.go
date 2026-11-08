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
		{model.Code{Market: protocol.MarketAStock, Code: "sh600000"}, "cn/stock/day/sh600000.db", "cn/stock/min/sh600000/sh600000-2026.db"},
		{model.Code{Market: protocol.MarketIndex, Code: "sh000001"}, "cn/index/day/sh000001.db", "cn/index/min/sh000001/sh000001-2026.db"},
		{model.Code{Market: protocol.MarketEtfLof, Code: "sh510300"}, "cn/etf/day/sh510300.db", "cn/etf/min/sh510300/sh510300-2026.db"},
		{model.Code{Market: protocol.MarketBlock, Code: "sh880001"}, "cn/block/day/sh880001.db", "cn/block/min/sh880001/sh880001-2026.db"},
		{model.Code{Market: protocol.MarketHK, Code: "00700"}, "hk/stock/day/00700.db", "hk/stock/min/00700/00700-2026.db"},
		{model.Code{Market: protocol.MarketHKIndex, Code: "HSI"}, "hk/index/day/HSI.db", "hk/index/min/HSI/HSI-2026.db"},
		{model.Code{Market: protocol.MarketUS, Code: "AAPL"}, "us/stock/day/AAPL.db", "us/stock/min/AAPL/AAPL-2026.db"},
		{model.Code{Market: protocol.MarketFuture, Code: "cff/IF2609"}, "cn/future/day/cff-IF2609.db", "cn/future/min/cff-IF2609/cff-IF2609-2026.db"},
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
