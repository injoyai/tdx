package protocol

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestDecodeQuoteListCategory(t *testing.T) {
	empty := make([]byte, 602)
	binary.LittleEndian.PutUint16(empty[0:2], 0)
	items, err := MEx.DecodeQuoteList(empty, ExQuoteListCache{Category: 2})
	if err != nil || len(items) != 0 {
		t.Fatalf("零记录应返回空列表: %v, %d", err, len(items))
	}
	if _, err := MEx.DecodeQuoteList(empty, ExQuoteListCache{Category: 9}); err == nil ||
		!strings.Contains(err.Error(), "不支持") {
		t.Fatalf("不支持的类别未报错: %v", err)
	}
}

func TestQuoteProtocolUsesRoute(t *testing.T) {
	for _, tc := range []struct {
		code string
		want MarketProtocol
	}{
		{"000001", ProtocolStandard},
		{"us.AAPL", ProtocolExtended},
		{"00700.31", ProtocolExtended},
		// 港股共享代码的候选路由协议一致，选择扩展市场后由快照消歧。
		{"02800", ProtocolExtended},
	} {
		got, err := QuoteProtocol(tc.code)
		if err != nil || got != tc.want {
			t.Fatalf("QuoteProtocol(%q)=%q, %v", tc.code, got, err)
		}
	}
	if _, err := QuoteProtocol("00700.257"); err == nil {
		t.Fatal("越界编号未被拒绝")
	}
}
