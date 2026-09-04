// Package market 实现各市场的拉取 Unit（可插拔）。
// 期货市场：走扩展行情(7727) ExBars 拉取日线/分钟线。
package market

import (
	"fmt"
	"strings"
	"time"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/extend/pull"
	"github.com/injoyai/tdx/protocol"
)

// 期货市场编码（扩展行情 ExInstruments.Market）。
const (
	marketCFF = 47 // 中金
	marketCZC = 28 // 郑商
	marketDCE = 29 // 大商
	marketSHF = 30 // 上期/能源
	marketGFE = 66 // 广期
	marketQHZ = 42 // 期货指数
)

// 扩展行情 category：日K 用 TypeKlineDay2(=4)（v1 实测，见 MEMORY.md），分钟用 TypeKlineMinute2(=8)。
const (
	exDayCategory    = protocol.TypeKlineDay2    // 日K category（期货 v1 实测）
	exMinuteCategory = protocol.TypeKlineMinute2 // 1分钟K category
)

// futureMarketNames 期货市场编码→简称（编码进 Code.Code 前缀，区分交易所）。
var futureMarketNames = map[uint8]string{
	marketCFF: "cff",
	marketCZC: "czc",
	marketDCE: "dce",
	marketSHF: "shf",
	marketGFE: "gfe",
	marketQHZ: "qhz",
}

// exUnit 扩展行情(7727)市场的基础实现：通过 pull.Service.ExPool 取连接。
// 品种代码列表中，同一 Unit 可能覆盖多个扩展市场（如期货多个交易所），
// 此时交易所简称作为 Code.Code 前缀（"cff/IF2609"），由 marketOf 解析。
type exUnit struct {
	name pull.Market // 市场标识（pull.Market 枚举）
	// markets 该 Unit 覆盖的扩展市场编码（用于从 ExInstruments 过滤）
	markets []uint8
	// dayCategory 日K category；期货=TypeKlineDay2(=4)（v1 实测），港股/美股各自覆盖为 TypeKlineDay(=9)
	dayCategory uint8
	// minuteCategory 分钟K category；统一 TypeKlineMinute2(=8)
	minuteCategory uint8
}

func (u *exUnit) Name() string { return u.name.String() }

// Ex 取扩展行情连接池；未配置返回错误。
func (u *exUnit) Ex(s *pull.Service) (tdx.IPool, error) {
	p := s.ExPool()
	if p == nil {
		return nil, fmt.Errorf("pull: 市场 %s 需要配置 ExPool（扩展行情7727连接池）", u.name)
	}
	return p, nil
}

// Codes 从 ExInstruments 分页拉取品种列表，过滤出本 Unit 覆盖的市场编码。
// 多交易所市场（期货）时 Code.Code 带交易所前缀，如 "cff/IF2609"。
func (u *exUnit) Codes(s *pull.Service) ([]pull.Code, error) {
	p, err := u.Ex(s)
	if err != nil {
		return nil, err
	}
	out := []pull.Code{}
	err = p.Do(func(c *tdx.Client) error {
		var start uint32
		for {
			ins, err := c.ExInstruments(start, 800)
			if err != nil {
				return err
			}
			if len(ins) == 0 {
				break
			}
			for _, it := range ins {
				if !u.isMarket(it.Market) {
					continue
				}
				code := it.Code
				if u.multiMarket() {
					code = futureMarketNames[it.Market] + "/" + it.Code
				}
				out = append(out, pull.Code{
					Market: u.name,
					Code:   code,
					Name:   it.Name,
				})
			}
			start += uint32(len(ins))
			if len(ins) < 800 {
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// isMarket 判断市场编码是否属于本 Unit。
func (u *exUnit) isMarket(m uint8) bool {
	for _, x := range u.markets {
		if x == m {
			return true
		}
	}
	return false
}

// multiMarket 是否覆盖多个扩展市场（需要交易所前缀区分）。
func (u *exUnit) multiMarket() bool { return len(u.markets) > 1 }

// marketOf 解析 Code 的市场编码与裸代码（去掉交易所前缀）。
func (u *exUnit) marketOf(code pull.Code) (uint8, string, error) {
	codeStr := code.Code
	if i := strings.IndexByte(codeStr, '/'); i > 0 {
		for m, name := range futureMarketNames {
			if codeStr[:i] == name {
				return m, codeStr[i+1:], nil
			}
		}
		return 0, "", fmt.Errorf("pull: 未知的扩展市场前缀 %q", codeStr[:i])
	}
	if len(u.markets) == 0 {
		return 0, "", fmt.Errorf("pull: 市场 %s 未配置扩展市场编码", u.name)
	}
	return u.markets[0], codeStr, nil
}

// parseExTime 解析扩展行情时间字符串 "YYYY-MM-DD HH:MM"。
func parseExTime(dt string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", dt, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

// exPageLimit ExBars 的 start 参数为 uint16，翻页偏移超过 65535 会回绕，
// 故设上限护栏：超出仍未命中停止条件时报错（也意味着该接口最多取 ~65535 根历史）。
const exPageLimit = 65535

// readExBars 扩展行情只扫描一次；包含增量边界，不跨 uint16 偏移上限。
func readExBars(from time.Time,
	fetch func(uint16, uint16) ([]protocol.ExKline, error)) ([]protocol.ExKline, error) {
	var out []protocol.ExKline
	for page := 0; ; page += 800 {
		if page > exPageLimit {
			return nil, fmt.Errorf("pull: 扩展行情翻页偏移超过 %d", exPageLimit)
		}
		bars, err := fetch(uint16(page), 800)
		if err != nil {
			return nil, err
		}
		for i := len(bars) - 1; i >= 0; i-- {
			t := parseExTime(bars[i].Datetime)
			if t.IsZero() {
				return nil, fmt.Errorf("pull: 无效扩展行情时间 %q", bars[i].Datetime)
			}
			if t.Before(from) {
				return out, nil
			}
			out = append(out, bars[i])
			if t.Equal(from) {
				return out, nil
			}
		}
		if len(bars) < 800 {
			return out, nil
		}
	}
}

func (u *exUnit) bars(s *pull.Service, code pull.Code, category uint8, from time.Time) ([]protocol.ExKline, error) {
	p, err := u.Ex(s)
	if err != nil {
		return nil, err
	}
	market, bare, err := u.marketOf(code)
	if err != nil {
		return nil, err
	}
	var out []protocol.ExKline
	err = p.Do(func(c *tdx.Client) error {
		var err error
		out, err = readExBars(from, func(start, count uint16) ([]protocol.ExKline, error) {
			return c.ExBars(category, market, bare, start, count)
		})
		return err
	})
	return out, err
}

// FetchDay 增量拉日线，含最后一根以便重插修复。
func (u *exUnit) FetchDay(s *pull.Service, code pull.Code) error {
	last, err := s.LastDayUnix(code)
	if err != nil {
		return err
	}
	from := s.Start()
	if last > from.Unix() {
		from = time.Unix(last, 0)
	}
	bars, err := u.bars(s, code, u.dayCategory, from)
	if err != nil {
		return err
	}
	out := make([]*pull.KlineDay, 0, len(bars))
	for _, k := range bars {
		out = append(out, &pull.KlineDay{
			Unix: parseExTime(k.Datetime).Unix(), Open: k.Open, High: k.High,
			Low: k.Low, Close: k.Close, Volume: pull.ToShares(int64(k.Trade)), Amount: k.Amount,
		})
	}
	return s.SaveDay(code, last, out)
}

// FetchMin 多年份共用一次分页，成功扫描后才提交年度完成标记。
func (u *exUnit) FetchMin(s *pull.Service, code pull.Code) error {
	return pullMinutes(s, code, time.Now(), func(from time.Time) ([]*pull.KlineMinute, error) {
		bars, err := u.bars(s, code, u.minuteCategory, from)
		if err != nil {
			return nil, err
		}
		out := make([]*pull.KlineMinute, 0, len(bars))
		for _, k := range bars {
			out = append(out, &pull.KlineMinute{
				Unix: parseExTime(k.Datetime).Unix(), Open: k.Open, High: k.High,
				Low: k.Low, Close: k.Close, Volume: pull.ToShares(int64(k.Trade)), Amount: k.Amount,
			})
		}
		return out, nil
	}, nil)
}

// Future 期货市场（中金/郑商/大商/上期/广期/期货指数）。
type Future struct{ exUnit }

var _ pull.Unit = (*Future)(nil)

func init() {
	pull.Register(&Future{exUnit{
		name:           pull.MarketFuture,
		markets:        []uint8{marketCFF, marketCZC, marketDCE, marketSHF, marketGFE, marketQHZ},
		dayCategory:    exDayCategory,
		minuteCategory: exMinuteCategory,
	}})
}
