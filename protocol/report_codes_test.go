package protocol

import (
	"strings"
	"testing"
)

func TestParseReportCodes(t *testing.T) {
	files := map[string][]byte{
		"first.cfg":  []byte("44|920000|2|First|1|\n"),
		"second.cfg": []byte("44|920001|2|Second|1|\n"),
	}
	for _, tc := range []struct {
		member string
		code   string
	}{
		{"first.cfg", "920000"},
		{"second.cfg", "920001"},
	} {
		got, err := ParseReportCodes(CodeReport{Member: tc.member, Parser: ReportTdxBjMore}, files)
		if err != nil || len(got) != 1 || got[0].Code != tc.code {
			t.Fatalf("%s: %+v, %v", tc.member, got, err)
		}
	}
	if _, err := ParseReportCodes(CodeReport{Member: "missing.cfg", Parser: ReportTdxBjMore}, files); err == nil || !strings.Contains(err.Error(), "missing.cfg") {
		t.Fatal("缺少成员未报错", err)
	}
	if _, err := ParseReportCodes(CodeReport{Member: "first.cfg", Parser: "unknown"}, files); err == nil {
		t.Fatal("未知解析器未报错")
	}
}
