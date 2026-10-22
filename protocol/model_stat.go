package protocol

import "strings"

// 通达信盘后逐股统计数据文件(随 zhb.zip 下发, GBK 文本, `|` 分隔, 每行一只股票, 全市场)。
const (
	FileTdxStat  = "tdxstat.cfg"  // 个股综合统计指标(35 字段)
	FileTdxStat2 = "tdxstat2.cfg" // 个股资金流向 + 板块归属(21 字段)
)

// TdxStat 个股综合统计指标(来自 tdxstat.cfg, 35 字段)。
//
// 字段下标按文件列(1 基)说明; Fields 为全部原始字段(0 基, 与文件列一一对应)。
// 已核验字段(10 只 >500 亿市值股票对照腾讯实盘 + 通达信同源日线, 见各字段后命中率):
//
//	[4]市盈率TTM [6]连涨连跌天数 [7]涨跌幅% [10]静态市盈率 [11]股息率%。
//
// 区间涨跌幅(下标为 0 基文件列)经 10 只大市值股对 130 日同源日线核验:
//
//	[28]5日 [30]10日 [20]60日 = MAE≈0(精确); [18]20日 MAE0.23; [21]年初至今(基准上年末) MAE0.73。
//	相邻字段为"含当日 N 根"变体(差 1 个交易日): [27]近5根 [29]近10根 [17]近20根 [19]近60根。
//
// 其余字段为通达信内部格式、无官方文档, 不强行命名, 用 Fields 自取。
// 已排除(实测比例不恒定, 非对应指标): [2]非换手率, [7][8]非短周期涨跌幅, [11][14][24]非成交量/额/市值。
type TdxStat struct {
	Market uint8  // [1] 市场 0=深 1=沪 2=京
	Code   string // [2] 证券代码
	Date   string // [5] 数据日期 YYYYMMDD

	PETTM     float64 // [4]  市盈率(TTM)        10/10 (同日核验 CV=0.012, 近乎精确)
	TrendDays int     // [6]  连涨连跌天数(正涨负跌) 10/10 (同源日线精确)
	ChangePct float64 // [7]  涨跌幅%            10/10 (同日核验 CV=0.004, 精确)
	PEStatic  float64 // [10] 静态市盈率          10/10 (同日核验 CV=0.012, 近乎精确)
	DivYield  float64 // [11] 股息率%            10/10 (通达信口径; F10 五粮液6.26 精确印证, 与第三方口径或异)

	Chg5   float64 // [28] 5日涨跌幅%   10/10 (130日日线核验 MAE≈0, 精确)
	Chg10  float64 // [30] 10日涨跌幅%  10/10 (MAE≈0, 精确)
	Chg20  float64 // [18] 20日涨跌幅%  10/10 (MAE0.23)
	Chg60  float64 // [20] 60日涨跌幅%  10/10 (MAE≈0)
	ChgYTD float64 // [21] 年初至今涨跌幅% 10/10 (基准上年末收盘, MAE0.73, 含分红口径差)

	Fields []string // 全部 35 个原始字段(0 基)
}

// TdxStat2 个股资金流向 + 板块归属(来自 tdxstat2.cfg, 21 字段)。
//
// 已核验字段(10 只大市值股对 6/3 同源日线 + 东财实盘):
//
//	[3]今日成交额 [5]昨日成交额(均万元, 精确); [16]IPO发行价(10/10 全中);
//	[17]52周最高价 [18]52周最低价; [13]所属/领涨板块指数代码(880xxx 行业概念/881xxx 地域)。
//
// 注: zhb.zip 不含融资融券数据(已对东财两融全字段比对, 无恒定关系)。
// 其余资金分档字段语义为推断, 原始值保留在 Fields。
type TdxStat2 struct {
	Market     uint8  // [0] 市场
	Code       string // [1] 证券代码
	Date       string // [2] 数据日期 YYYYMMDD
	BlockIndex string // [13] 板块指数代码(id), 可能为空

	Amount     float64 // [3]  今日成交额(万元)  精确
	AmountPrev float64 // [5]  昨日成交额(万元)  精确
	IPOPrice   float64 // [16] IPO 发行价(元)   10/10 全中
	High52W    float64 // [17] 52周最高价(元)
	Low52W     float64 // [18] 52周最低价(元)

	Fields []string // 全部 21 个原始字段
}

// splitStatLines 按行分割 GBK 文本并解码, 去空行。
func splitStatLines(data []byte) []string {
	return strings.Split(string(GBKToUTF8(data)), "\n")
}

func field(f []string, i int) string {
	if i < len(f) {
		return f[i]
	}
	return ""
}

// ParseTdxStat 解析 tdxstat.cfg → 个股综合统计指标。
func ParseTdxStat(data []byte) []*TdxStat {
	lines := splitStatLines(data)
	out := make([]*TdxStat, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimRight(ln, "\r")
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		f := strings.Split(ln, "|")
		if len(f) < 5 || f[1] == "" {
			continue
		}
		out = append(out, &TdxStat{
			Market:    uint8(Uint16FromStr(f[0])),
			Code:      f[1],
			Date:      field(f, 4),
			PETTM:     Float64FromStr(field(f, 3)),  // 文件列[4]
			TrendDays: IntFromStr(field(f, 5)),      // 文件列[6]
			ChangePct: Float64FromStr(field(f, 6)),  // 文件列[7]
			PEStatic:  Float64FromStr(field(f, 9)),  // 文件列[10]
			DivYield:  Float64FromStr(field(f, 10)), // 文件列[11]
			Chg5:      Float64FromStr(field(f, 28)),
			Chg10:     Float64FromStr(field(f, 30)),
			Chg20:     Float64FromStr(field(f, 18)),
			Chg60:     Float64FromStr(field(f, 20)),
			ChgYTD:    Float64FromStr(field(f, 21)),
			Fields:    f,
		})
	}
	return out
}

// ParseTdxStat2 解析 tdxstat2.cfg → 个股资金流向 + 板块归属。
func ParseTdxStat2(data []byte) []*TdxStat2 {
	lines := splitStatLines(data)
	out := make([]*TdxStat2, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimRight(ln, "\r")
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		f := strings.Split(ln, "|")
		if len(f) < 14 || f[1] == "" {
			continue
		}
		out = append(out, &TdxStat2{
			Market:     uint8(Uint16FromStr(f[0])),
			Code:       f[1],
			Date:       field(f, 2),
			BlockIndex: field(f, 13),
			Amount:     Float64FromStr(field(f, 3)),
			AmountPrev: Float64FromStr(field(f, 5)),
			IPOPrice:   Float64FromStr(field(f, 16)),
			High52W:    Float64FromStr(field(f, 17)),
			Low52W:     Float64FromStr(field(f, 18)),
			Fields:     f,
		})
	}
	return out
}

// StockBlockIndex 从 tdxstat2 数据提取 证券代码→所属板块指数代码(id) 映射, 跳过无归属个股。
// 这是 block_*.dat(板块→成分股) 的反向映射, 且免名称匹配。
func StockBlockIndex(stats []*TdxStat2) map[string]string {
	m := make(map[string]string, len(stats))
	for _, s := range stats {
		if s.BlockIndex != "" {
			m[s.Code] = s.BlockIndex
		}
	}
	return m
}
