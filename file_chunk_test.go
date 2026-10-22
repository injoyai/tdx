package tdx

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strconv"
	"testing"

	"github.com/injoyai/tdx/v2/protocol"
)

func TestFileChunkLengthValidation(t *testing.T) {
	c := respondingTestClient(t, false, func(h, b []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeBlockMeta {
			return protocol.Bytes(uint32(30001))
		}
		return make([]byte, 4+30001)
	})
	for _, get := range []func(string) ([]byte, error){c.GetReportFile, c.GetBlockFileRaw} {
		if data, err := get("sample"); err == nil || data != nil {
			t.Fatal("oversized chunk accepted", len(data), err)
		}
	}
}

func TestFileChunkTermination(t *testing.T) {
	for _, size := range []int{0, 17, 30000, 30017} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			want := bytes.Repeat([]byte{7}, size)
			c := respondingTestClient(t, false, func(h, b []byte) []byte {
				if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeBlockMeta {
					return protocol.Bytes(uint32(size))
				}
				start, count := int(binary.LittleEndian.Uint32(b)), int(binary.LittleEndian.Uint32(b[4:]))
				end := min(start+count, size)
				return append(protocol.Bytes(uint32(end-start)), want[start:end]...)
			})
			got, err := c.GetReportFile("sample")
			if err != nil || !bytes.Equal(got, want) {
				t.Fatal("report differs", len(got), err)
			}
			got, err = c.GetBlockFileRaw("sample")
			if size == 0 {
				if err == nil {
					t.Fatal("empty metadata accepted")
				}
			} else if err != nil || !bytes.Equal(got, want) {
				t.Fatal("block differs", len(got), err)
			}
		})
	}
	// 已知大小的文件不能将提前遇到空块当作成功 EOF。
	c := respondingTestClient(t, false, func(h, b []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:]) == protocol.TypeBlockMeta {
			return protocol.Bytes(uint32(17))
		}
		return make([]byte, 4)
	})
	if _, err := c.GetBlockFileRaw("truncated"); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal("missing data not reported", err)
	}
}
