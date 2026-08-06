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
// TDX 协议正值=卖盘未匹配（Flag=-1），负值=买盘未匹配（Flag=1）；
// 未匹配量是 4 字节有符号数，超过 32767 手不得被截断。
func TestCallAuctionDecodeUnmatchedSideAndWidth(t *testing.T) {
	payload := []byte{3, 0} // count = 3
	payload = append(payload,
		auctionRecord(9*60+15, 11.75, 725, -78604, 0)...)  // 负值：买盘未匹配，且 |值| > 32767
	payload = append(payload,
		auctionRecord(9*60+15, 11.7, 920, 414, 9)...)      // 正值：卖盘未匹配
	payload = append(payload,
		auctionRecord(9*60+24, 16.34, 5401, -21160, 57)...) // 涨停一字场景：买盘排队 21160 手

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
		{"负值大数应判买盘未匹配且不截断", resp.List[0], 1, 78604, 11.75},
		{"正值应判卖盘未匹配", resp.List[1], -1, 414, 11.7},
		{"涨停一字买盘排队", resp.List[2], 1, 21160, 16.34},
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
