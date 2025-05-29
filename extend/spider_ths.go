package extend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/injoyai/conv"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/protocol"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	UrlTHSDayKline       = "http://d.10jqka.com.cn/v6/line/hs_%s/0%d/all.js"
	THS_QFQ        uint8 = 1 //前复权
	THS_HFQ        uint8 = 2 //后复权
)

func NewTHSDayKline() *THSDayKline {
	return &THSDayKline{
		Client: http.DefaultClient,
	}
}

/*
THSDayKline
前复权,和通达信对的上,和东方财富对不上
后复权,和通达信,东方财富都对不上
*/
type THSDayKline struct {
	Codes  []string
	Client *http.Client
}

func (this *THSDayKline) GetName() string {
	return "同花顺日线"
}

func (this *THSDayKline) Run(ctx context.Context, m *tdx.Manage) error {
	codes := this.Codes
	if len(codes) == 0 {
		codes = m.Codes.GetStocks()
	}
	for _, _type := range []uint8{THS_QFQ, THS_HFQ} {
		for _, code := range codes {
			ls, err := this.Pull(ctx, code, _type)
			if err != nil {
				return err
			}

			_ = ls
			//加入数据库

			<-time.After(time.Millisecond * 300)
		}
	}
	return nil
}

func (this *THSDayKline) Pull(ctx context.Context, code string, _type uint8) ([]*Kline, error) {
	if _type != THS_QFQ && _type != THS_HFQ {
		return nil, fmt.Errorf("数据类型错误,例如:前复权1或后复权2")
	}

	code = protocol.AddPrefix(code)
	if len(code) != 8 {
		return nil, fmt.Errorf("股票代码错误,例如:SZ000001或000001")
	}

	u := fmt.Sprintf(UrlTHSDayKline, code[2:], _type)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	/*
	 'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) '
	                      'Chrome/90.0.4430.212 Safari/537.36',
	        'Referer': 'http://stockpage.10jqka.com.cn/',
	        'DNT': '1',
	*/
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/89.0.4389.90 Safari/537.36 Edg/89.0.774.54")
	req.Header.Set("Referer", "http://stockpage.10jqka.com.cn/")
	req.Header.Set("DNT", "1")
	resp, err := this.Client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	n := bytes.IndexByte(bs, '(')
	bs = bs[n+1 : len(bs)-1]

	m := map[string]any{}
	err = json.Unmarshal(bs, &m)
	if err != nil {
		return nil, err
	}

	total := conv.Int(m["total"])
	priceFactor := conv.Float64(m["priceFactor"])
	prices := strings.Split(conv.String(m["price"]), ",")
	dates := strings.Split(conv.String(m["dates"]), ",")
	volumes := strings.Split(conv.String(m["volumn"]), ",")
	start := conv.String(m["start"])
	t, err := time.Parse("20060102", start)
	if err != nil {
		return nil, err
	}

	//好像到了22点,总数量会比实际多1
	if total == len(dates)+1 && total == len(volumes)+1 {
		total -= 1
	}
	//判断数量是否对应
	if total*4 != len(prices) || total != len(dates) || total != len(volumes) {
		return nil, fmt.Errorf("total=%d prices=%d dates=%d volumns=%d", total, len(prices), len(dates), len(volumes))
	}

	ls := []*Kline(nil)

	year := t.Year()
	lastDate := ""
	for i := 0; i < total; i++ {
		//当日前变小时(12xx变01xx),说明过了1年,除非该股票停牌了1年多则数据错误
		if dates[i] < lastDate {
			year++
		}
		lastDate = dates[i]
		x, err := time.Parse("0102", dates[i])
		if err != nil {
			return nil, err
		}
		x = time.Date(year, x.Month(), x.Day(), 15, 0, 0, 0, time.Local)
		low := protocol.Price(conv.Float64(prices[i*4+0]) * 1000 / priceFactor)
		ls = append(ls, &Kline{
			Code:   protocol.AddPrefix(code),
			Date:   x.Unix(),
			Open:   protocol.Price(conv.Float64(prices[i*4+1])*1000/priceFactor) + low,
			High:   protocol.Price(conv.Float64(prices[i*4+2])*1000/priceFactor) + low,
			Low:    low,
			Close:  protocol.Price(conv.Float64(prices[i*4+3])*1000/priceFactor) + low,
			Volume: (conv.Int64(volumes[i]) + 50) / 100,
		})
	}

	return ls, nil
}
