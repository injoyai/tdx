package protocol

import (
	"encoding/hex"
	"math"
	"testing"
)

func TestUTF8ToGBK(t *testing.T) {
	s := "789c6378c1cec7a5cbcbc061c5b4898987b9050ed1f90c65b74c1825bd18c1b42890fecff09c81819191f13fc3c9f3bb169f5e7dfefeb5ef57f7199a305009308208e5b32bb6bcbf7014871200176e1df3"
	//s := "b1cb74001c00000000000d005100bd00789c6378c1cecb252ace6066c5b4898987b9050ed1f90cc5b74c18a5bc18c1b43490fecff09c81819191f13fc3c9f3bb169f5e7dfefeb5ef57f7199a305009308208e5b32bb6bcbf70148712002d7f1e13"
	bs, err := hex.DecodeString(s)
	if err != nil {
		t.Error(err)
		return
	}
	t.Log(string(bs[68:]))
	bs = UTF8ToGBK(bs)
	t.Log(string(bs))
}

func TestGetVolume(t *testing.T) {
	for _, want := range []float32{0, 0.5, 1, 10, 436, 587760.1875} {
		bits := math.Float32bits(want)
		if got := getVolume(bits); got != float64(want) {
			t.Errorf("getVolume(%08x) = %v, want %v", bits, got, want)
		}
		if got := getVolume2(bits); got != float64(want) {
			t.Errorf("getVolume2(%08x) = %v, want %v", bits, got, want)
		}
	}
}

func TestIsConvertibleBond(t *testing.T) {
	for _, code := range []string{"sh110075", "SH111000", "sh113001", "sh118001", "sz123064", "sz125001", "sz126001", "sz127001", "sz128001"} {
		if !IsConvertibleBond(code) {
			t.Errorf("IsConvertibleBond(%q) = false, want true", code)
		}
	}
	for _, code := range []string{"sh600000", "sz000001", "sh510300", "sz159919", "sh000001", "110075"} {
		if IsConvertibleBond(code) {
			t.Errorf("IsConvertibleBond(%q) = true, want false", code)
		}
	}
}

func TestFloat32(t *testing.T) {
	t.Log(Float32([]byte{0x00, 0x00, 0x20, 0x41})) //10
}

func TestParseExchange(t *testing.T) {
	// 数字字符串
	for _, s := range []string{"0", "1", "2", "44", "8", "9", "31", "74", "62", "102", "38", "47", "28", "29", "30", "66", "27", "33", "7", "4", "5", "6", "67", "42"} {
		ex, err := ParseExchange(s)
		if err != nil {
			t.Errorf("ParseExchange(%q) error: %v", s, err)
			continue
		}
		if ex.String() == "unknown" {
			t.Errorf("ParseExchange(%q) = unknown", s)
		}
	}
	// 小写缩写
	expects := map[string]Exchange{
		"sz": ExchangeSZ, "sh": ExchangeSH, "bj": ExchangeBJ, "nq": ExchangeNQ,
		"sho": ExchangeSHO, "szo": ExchangeSZO, "hk": ExchangeHK, "us": ExchangeUS,
		"csi": ExchangeCSI, "cni": ExchangeCNI, "hg": ExchangeHG, "cff": ExchangeCFF,
		"czc": ExchangeCZC, "dce": ExchangeDCE, "shf": ExchangeSHF,
		"gfe": ExchangeGFE, "hi": ExchangeHI, "of": ExchangeOF,
		"cffo": ExchangeCFFO, "czco": ExchangeCZCO, "dceo": ExchangeDCEO,
		"shfo": ExchangeSHFO, "gfeo": ExchangeGFEO, "qhz": ExchangeQHZ,
	}
	for s, want := range expects {
		got, err := ParseExchange(s)
		if err != nil {
			t.Errorf("ParseExchange(%q) error: %v", s, err)
			continue
		}
		if got != want {
			t.Errorf("ParseExchange(%q) = %v, want %v", s, got, want)
		}
	}
	// 中文名称
	zhExpects := map[string]Exchange{
		"深圳": ExchangeSZ, "上海": ExchangeSH, "北京": ExchangeBJ, "新三板": ExchangeNQ,
		"香港交易所": ExchangeHK, "美国股票": ExchangeUS,
	}
	for s, want := range zhExpects {
		got, err := ParseExchange(s)
		if err != nil {
			t.Errorf("ParseExchange(%q) error: %v", s, err)
			continue
		}
		if got != want {
			t.Errorf("ParseExchange(%q) = %v, want %v", s, got, want)
		}
	}
	// 非法输入
	if _, err := ParseExchange("xx"); err == nil {
		t.Error("ParseExchange(\"xx\") 应当返回错误")
	}
}

func TestDecodeCode(t *testing.T) {
	// 正常路径
	good := []struct {
		in   string
		ex   Exchange
		num  string
	}{
		{"sz000001", ExchangeSZ, "000001"},
		{"sh600000", ExchangeSH, "600000"},
		{"bj920000", ExchangeBJ, "920000"},
		{"SH000001", ExchangeSH, "000001"}, //大写
		{"000001", ExchangeSZ, "000001"},   //自动补前缀
		{"600000", ExchangeSH, "600000"},
		{"920000", ExchangeBJ, "920000"},
		{"399001", ExchangeSZ, "399001"}, //深成指
		{"nq400001", ExchangeNQ, "400001"},
		{"sh000001", ExchangeSH, "000001"}, //上证指数
	}
	for _, c := range good {
		ex, num, err := DecodeCode(c.in)
		if err != nil {
			t.Errorf("DecodeCode(%q) error: %v", c.in, err)
			continue
		}
		if ex != c.ex || num != c.num {
			t.Errorf("DecodeCode(%q) = (%v,%q), want (%v,%q)", c.in, ex, num, c.ex, c.num)
		}
	}
	// 非法路径:必须明确报错,不得静默解析
	bad := []string{
		"000001.SZ",  //带点后缀,数字前缀不能被当市场编码
		"123456789",  //纯数字9位
		"xx000001",   //非法前缀
		"sh0000010",  //9位数字无合法前缀
		"",           //空串
		"sz",         //只有前缀
		"hk00700",    //港股5位代码,当前不支持
	}
	for _, in := range bad {
		if _, _, err := DecodeCode(in); err == nil {
			t.Errorf("DecodeCode(%q) 应当返回错误", in)
		}
	}
}
