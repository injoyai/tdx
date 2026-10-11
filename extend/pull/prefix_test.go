package pull

import (
	"strings"
	"testing"
)

func TestMarketPrefixForms(t *testing.T) {
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	for _, tt := range []struct {
		inputs []string
		market Market
		code   string
	}{
		{[]string{"sh600000", "sh.600000", "SH.600000"}, MarketAStock, "sh600000"},
		{[]string{"sz000001", "Sz.000001"}, MarketAStock, "sz000001"},
		{[]string{"bj920001", "BJ.920001"}, MarketAStock, "bj920001"},
		{[]string{"sh000001", "SH.000001"}, MarketIndex, "sh000001"},
		{[]string{"sz399001", "sz.399001"}, MarketIndex, "sz399001"},
		{[]string{"bj899050", "bj.899050"}, MarketIndex, "bj899050"},
		{[]string{"sh510300", "sh.510300"}, MarketEtfLof, "sh510300"},
		{[]string{"sh880001", "sh.880001"}, MarketBlock, "sh880001"},
		{[]string{"hk00700", "HK00700", "hk.00700", "HK.00700"}, MarketHK, "00700"},
		{[]string{"hkHSI", "hk.HSI", "HK.HSI", "Hk.hsi", " hk.HSI "}, MarketHKIndex, "HSI"},
		{[]string{"hkCES120", "HK.CES120"}, MarketHKIndex, "CES120"},
		{[]string{"hk.HZ5021", "hkHZ5021"}, MarketHKIndex, "HZ5021"},
		{[]string{"usAAPL", "US.AAPL", "Us.aapl"}, MarketUS, "AAPL"},
		{[]string{"usBRK.B", "us.BRK.B", "US.BRK.B"}, MarketUS, "BRK.B"},
		{[]string{"usABC1", "us.ABC1", "US.abc1"}, MarketUS, "ABC1"},
		{[]string{"cffIF2609", "CFF.IF2609", "cff/IF2609"}, MarketFuture, "cff/IF2609"},
		{[]string{"czcSR2609", "CZC.SR2609"}, MarketFuture, "czc/SR2609"},
		{[]string{"dceA2609", "DCE.A2609"}, MarketFuture, "dce/A2609"},
		{[]string{"shfcu2609", "SHF.cu2609"}, MarketFuture, "shf/cu2609"},
		{[]string{"gfeLC2609", "GFE.LC2609"}, MarketFuture, "gfe/LC2609"},
		{[]string{"qhzT001", "QHZ.T001"}, MarketFuture, "qhz/T001"},
		{[]string{"CN/STOCK.SH600000", "cn/stocksh600000"}, MarketAStock, "sh600000"},
		{[]string{"CN/INDEX.SH000001", "cn/indexsh000001"}, MarketIndex, "sh000001"},
		{[]string{"CN/ETF.SH510300", "cn/etfsh510300"}, MarketEtfLof, "sh510300"},
		{[]string{"CN/BLOCK.SH880001", "cn/blocksh880001"}, MarketBlock, "sh880001"},
		{[]string{"HK/STOCK.00700", "hk/stock00700"}, MarketHK, "00700"},
		{[]string{"HK/INDEX.HSI", "hk/indexHSI"}, MarketHKIndex, "HSI"},
		{[]string{"US/STOCK.AAPL", "us/stockAAPL"}, MarketUS, "AAPL"},
		{[]string{"CN/FUTURE.CFF/IF2609", "cn/futurecff/IF2609"}, MarketFuture, "cff/IF2609"},
		{[]string{"TeSt.one", "TESTone"}, "test", "one"},
	} {
		for _, input := range tt.inputs {
			t.Run(input, func(t *testing.T) {
				code := tt.code
				got, err := ParseCode(input)
				if err != nil || got.Market != tt.market || got.Code != code {
					t.Fatalf("ParseCode(%q)=%+v, %v; want %s %s", input, got, err, tt.market, code)
				}
				if key := SplitKey(input); key != got {
					t.Fatal("SplitKey 与 ParseCode 的市场规则不一致", key, got)
				}
			})
		}
	}
}

func TestMarketRegistryIsCaseInsensitive(t *testing.T) {
	name := "CaseMarket"
	u := &mockUnit{name: name}
	Register(u)
	t.Cleanup(func() {
		registry.Lock()
		delete(registry.units, name)
		registry.Unlock()
	})
	if got, ok := Get(strings.ToLower(name)); !ok || got != u {
		t.Fatal("市场查找区分了大小写")
	}
	for _, input := range []string{"casemarket.one", "CASEMARKETone"} {
		got, err := ParseCode(input)
		if err != nil || got.Market != Market(name) || got.Code != "one" {
			t.Fatal("自定义市场未保留注册名称", got, err)
		}
	}
	defer func() {
		if recover() == nil {
			t.Fatal("允许仅大小写不同的重复市场注册")
		}
	}()
	Register(&mockUnit{name: strings.ToUpper(name)})
}

func TestRegisterRejectsReservedMarketAlias(t *testing.T) {
	for _, name := range []string{"hk", "US", "ShF"} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("自定义市场占用了内置简称")
				}
			}()
			Register(&mockUnit{name: name})
		})
	}
}

func TestMarketPrefixDoesNotStripBareSymbols(t *testing.T) {
	for _, input := range []string{"SHOP", "SHY", "BJK", "SZL"} {
		code, err := ParseCode(input)
		if err != nil || code.Market != MarketUS || code.Code != input {
			t.Fatal("误拆美股代码", input, code, err)
		}
	}
	if code, err := ParseCode("HKL"); err != nil || code.Market != MarketHKIndex || code.Code != "HKL" {
		t.Fatal("误拆裸港股指数", code, err)
	}
	for _, input := range []string{"hk.", "us.", "sh.", "sh.HSI", "hk..HSI", "hk.0070", "us..AAPL", "cff.", "hk/index."} {
		if _, err := ParseCode(input); err == nil {
			t.Fatal("非法显式市场代码被接受", input)
		}
	}
}
