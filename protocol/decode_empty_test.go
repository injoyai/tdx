package protocol

import (
	"testing"
)

// 截断报文（<2字节）必须返回 error
// count==0 的合法空报文必须返回空结果且 nil error

func TestMinute_Decode_Truncated(t *testing.T) {
	m := &minute{}
	_, err := m.Decode([]byte{0x01})
	if err == nil {
		t.Fatal("expected error for truncated payload")
	}
}

func TestMinute_Decode_Empty(t *testing.T) {
	m := &minute{}
	resp, err := m.Decode([]byte{0x00, 0x00})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 0 || len(resp.List) != 0 {
		t.Fatalf("expected empty resp, got count=%d list=%d", resp.Count, len(resp.List))
	}
}

func TestHistoryMinute_Decode_Truncated(t *testing.T) {
	_, err := MHistoryMinute.Decode([]byte{0x01})
	if err == nil {
		t.Fatal("expected error for truncated payload")
	}
}

func TestHistoryMinute_Decode_Empty(t *testing.T) {
	resp, err := MHistoryMinute.Decode([]byte{0x00, 0x00})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 0 || len(resp.List) != 0 {
		t.Fatalf("expected empty resp, got count=%d list=%d", resp.Count, len(resp.List))
	}
}

func TestHistoryMinute_Decode_CountNonZeroButShort(t *testing.T) {
	// count=1 但总长度只有 4 字节（不足 6），应报错
	_, err := MHistoryMinute.Decode([]byte{0x01, 0x00, 0x00, 0x00})
	if err == nil {
		t.Fatal("expected error for count>0 but insufficient header")
	}
}

func TestKline_Decode_Truncated(t *testing.T) {
	_, err := MKline.Decode([]byte{0x01}, KlineCache{})
	if err == nil {
		t.Fatal("expected error for truncated payload")
	}
}

func TestKline_Decode_Empty(t *testing.T) {
	resp, err := MKline.Decode([]byte{0x00, 0x00}, KlineCache{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 0 {
		t.Fatalf("expected count=0, got %d", resp.Count)
	}
}

func TestTrade_Decode_Truncated(t *testing.T) {
	_, err := MTrade.Decode([]byte{0x01}, TradeCache{})
	if err == nil {
		t.Fatal("expected error for truncated payload")
	}
}

func TestTrade_Decode_Empty(t *testing.T) {
	resp, err := MTrade.Decode([]byte{0x00, 0x00}, TradeCache{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 0 {
		t.Fatalf("expected count=0, got %d", resp.Count)
	}
}

func TestHistoryTrade_Decode_Truncated(t *testing.T) {
	_, err := MHistoryTrade.Decode([]byte{0x01}, TradeCache{})
	if err == nil {
		t.Fatal("expected error for truncated payload")
	}
}

func TestHistoryTrade_Decode_Empty(t *testing.T) {
	resp, err := MHistoryTrade.Decode([]byte{0x00, 0x00}, TradeCache{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 0 {
		t.Fatalf("expected count=0, got %d", resp.Count)
	}
}

func TestCallAuction_Decode_Truncated(t *testing.T) {
	_, err := MCallAuction.Decode([]byte{0x01})
	if err == nil {
		t.Fatal("expected error for truncated payload")
	}
}

func TestCallAuction_Decode_Empty(t *testing.T) {
	resp, err := MCallAuction.Decode([]byte{0x00, 0x00})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 0 {
		t.Fatalf("expected count=0, got %d", resp.Count)
	}
}

func TestCode_Decode_Truncated(t *testing.T) {
	_, err := MCode.Decode([]byte{0x01})
	if err == nil {
		t.Fatal("expected error for truncated payload")
	}
}

func TestCode_Decode_Empty(t *testing.T) {
	resp, err := MCode.Decode([]byte{0x00, 0x00})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 0 {
		t.Fatalf("expected count=0, got %d", resp.Count)
	}
}

func TestCount_Decode_Truncated(t *testing.T) {
	_, err := MCount.Decode([]byte{0x01})
	if err == nil {
		t.Fatal("expected error for truncated payload")
	}
}

func TestGbbq_Decode_Truncated(t *testing.T) {
	_, err := MGbbq.Decode([]byte{0x01, 0x02, 0x03})
	if err == nil {
		t.Fatal("expected error for truncated payload")
	}
}

func TestGbbq_Decode_Empty(t *testing.T) {
	// 11 字节头，count 在 offset 9-10，设为 0
	bs := make([]byte, 11)
	resp, err := MGbbq.Decode(bs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Count != 0 {
		t.Fatalf("expected count=0, got %d", resp.Count)
	}
}
