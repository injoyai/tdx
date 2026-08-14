package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

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

func UTF8ToGBK(text []byte) []byte {
	r := bytes.NewReader(text)
	decoder := transform.NewReader(r, simplifiedchinese.GBK.NewDecoder()) //GB18030
	content, _ := io.ReadAll(decoder)
	return bytes.ReplaceAll(content, []byte{0x00}, []byte{})
}

// DecodeCode 解析证券代码,返回(交易所, 去掉前缀的代码主体, 错误)。
//
// 支持三种输入形式:
//  1. 带交易所前缀,如 "sz000001"/"sh600000"/"hk00700"/"usAAPL"/"cffIF2609"/"上海600000",
//     前缀支持小写缩写(见 Exchange.String())、大写("SZ000001")或中文名("上海600000")。
//  2. 带点后缀,如 "000001.SZ"/"600000.SH"/"00700.HK"/"AAPL.US"/"IF2609.CFF",
//     后缀为交易所缩写(大小写均可);美股点代码如 "BRK.B" 因后缀 B 不是交易所而按美股代码解析。
//  3. 裸 6 位数字代码,自动识别并补前缀(A股/基金/指数/可转债/板块指数),如 "000001"→sz000001。
//  4. 裸非 6 位代码,根据代码形态推断市场:
//     - 5 位纯数字(如 00700)→ 香港(ExchangeHK)
//     - 纯字母代码(如 AAPL/BRK.B)→ 美国(ExchangeUS)
//     - 字母+数字的合约代码(如 IF2609)→ 无法确定交易所,提示使用显式前缀
//
// 代码主体保持原始形态(A股仍为6位数字);无法识别的输入返回明确错误,不会静默解析。
func DecodeCode(code string) (Exchange, string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return 0, "", fmt.Errorf("代码不能为空")
	}

	// 1. 带前缀形式: 前缀(交易所缩写/中文名) + 代码主体
	if ex, body, ok := splitPrefix(code); ok {
		return ex, body, nil
	}

	// 2. 带点后缀形式: 主体.交易所缩写,如 "000001.SZ"/"AAPL.US"
	if i := strings.LastIndex(code, "."); i > 0 {
		suffix, body := code[i+1:], code[:i]
		if ex, err := ParseExchange(suffix); err == nil {
			if num, err := normalizeCode(ex, body); err == nil {
				return ex, num, nil
			}
		}
		// 后缀不是已知交易所时继续走下方推断(如美股 "BRK.B",后缀 B 非交易所)
	}

	// 3. 裸代码形式(无前缀),根据形态推断
	switch {
	case len(code) == 6 && isDigits(code):
		// A股/指数/ETF/可转债等,自动补前缀
		prefixed := AddPrefix(code)
		if prefixed == code {
			return 0, "", fmt.Errorf("无法识别的代码: %q", code)
		}
		return DecodeCode(prefixed)
	case len(code) == 5 && isDigits(code):
		// 港股: 5 位纯数字,如 00700
		return ExchangeHK, code, nil
	case isStockSymbol(code):
		// 美股: 纯字母代码,如 AAPL/BRK.B
		if isExchangeName(code) {
			return 0, "", fmt.Errorf("不能单独使用交易所前缀: %q", code)
		}
		return ExchangeUS, strings.ToUpper(code), nil
	case isFuturesCode(code):
		// 期货合约: 字母+数字,如 IF2609,交易所需显式前缀(cff/dce/shf等)
		return 0, "", fmt.Errorf("期货合约需显式交易所前缀,如 cff%s", code)
	default:
		return 0, "", fmt.Errorf("无法识别的代码: %q", code)
	}
}

// splitPrefix 尝试按"交易所前缀+代码主体"拆分代码。
// 前缀取已知市场缩写/中文名(最长优先匹配),且主体须通过该市场的代码格式校验,
// 否则视为非前缀(如美股代码 SHOP 不以 "sh"+"OP" 解析)。返回 (交易所, 主体, 是否匹配)。
func splitPrefix(s string) (Exchange, string, bool) {
	type cand struct {
		ex Exchange
		ln int
	}
	lower := strings.ToLower(s)
	var cands []cand
	for _, e := range allExchanges {
		abbr := strings.ToLower(e.String())
		if len(s) > len(abbr) && strings.HasPrefix(lower, abbr) {
			cands = append(cands, cand{e, len(abbr)})
		}
		name := strings.ToLower(e.Name())
		if len(s) > len(name) && strings.HasPrefix(lower, name) {
			cands = append(cands, cand{e, len(name)})
		}
	}
	// 最长前缀优先,如 "sho" 优先于 "sh"
	for i := 1; i < len(cands); i++ {
		for j := i; j > 0 && cands[j].ln > cands[j-1].ln; j-- {
			cands[j], cands[j-1] = cands[j-1], cands[j]
		}
	}
	for _, c := range cands {
		body := s[c.ln:]
		if _, err := normalizeCode(c.ex, body); err == nil {
			return c.ex, body, true
		}
	}
	return 0, "", false
}

// normalizeCode 校验并规整某市场下的代码主体。
//   - 美股(us): 纯字母代码,如 AAPL/BRK.B,统一转大写
//   - 港股/期权/期货等: 数字或字母数字混合代码(合约/行情代码),保持原样
//   - A股等(默认): 必须 6 位纯数字
func normalizeCode(ex Exchange, body string) (string, error) {
	if body == "" {
		return "", fmt.Errorf("代码不能为空: %s", ex.String())
	}
	switch ex {
	case ExchangeUS:
		if !isStockSymbol(body) {
			return "", fmt.Errorf("美股代码非法: %q", body)
		}
		return strings.ToUpper(body), nil
	case ExchangeHK, ExchangeSHO, ExchangeSZO, ExchangeOF, ExchangeCFF, ExchangeCZC, ExchangeDCE, ExchangeSHF, ExchangeGFE, ExchangeQHZ, ExchangeHI, ExchangeHG, ExchangeNQ:
		if isStockSymbol(body) {
			// 纯字母主体属于美股代码,不属于这些市场
			return "", fmt.Errorf("非法代码: %q", body)
		}
		if len(body) > 6 {
			return "", fmt.Errorf("代码过长: %q", body)
		}
		return body, nil
	default:
		if len(body) != 6 || !isDigits(body) {
			return "", fmt.Errorf("代码长度错误,例如:SZ000001")
		}
		return body, nil
	}
}

// isDigits 是否全部为数字
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// isStockSymbol 是否美股代码(1~5位字母,可含 . / - 等特殊字符,如 BRK.B)
func isStockSymbol(s string) bool {
	if s == "" || len(s) > 5 {
		return false
	}
	for _, c := range s {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '.' || c == '-' {
			continue
		}
		return false
	}
	return true
}

// isFuturesCode 是否期货合约代码(字母+数字混合,如 IF2609/A2609/CU2608)
func isFuturesCode(s string) bool {
	if len(s) < 2 || len(s) > 6 {
		return false
	}
	hasLetter, hasDigit := false, false
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			hasLetter = true
		case c >= '0' && c <= '9':
			hasDigit = true
		default:
			return false
		}
	}
	return hasLetter && hasDigit
}

func FloatUnit(f float64) (float64, string) {
	m := []string{"万", "亿"}
	unit := ""
	for i := 0; f > 1e4 && i < len(m); f /= 1e4 {
		unit = m[i]
	}
	return f, unit
}

func FloatUnitString(f float64) string {
	m := []string{"万", "亿", "万亿", "亿亿", "万亿亿", "亿亿亿"}
	unit := ""
	for i := 0; f > 1e4 && i < len(m); i++ {
		unit = m[i]
		f /= 1e4
	}
	if unit == "" {
		return conv.String(f)
	}
	return fmt.Sprintf("%0.2f%s", f, unit)
}

func IntUnitString(n int) string {
	return FloatUnitString(float64(n))
}

func Int64UnitString(n int64) string {
	return FloatUnitString(float64(n))
}

func GetHourMinute(bs [2]byte) string {
	n := Uint16(bs[:])
	h := n / 60
	m := n % 60
	return fmt.Sprintf("%02d:%02d", h, m)
}

func GetTime(bs [4]byte, Type uint8) time.Time {
	switch Type {
	case TypeKlineMinute, TypeKlineMinute2, TypeKline5Minute, TypeKline15Minute, TypeKline30Minute, TypeKline60Minute:

		yearMonthDay := Uint16(bs[:2])
		hourMinute := Uint16(bs[2:4])
		year := int(yearMonthDay>>11 + 2004)
		month := yearMonthDay % 2048 / 100
		day := int((yearMonthDay % 2048) % 100)
		hour := int(hourMinute / 60)
		minute := int(hourMinute % 60)
		return time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.Local)

	default:

		yearMonthDay := Uint32(bs[:4])
		year := int(yearMonthDay / 10000)
		month := int((yearMonthDay % 10000) / 100)
		day := int(yearMonthDay % 100)
		return time.Date(year, time.Month(month), day, 15, 0, 0, 0, time.Local)

	}
}

//func basePrice(code string) Price {
//	if len(code) < 2 {
//		return 1
//	}
//	switch code[:1] {
//	case "8":
//		return 1
//	}
//	switch code[:2] {
//	case "60", "30", "68", "00", "92", "43", "39":
//		return 1
//	default:
//		return 1
//	}
//}

func basePrice(code string) Price {
	switch {
	case IsETF(code):
		return 10
	case IsStock(code):
		return 1
	case IsIndex(code):
		return 1
	default:
		return 1
	}
}

func getVolume(val uint32) float64 {
	return float64(math.Float32frombits(val))
}

func getVolume2(val uint32) float64 {
	return getVolume(val)
}

// IsStock 是否是股票,示例sz000001
func IsStock(code string) bool {
	return IsSZStock(code) || IsSHStock(code) || IsBJStock(code)
}

// IsConvertibleBond reports whether code belongs to a current convertible-bond code range.
func IsConvertibleBond(code string) bool {
	if len(code) != 8 {
		return false
	}
	code = strings.ToLower(code)
	number := code[2:]
	switch code[:2] {
	case ExchangeSH.String():
		return strings.HasPrefix(number, "110") ||
			strings.HasPrefix(number, "111") ||
			strings.HasPrefix(number, "113") ||
			strings.HasPrefix(number, "118")
	case ExchangeSZ.String():
		return strings.HasPrefix(number, "123") ||
			strings.HasPrefix(number, "125") ||
			strings.HasPrefix(number, "126") ||
			strings.HasPrefix(number, "127") ||
			strings.HasPrefix(number, "128")
	default:
		return false
	}
}

func IsSZStock(code string) bool {
	return len(code) == 8 && strings.ToLower(code[0:2]) == ExchangeSZ.String() && isSZStock(code[2:])
}

func IsSHStock(code string) bool {
	return len(code) == 8 && strings.ToLower(code[0:2]) == ExchangeSH.String() && isSHStock(code[2:])
}

func IsBJStock(code string) bool {
	return len(code) == 8 && strings.ToLower(code[0:2]) == ExchangeBJ.String() && isBJStock(code[2:])
}

func isSHStock(code string) bool {
	if len(code) != 6 {
		return false
	}
	return code[:1] == "6"
}

func isSZStock(code string) bool {
	if len(code) != 6 {
		return false
	}
	return code[:1] == "0" || code[:2] == "30"
}

func isBJStock(code string) bool {
	if len(code) != 6 {
		return false
	}
	return code[:2] == "92"
}

// IsETF 是否是基金,示例sz159558
func IsETF(code string) bool {
	if len(code) != 8 {
		return false
	}
	code = strings.ToLower(code)
	switch {
	case code[0:2] == ExchangeSH.String() && isSHETF(code[2:]):
		return true
	case code[0:2] == ExchangeSZ.String() && isSZETF(code[2:]):
		return true
	}
	return false
}

func isSHETF(code string) bool {
	if len(code) != 6 {
		return false
	}
	switch code[:2] {
	case "50", "51", "52", "53", "56", "58": //55不是
		return true
	}

	return false
}

func isSZETF(code string) bool {
	if len(code) != 6 {
		return false
	}
	return code[:2] == "15" || code[:2] == "16"
}

func isBJETF(code string) bool {
	if len(code) != 6 {
		return false
	}
	return false
}

// IsIndex 是否是指数,sh000001,sz399001,bj899100
// 板块指数(880xxx 行业/概念, 881xxx 地域)归属上海交易所(ExchangeSH), 也视为指数。
func IsIndex(code string) bool {
	if len(code) != 8 {
		return false
	}
	code = strings.ToLower(code)
	switch {
	case code[0:2] == ExchangeSH.String() && isSHIndex(code[2:]):
		return true
	case code[0:2] == ExchangeSZ.String() && isSZIndex(code[2:]):
		return true
	case code[0:2] == ExchangeBJ.String() && isBJIndex(code[2:]):
		return true
	case code[0:2] == ExchangeSH.String() && isBlock(code[2:]):
		return true
	}
	return false
}

// isBlock 板块指数: 880xxx(概念/风格/地区) 881xxx(行业), 归属上海交易所。
func isBlock(code string) bool {
	if len(code) != 6 {
		return false
	}
	return code[:3] == "880" || code[:3] == "881"
}

func isSHIndex(code string) bool {
	if len(code) != 6 {
		return false
	}
	return code[:3] == "000" || code == "999999"
}

func isSZIndex(code string) bool {
	if len(code) != 6 {
		return false
	}
	return code[:3] == "399"
}

func isBJIndex(code string) bool {
	if len(code) != 6 {
		return false
	}
	return code[:3] == "899"
}

// AddPrefix 添加股票/基金/指数/可转债代码前缀,针对股票/基金/指数/可转债生效,例如000001,会增加前缀sz000001(平安银行),而不是sh000001(上证指数)
// 板块指数(880xxx/881xxx)增加前缀 sh,例如 880741 -> sh880741(归属上海交易所)。
// 可转债: 沪市(110/111/113/118)增加前缀 sh, 深市(123/125/126/127/128)增加前缀 sz。
func AddPrefix(code string) string {
	if len(code) == 6 {
		switch {
		case isSHStock(code):
			return ExchangeSH.String() + code
		case isSZStock(code):
			return ExchangeSZ.String() + code
		case isBJStock(code):
			return ExchangeBJ.String() + code

		case isSHETF(code):
			return ExchangeSH.String() + code
		case isSZETF(code):
			return ExchangeSZ.String() + code
		case isBJETF(code):
			return ExchangeBJ.String() + code

		case isSHIndex(code):
			return ExchangeSH.String() + code
		case isSZIndex(code):
			return ExchangeSZ.String() + code
		case isBJIndex(code):
			return ExchangeBJ.String() + code

		case isBlock(code):
			return ExchangeSH.String() + code

		case isSHBond(code):
			return ExchangeSH.String() + code
		case isSZBond(code):
			return ExchangeSZ.String() + code
		}
	}
	return code
}

// isSHBond 沪市可转债: 110/111/113/118。
func isSHBond(code string) bool {
	if len(code) != 6 {
		return false
	}
	return strings.HasPrefix(code, "110") ||
		strings.HasPrefix(code, "111") ||
		strings.HasPrefix(code, "113") ||
		strings.HasPrefix(code, "118")
}

// isSZBond 深市可转债: 123/125/126/127/128。
func isSZBond(code string) bool {
	if len(code) != 6 {
		return false
	}
	return strings.HasPrefix(code, "123") ||
		strings.HasPrefix(code, "125") ||
		strings.HasPrefix(code, "126") ||
		strings.HasPrefix(code, "127") ||
		strings.HasPrefix(code, "128")
}

func minutes(t time.Time) int {
	return t.Hour()*60 + t.Minute()
}

// I64Sqrt int64版的math.Sqrt
func I64Sqrt(x int64) int64 {
	r := int64(math.Sqrt(float64(x)))
	for (r+1)*(r+1) <= x {
		r++
	}
	for r*r > x {
		r--
	}
	return r
}
