package pull

import (
	"os"
	"strings"
	"testing"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestMarketPrefixForms(t *testing.T) {
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	for _, tt := range []struct {
		inputs []string
		market protocol.Market
		code   string
	}{
		{[]string{"sh600000", "sh.600000", "SH.600000"}, protocol.MarketAStock, "sh600000"},
		{[]string{"sz000001", "Sz.000001"}, protocol.MarketAStock, "sz000001"},
		{[]string{"000001.SZ", "000001.sz"}, protocol.MarketAStock, "sz000001"},
		{[]string{"上海600000", "600000.SH"}, protocol.MarketAStock, "sh600000"},
		{[]string{"bj920001", "BJ.920001"}, protocol.MarketAStock, "bj920001"},
		{[]string{"sh000001", "SH.000001"}, protocol.MarketIndex, "sh000001"},
		{[]string{"sz399001", "sz.399001"}, protocol.MarketIndex, "sz399001"},
		{[]string{"bj899050", "bj.899050"}, protocol.MarketIndex, "bj899050"},
		{[]string{"sh510300", "sh.510300"}, protocol.MarketEtfLof, "sh510300"},
		{[]string{"sh880001", "sh.880001"}, protocol.MarketBlock, "sh880001"},
		{[]string{"hk00700", "HK00700", "hk.00700", "HK.00700"}, protocol.MarketHK, "00700"},
		{[]string{"hkHSI", "hk.HSI", "HK.HSI", "Hk.hsi", " hk.HSI "}, protocol.MarketHKIndex, "HSI"},
		{[]string{"hkCES120", "HK.CES120"}, protocol.MarketHKIndex, "CES120"},
		{[]string{"hk.HZ5021", "hkHZ5021"}, protocol.MarketHKIndex, "HZ5021"},
		{[]string{"usAAPL", "US.AAPL", "Us.aapl"}, protocol.MarketUS, "AAPL"},
		{[]string{"usBRK.B", "us.BRK.B", "US.BRK.B", "BRK.B", "brk.b"}, protocol.MarketUS, "BRK.B"},
		{[]string{"usABC1", "us.ABC1", "US.abc1"}, protocol.MarketUS, "ABC1"},
		{[]string{"cffIF2609", "CFF.IF2609", "cff/IF2609"}, protocol.MarketFuture, "cff/IF2609"},
		{[]string{"czcSR2609", "CZC.SR2609"}, protocol.MarketFuture, "czc/SR2609"},
		{[]string{"dceA2609", "DCE.A2609"}, protocol.MarketFuture, "dce/A2609"},
		{[]string{"shfcu2609", "SHF.cu2609"}, protocol.MarketFuture, "shf/cu2609"},
		{[]string{"gfeLC2609", "GFE.LC2609"}, protocol.MarketFuture, "gfe/LC2609"},
		{[]string{"qhzT001", "QHZ.T001"}, protocol.MarketFuture, "qhz/T001"},
		{[]string{"CN/STOCK.SH600000", "cn/stocksh600000"}, protocol.MarketAStock, "sh600000"},
		{[]string{"CN/INDEX.SH000001", "cn/indexsh000001"}, protocol.MarketIndex, "sh000001"},
		{[]string{"CN/ETF.SH510300", "cn/etfsh510300"}, protocol.MarketEtfLof, "sh510300"},
		{[]string{"CN/BLOCK.SH880001", "cn/blocksh880001"}, protocol.MarketBlock, "sh880001"},
		{[]string{"HK/STOCK.00700", "hk/stock00700"}, protocol.MarketHK, "00700"},
		{[]string{"HK/INDEX.HSI", "hk/indexHSI"}, protocol.MarketHKIndex, "HSI"},
		{[]string{"US/STOCK.AAPL", "us/stockAAPL"}, protocol.MarketUS, "AAPL"},
		{[]string{"CN/FUTURE.CFF/IF2609", "cn/futurecff/IF2609"}, protocol.MarketFuture, "cff/IF2609"},
		{[]string{"TeSt.one", "TESTone"}, "test", "one"},
	} {
		for _, input := range tt.inputs {
			t.Run(input, func(t *testing.T) {
				code := tt.code
				got, err := ParseCode(input)
				if err != nil || got.Market != tt.market || got.Code != code {
					t.Fatalf("ParseCode(%q)=%+v, %v; want %s %s", input, got, err, tt.market, code)
				}
			})
		}
	}
}

func TestBuiltinParsersAgree(t *testing.T) {
	for _, s := range []string{"000001.SZ", "BRK.B", "hkHSI", "上海600000", "sh.600000", "us.BRK.B", "cff/IF2609", "899050", "hk.CES120"} {
		i, err := protocol.ParseInstrument(s)
		if err != nil {
			t.Fatal(s, err)
		}
		ex, body, err := protocol.DecodeCode(s)
		if err != nil || ex != i.Exchange || body != i.Code {
			t.Fatalf("DecodeCode(%q)=%s/%s, %v; Instrument=%s", s, ex, body, err, i)
		}
		c, err := ParseCode(s)
		if err != nil {
			t.Fatal(s, err)
		}
		market, _ := i.Market()
		if c.Market != market {
			t.Fatalf("ParseCode(%q) market=%s, want %s", s, c.Market, market)
		}
	}
	if _, err := ParseCode("unknown.opaque-code"); err == nil {
		t.Fatal("unregistered market silently accepted")
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
		if err != nil || got.Market != protocol.Market(name) || got.Code != "one" {
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
		if err != nil || code.Market != protocol.MarketUS || code.Code != input {
			t.Fatal("误拆美股代码", input, code, err)
		}
	}
	if code, err := ParseCode("HKL"); err != nil || code.Market != protocol.MarketHKIndex || code.Code != "HKL" {
		t.Fatal("误拆裸港股指数", code, err)
	}
	for _, input := range []string{"hk.", "us.", "sh.", "sh.HSI", "hk..HSI", "hk.0070", "us..AAPL", "cff.", "hk/index."} {
		if _, err := ParseCode(input); err == nil {
			t.Fatal("非法显式市场代码被接受", input)
		}
	}
}

func TestBuiltinCodesUseProtocolValidation(t *testing.T) {
	for _, input := range []string{
		"us.12345", "us.ABCDEFGHIJ", "ABCDEFGHIJ", "us.AAPL-", "hk.ABCDEFGHIJ", "cff.12345", "cff.ABCDEFGHI1",
		"cn/stock.hk00700", "cn/index.sh600000", "cn/stock.sh110001", "hk/stock.HSI", "hk/index.00700",
		"us/stock.12345", "us/stock.ABCDEFGHIJ", "cn/future.us.AAPL", "cn/future.cff/12345", "unknown/IF2609",
	} {
		t.Run(input, func(t *testing.T) {
			if got, err := ParseCode(input); err == nil {
				t.Fatal("invalid builtin accepted", got)
			}
		})
	}
	got, err := ParseCode("CN/STOCK.600000")
	if err != nil || got.Market != protocol.MarketAStock || got.FileName() != "sh600000" {
		t.Fatal("market code was not normalized", got, err)
	}
	registerMockOnce.Do(func() { Register(&mockUnit{name: "test"}) })
	got, err = ParseCode("test.arbitrary-opaque-code")
	if err != nil || got.Code != "arbitrary-opaque-code" {
		t.Fatal("custom market validation changed", got, err)
	}
}

func TestInvalidBuiltinFailsBeforeCreatingDatabase(t *testing.T) {
	dir := t.TempDir()
	for _, input := range []string{"us.12345", "us.ABCDEFGHIJ", "cn/stock.hk00700"} {
		s, err := New(&Config{Dir: dir, Codes: []string{input}})
		if err == nil {
			s.Close()
			t.Fatal("invalid input initialized service", input)
		}
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatal("invalid codes created files", files, err)
	}
}
