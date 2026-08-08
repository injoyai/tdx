package protocol

import (
	"math"
	"testing"
)

// 记录布局：时间(2 字节, 小时*60+分钟) 价格(4 float32*1000) 匹配量(4) 未匹配量(4 int32) padding(1) 秒(1)
func auctionRecord(minute uint16, price float32, match uint32, unmatched int32, sec byte) []byte {
	b := make([]byte, 16)
	b[0], b[1] = byte(minute), byte(minute>>8)
	u32 := math.Float32bits(price)
	b[2], b[3], b[4], b[5] = byte(u32), byte(u32>>8), byte(u32>>16), byte(u32>>24)
	b[6], b[7], b[8], b[9] = byte(match), byte(match>>8), byte(match>>16), byte(match>>24)
	u := uint32(unmatched)
	b[10], b[11], b[12], b[13] = byte(u), byte(u>>8), byte(u>>16), byte(u>>24)
	b[15] = sec
	return b
}

// TestCallAuctionDecodeUnmatchedSideAndWidth 验证集合竞价解码的未匹配方向与 4 字节位宽：
// TDX 协议正值=买盘未匹配（Flag=1），负值=卖盘未匹配（Flag=-1）；
// 未匹配量是 4 字节有符号数，超过 32767 手不得被截断。
// 用例来自 2026-08-06 实盘：*ST大立 002214 涨停一字 09:24:57 买盘排队 109912 手，
// 旧实现读低 2 字节 0xAD58 = -21160，方向被误判为卖。
func TestCallAuctionDecodeUnmatchedSideAndWidth(t *testing.T) {
	payload := []byte{3, 0} // count = 3
	payload = append(payload,
		auctionRecord(9*60+15, 11.75, 725, -78604, 0)...)  // 负值：卖盘未匹配，且 |值| > 32767
	payload = append(payload,
		auctionRecord(9*60+15, 11.7, 920, 414, 9)...)      // 正值：买盘未匹配
	payload = append(payload,
		auctionRecord(9*60+24, 16.34, 5401, 109912, 57)...) // 涨停一字场景：买盘排队 109912 手

	resp, err := MCallAuction.Decode(payload)
	if err != nil {
		t.Fatalf("Decode 失败: %v", err)
	}
	if resp.Count != 3 {
		t.Fatalf("Count = %d, want 3", resp.Count)
	}
	if len(resp.List) != 3 {
		t.Fatalf("len(List) = %d, want 3", len(resp.List))
	}

	cases := []struct {
		name      string
		item      *CallAuction
		wantFlag  int8
		wantUn    int64
		wantPrice float64
	}{
		{"负值大数应判卖盘未匹配且不截断", resp.List[0], -1, 78604, 11.75},
		{"正值应判买盘未匹配", resp.List[1], 1, 414, 11.7},
		{"涨停一字买盘排队109912", resp.List[2], 1, 109912, 16.34},
	}
	for _, c := range cases {
		if c.item.Flag != c.wantFlag {
			t.Errorf("%s: Flag = %d, want %d", c.name, c.item.Flag, c.wantFlag)
		}
		if c.item.Unmatched != c.wantUn {
			t.Errorf("%s: Unmatched = %d, want %d（旧实现只读低 2 字节会截断）", c.name, c.item.Unmatched, c.wantUn)
		}
		if got := float64(c.item.Price) / 1000; got != c.wantPrice {
			t.Errorf("%s: Price = %v, want %v", c.name, got, c.wantPrice)
		}
	}

	if got := resp.List[0].Time.Hour()*60 + resp.List[0].Time.Minute(); got != 9*60+15 {
		t.Errorf("时间解析 = %d, want %d", got, 9*60+15)
	}
	if got := resp.List[2].Time.Second(); got != 57 {
		t.Errorf("秒解析 = %d, want 57", got)
	}
	if got := resp.List[0].Match; got != 725 {
		t.Errorf("匹配量 = %d, want 725", got)
	}
}

// TestCallAuctionDecodeInt16TruncationFlipsSign 验证旧实现（只读低 2 字节 int16）在
// 未匹配量超过 32767 手时把正数截断成负数、方向翻转的问题，新实现读 4 字节不受影响：
//   - 真实 +109912（买盘未匹配 0x0001AD58）低 16 位 0xAD58 = -21160，旧代码会误判成卖 21160；
//   - 真实 +30000（买盘未匹配）未超 32767，低 16 位符号不变，旧代码方向仍正确但只拿到低 16 位。
func TestCallAuctionDecodeInt16TruncationFlipsSign(t *testing.T) {
	payload := []byte{1, 0}
	payload = append(payload, auctionRecord(9*60+24, 16.34, 5401, 109912, 57)...)

	resp, err := MCallAuction.Decode(payload)
	if err != nil {
		t.Fatalf("Decode 失败: %v", err)
	}
	item := resp.List[0]
	if item.Unmatched != 109912 {
		t.Errorf("Unmatched = %d, want 109912（int16 截断会读成 -21160）", item.Unmatched)
	}
	if item.Flag != 1 {
		t.Errorf("Flag = %d, want 1（协议正值=买盘未匹配；int16 截断低 16 位 0xAD58 为负，旧代码会误判成卖）", item.Flag)
	}

	// 真实 +30000（买盘未匹配），未超 32767 不会截断符号，数值也不丢
	payload2 := []byte{1, 0}
	payload2 = append(payload2, auctionRecord(9*60+24, 10.0, 100, 30000, 57)...)
	resp2, err := MCallAuction.Decode(payload2)
	if err != nil {
		t.Fatalf("Decode 失败: %v", err)
	}
	if resp2.List[0].Unmatched != 30000 || resp2.List[0].Flag != 1 {
		t.Errorf("+30000 应解为买盘未匹配 30000（实际 unmatched=%d flag=%d）", resp2.List[0].Unmatched, resp2.List[0].Flag)
	}
}
