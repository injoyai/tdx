package protocol

import (
	"errors"
	"io"
)

func cutIntChecked(bs []byte) ([]byte, int, error) {
	for i, b := range bs {
		// getData 使用 int32；第五字节只剩四个有效数值位。
		if i >= 5 || (i == 4 && b&0x7f > 0x0f) {
			return nil, 0, errors.New("整数超出 int32 范围")
		}
		if b&0x80 == 0 {
			return bs[i+1:], int(getData(bs[:i+1])), nil
		}
	}
	return nil, 0, errors.New("整数数据不足或未结束")
}

// wireDecoder 保留第一个错误，让固定字段和变长整数共用边界检查。
type wireDecoder struct {
	bs  []byte
	err error
}

func (d *wireDecoder) take(n int) []byte {
	if d.err != nil {
		return nil
	}
	if len(d.bs) < n {
		d.err = io.ErrUnexpectedEOF
		return nil
	}
	b := d.bs[:n]
	d.bs = d.bs[n:]
	return b
}

func (d *wireDecoder) uint16() uint16 {
	b := d.take(2)
	if d.err != nil {
		return 0
	}
	return Uint16(b)
}

func (d *wireDecoder) uint32() uint32 {
	b := d.take(4)
	if d.err != nil {
		return 0
	}
	return Uint32(b)
}

func (d *wireDecoder) number() int {
	if d.err != nil {
		return 0
	}
	var n int
	d.bs, n, d.err = cutIntChecked(d.bs)
	return n
}
