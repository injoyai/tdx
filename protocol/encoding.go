package protocol

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/injoyai/conv"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
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
	return conv.Uint32(Reverse(bs))
}

// Float32 字节通过小端方式转为float32
func Float32(bs []byte) float32 {
	var f float32
	binary.Read(bytes.NewBuffer(bs), binary.LittleEndian, &f)
	return f
}

// Uint16 字节通过小端方式转为uint16
func Uint16(bs []byte) uint16 {
	return conv.Uint16(Reverse(bs))
}

// GBKToUTF8 将 GBK 文本解码为 UTF-8，并移除 NUL 字节。
func GBKToUTF8(text []byte) []byte {
	r := bytes.NewReader(text)
	decoder := transform.NewReader(r, simplifiedchinese.GBK.NewDecoder())
	content, _ := io.ReadAll(decoder)
	return bytes.ReplaceAll(content, []byte{0x00}, []byte{})
}
