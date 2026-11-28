package model

import (
	"testing"
	"time"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestStandardCodeUsesRouteProtocol(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"600000", "sh600000"},
		{"sh.600000", "sh600000"},
		{"600000.SH", "sh600000"},
		{"000001", "sz000001"},
	} {
		got, err := StandardCode(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("StandardCode(%q)=%q, %v", tc.in, got, err)
		}
	}
	for _, in := range []string{"us.AAPL", "00700.31", "cff.IF2609"} {
		if _, err := StandardCode(in); err == nil {
			t.Fatalf("扩展市场 %q 不应进入标准元数据缓存", in)
		}
	}
}

func TestCodeSnapshotRejectsExtendedAndMixedRows(t *testing.T) {
	now := time.Now()
	s := CodeSnapshot{
		Exchanges: []protocol.Exchange{protocol.ExchangeUS},
		Rows:      []*CodeModel{{Name: "苹果", Code: "AAPL", Exchange: "us"}},
		FetchedAt: now,
	}
	if err := s.Validate(); err == nil {
		t.Fatal("扩展市场不应作为标准代码快照范围")
	}
	s = CodeSnapshot{
		Exchanges: []protocol.Exchange{protocol.ExchangeSH},
		Rows:      []*CodeModel{{Name: "苹果", Code: "AAPL", Exchange: "us"}},
		FetchedAt: now,
	}
	if err := s.Validate(); err == nil {
		t.Fatal("标准快照不应接受扩展市场记录")
	}
	// 规范简称由解析结果比对，不再固定切两个字符。
	if err := ValidateCodes(CodeModels{{Name: "浦发银行", Code: "600000", Exchange: "sh"}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCodes(CodeModels{{Name: "浦发银行", Code: "600000", Exchange: "SH"}}); err == nil {
		t.Fatal("非规范简称应被拒绝")
	}
}
