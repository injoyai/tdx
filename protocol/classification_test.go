package protocol

import "testing"

// TestClassificationAcceptsMarketQualifiedForms 保证同一标准证券的带市场标识写法分类一致。
func TestClassificationAcceptsMarketQualifiedForms(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		stock, etf, index bool
	}{
		{"sh000001", "sh.000001", false, false, true},
		{"sz399001", "399001.SZ", false, false, true},
		{"sz000001", "sz.000001", true, false, false},
		{"sh600000", "600000.SH", true, false, false},
		{"sz159558", "sz.159558", false, true, false},
	} {
		if IsStock(tc.a) != tc.stock || IsStock(tc.b) != tc.stock {
			t.Fatalf("IsStock(%s/%s)", tc.a, tc.b)
		}
		if IsETF(tc.a) != tc.etf || IsETF(tc.b) != tc.etf {
			t.Fatalf("IsETF(%s/%s)", tc.a, tc.b)
		}
		if IsIndex(tc.a) != tc.index || IsIndex(tc.b) != tc.index {
			t.Fatalf("IsIndex(%s/%s)", tc.a, tc.b)
		}
	}
	// 裸 6 位代码仍不在公开支持范围内（与既有契约一致）。
	for _, code := range []string{"880741", "881101", "000001"} {
		if IsIndex(code) {
			t.Fatalf("IsIndex(%q) 应要求市场标识", code)
		}
	}
	// 扩展市场基金不能被当作 ETF。
	if IsETF("00823.49") {
		t.Fatal("港股基金不应视为 ETF")
	}
}
