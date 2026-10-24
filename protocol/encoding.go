package protocol

import (
	"bytes"
	"encoding/binary"
	"math"

	"github.com/injoyai/conv"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// String 字节先转小端,再转字符
func String(bs []byte) string {
	return string(Reverse(bs))
}

// Bytes 任意类型转小端字节
func Bytes(n any) []byte {
	return Reverse(conv.Bytes(n))
}

// Reverse 字节倒序
func Reverse(bs []byte) []byte {
	x := make([]byte, len(bs))
	for i, v := range bs {
		x[len(bs)-i-1] = v
	}
	return x
}

// Uint32 字节通过小端方式转为uint32
func Uint32(bs []byte) uint32 {
	// conv historically reads the first eight reversed bytes for oversized input.
	if len(bs) > 8 {
		bs = bs[len(bs)-8:]
	}
	if len(bs) >= 4 {
		return binary.LittleEndian.Uint32(bs)
	}
	var n uint32
	for i, b := range bs {
		n |= uint32(b) << (8 * i)
	}
	return n
}

// Float32 字节通过小端方式转为float32
func Float32(bs []byte) float32 {
	if len(bs) < 4 {
		return 0
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(bs))
}

// Uint16 字节通过小端方式转为uint16
func Uint16(bs []byte) uint16 {
	if len(bs) > 8 {
		bs = bs[len(bs)-8:]
	}
	if len(bs) >= 2 {
		return binary.LittleEndian.Uint16(bs)
	}
	if len(bs) == 1 {
		return uint16(bs[0])
	}
	return 0
}

// GBKToUTF8 将 GBK 文本解码为 UTF-8，并移除 NUL 字节。
func GBKToUTF8(text []byte) []byte {
	content, _ := simplifiedchinese.GBK.NewDecoder().Bytes(text)
	if content == nil {
		return []byte{}
	}
	return bytes.ReplaceAll(content, []byte{0x00}, []byte{})
}
