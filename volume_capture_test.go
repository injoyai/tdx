package tdx

import (
	"encoding/binary"
	"encoding/hex"
	"github.com/injoyai/tdx/v2/protocol"
	"testing"
)

// 由2026-10-25保存的category=4解码样本重建固定响应；AAPL独立报价为39539690股。
// category=9的同源单根/多根量不一致，不应作为美股日线入口。
func TestCapturedUSDayVolume(t *testing.T) {
	for _, tc := range []struct {
		count   uint16
		payload string
	}{
		{1, "00000000000000000000000000000000000001009f283501b88eae4371ddaf4300e0ac43cddcad43cceb4c50ea535b0200000000"},
		{2, "00000000000000000000000000000000000002009e2835015278ac43142eae43d723ab43291cae438c3b7250bb9ecb02000000009f283501b88eae4371ddaf4300e0ac43cddcad43cceb4c50ea535b0200000000"},
	} {
		data, err := hex.DecodeString(tc.payload)
		if err != nil {
			t.Fatal(err)
		}
		c := marketTestClient(t, nil, func(_ []byte, b []byte) []byte {
			if b[0] != 74 || binary.LittleEndian.Uint16(b[10:12]) != 4 || binary.LittleEndian.Uint16(b[18:20]) != tc.count {
				t.Error("wrong US day request", b)
			}
			return data
		})
		r, err := c.GetKlineDay("usAAPL", 0, tc.count)
		if err != nil || r.Count != int(tc.count) || r.Category != protocol.TypeKlineDay2 {
			t.Fatal(r, err)
		}
		last := r.List[len(r.List)-1]
		if last.Time != "2026-10-23" || last.Volume != 39539690 || int64(last.Extended.Trade) != 39539690 {
			t.Fatal("US shares disagree with independent saved quote", last)
		}
	}
}
