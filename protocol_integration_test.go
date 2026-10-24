package tdx

import (
	"encoding/binary"
	"testing"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestGetBlockDataPropagatesParseErrors(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		size, count, components int
		valid                   bool
	}{
		{"complete", 386 + 2813, 1, 1, true},
		{"empty", 386, 0, 0, true},
		{"missing component region", 399, 1, 1, false},
		{"oversized component count", 386 + 2813, 1, 401, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, tc.size)
			binary.LittleEndian.PutUint16(data[384:], uint16(tc.count))
			if tc.count > 0 {
				copy(data[386:], "Sample")
				binary.LittleEndian.PutUint16(data[395:], uint16(tc.components))
				copy(data[399:], "1600000")
			}
			c := respondingTestClient(t, false, func(h, request []byte) []byte {
				if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeBlockMeta {
					return protocol.Bytes(uint32(len(data)))
				}
				start, size := int(binary.LittleEndian.Uint32(request)), int(binary.LittleEndian.Uint32(request[4:]))
				chunk := data[start:min(start+size, len(data))]
				return append(protocol.Bytes(uint32(len(chunk))), chunk...)
			})
			got, err := c.GetBlockData("sample.dat")
			if !tc.valid {
				if err == nil || got != nil {
					t.Fatal("malformed block data reported as success", got, err)
				}
				return
			}
			if err != nil || len(got) != tc.count {
				t.Fatal("valid block data changed", got, err)
			}
			if tc.count > 0 && (got[0].Name != "Sample" || len(got[0].Codes) != 1 || got[0].Codes[0] != "1600000") {
				t.Fatal("block fields changed", got)
			}
		})
	}
}
