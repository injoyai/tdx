package protocol

import (
	"bytes"
	"encoding/binary"
	"io"
)

// F10 公司信息协议。参考 pytdx GetCompanyInfoCategory / GetCompanyInfoContent。
const (
	TypeCompanyCat     uint16 = 0x02CF // 公司信息分类（F10 目录）
	TypeCompanyContent uint16 = 0x02D0 // 公司信息内容
)

// CompanyCategory F10 一个分类条目（指向资讯文件的某段）。
type CompanyCategory struct {
	Name     string `json:"name"`     // 分类名（公司概况/财务分析/分红扩股…）
	Filename string `json:"filename"` // 资讯文件名
	Start    uint32 `json:"start"`    // 文件内偏移
	Length   uint32 `json:"length"`   // 内容长度
}

type companyCat struct{}
type companyContent struct{}

// MCompanyCat / MCompanyContent 协议单例。
var (
	MCompanyCat     companyCat
	MCompanyContent companyContent
)

// Frame 构造 F10 分类请求帧。market: 0深 1沪 2京。
func (companyCat) Frame(market uint8, code string) *Frame {
	data := make([]byte, 12)
	binary.LittleEndian.PutUint16(data[0:2], uint16(market))
	copy(data[2:8], code)
	// data[8:12] = 0 (uint32)
	return &Frame{Control: Control01, Type: TypeCompanyCat, Data: data}
}

// Decode 解析 F10 分类列表。
func (companyCat) Decode(bs []byte) ([]CompanyCategory, error) {
	if len(bs) < 2 {
		return nil, io.ErrUnexpectedEOF
	}
	num := int(Uint16(bs[0:2]))
	if len(bs)-2 < num*152 {
		return nil, io.ErrUnexpectedEOF
	}
	pos := 2
	out := make([]CompanyCategory, 0, num)
	for i := 0; i < num; i++ {
		name := decodeGBKName(bs[pos : pos+64])
		filename := cutAtNull(bs[pos+64 : pos+144])
		start := Uint32(bs[pos+144 : pos+148])
		length := Uint32(bs[pos+148 : pos+152])
		pos += 152
		out = append(out, CompanyCategory{Name: name, Filename: filename, Start: start, Length: length})
	}
	return out, nil
}

// cutAtNull 截取到首个 0x00（GBK 字段名/文件名定长 null 填充）。
func cutAtNull(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

// decodeGBKName 取首个 null 前的字节并 GBK 解码（去尾随定长填充与杂字节）。
func decodeGBKName(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(GBKToUTF8(b))
}

// Frame 构造 F10 内容请求帧。
func (companyContent) Frame(market uint8, code, filename string, start, length uint32) *Frame {
	data := make([]byte, 102)
	binary.LittleEndian.PutUint16(data[0:2], uint16(market))
	copy(data[2:8], code)
	// data[8:10] = 0 (uint16)
	copy(data[10:90], filename) // 80s
	binary.LittleEndian.PutUint32(data[90:94], start)
	binary.LittleEndian.PutUint32(data[94:98], length)
	// data[98:102] = 0 (uint32)
	return &Frame{Control: Control01, Type: TypeCompanyContent, Data: data}
}

// Decode 解析 F10 内容（GBK 文本）。
func (companyContent) Decode(bs []byte) (string, error) {
	if len(bs) < 12 {
		return "", io.ErrUnexpectedEOF
	}
	length := int(Uint16(bs[10:12]))
	end := 12 + length
	if end > len(bs) {
		return "", io.ErrUnexpectedEOF
	}
	return string(GBKToUTF8(bs[12:end])), nil
}
