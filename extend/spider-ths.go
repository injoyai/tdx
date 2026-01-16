package extend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/injoyai/conv"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/protocol"
)

const (
	UrlTHSDayKline       = "http://d.10jqka.com.cn/v6/line/hs_%s/0%d/all.js"
	THS_BFQ        uint8 = 0 //不复权
	THS_QFQ        uint8 = 1 //前复权
	THS_HFQ        uint8 = 2 //后复权
)

// GetTHSDayKlineFactorFull 增加计算复权因子
func GetTHSDayKlineFactorFull(code string, c *tdx.Client) ([3]protocol.Klines, []*Factor, error) {
	ks, err := GetTHSDayKlineFull(code, c)
	if err != nil {
		return [3]protocol.Klines{}, nil, err
	}
	mQPrice := make(map[string]float64)
	for _, v := range ks[1] {
		mQPrice[v.Time.Format(time.DateOnly)] = v.Close.Float64()
	}
	mHPrice := make(map[string]float64)
	for _, v := range ks[2] {
		mHPrice[v.Time.Format(time.DateOnly)] = v.Close.Float64()
	}
	fs := make([]*Factor, 0, len(ks[0]))
	for _, v := range ks[0] {
		fs = append(fs, &Factor{
			Date:    v.Time.Unix(),
			QFactor: mQPrice[v.Time.Format(time.DateOnly)] / v.Close.Float64(),
			HFactor: mHPrice[v.Time.Format(time.DateOnly)] / v.Close.Float64(),
		})
	}
	return ks, fs, nil
}

/*
GetTHSDayKlineFull
获取[不复权,前复权,后复权]数据,并补充成交金额数据
前复权,和通达信对的上,和东方财富对不上
后复权,和通达信,东方财富都对不上
*/
func GetTHSDayKlineFull(code string, c *tdx.Client) ([3]protocol.Klines, error) {
	resp, err := c.GetKlineDayAll(code)
	if err != nil {
		return [3]protocol.Klines{}, err
	}
	mAmount := make(map[int64]protocol.Price)
	bfq := protocol.Klines(nil)
	for _, v := range resp.List {
		mAmount[v.Time.Unix()] = v.Amount
		bfq = append(bfq, v)
	}
	//前复权
	qfq, err := GetTHSDayKline(code, THS_QFQ)
	if err != nil {
		return [3]protocol.Klines{}, err
	}
	for i := range qfq {
		qfq[i].Amount = mAmount[qfq[i].Time.Unix()]
	}
	//后复权
	hfq, err := GetTHSDayKline(code, THS_HFQ)
	if err != nil {
		return [3]protocol.Klines{}, err
	}
	for i := range hfq {
		hfq[i].Amount = mAmount[hfq[i].Time.Unix()]
	}
	return [3]protocol.Klines{bfq, qfq, hfq}, nil
}

/*
GetTHSDayKline
前复权,和通达信对的上,和东方财富对不上
后复权,和通达信,东方财富都对不上
*/
func GetTHSDayKline(code string, _type uint8) (protocol.Klines, error) {
	if _type != THS_BFQ && _type != THS_QFQ && _type != THS_HFQ {
		return nil, fmt.Errorf("数据类型错误,例如:不复权0或前复权1或后复权2")
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
	resp, err := http.DefaultClient.Do(req)
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
	sortYears := conv.Interfaces(m["sortYear"])
	priceFactor := conv.Float64(m["priceFactor"])
	prices := strings.Split(conv.String(m["price"]), ",")
	dates := strings.Split(conv.String(m["dates"]), ",")
	volumes := strings.Split(conv.String(m["volumn"]), ",")

	//好像到了22点,总数量会比实际多1
	if total == len(dates)+1 && total == len(volumes)+1 {
		total -= 1
	}
	//判断数量是否对应
	if total*4 != len(prices) || total != len(dates) || total != len(volumes) {
		return nil, fmt.Errorf("total=%d prices=%d dates=%d volumns=%d", total, len(prices), len(dates), len(volumes))
	}

	mYear := make(map[int][]string)
	index := 0
	for i, v := range sortYears {
		if ls := conv.Ints(v); len(ls) == 2 {
			year := conv.Int(ls[0])
			length := conv.Int(ls[1])
			if i == len(sortYears)-1 {
				mYear[year] = dates[index:]
				break
			}
			mYear[year] = dates[index : index+length]
			index += length
		}
	}

	ls := protocol.Klines(nil)
	i := 0
	nowYear := time.Now().Year()
	for year := 1990; year <= nowYear; year++ {
		for _, d := range mYear[year] {
			x, err := time.Parse("0102", d)
			if err != nil {
				return nil, err
			}
			x = time.Date(year, x.Month(), x.Day(), 15, 0, 0, 0, time.Local)
			low := protocol.Price(conv.Float64(prices[i*4+0]) * 1000 / priceFactor)
			ls = append(ls, &protocol.Kline{
				Time:   x,
				Open:   protocol.Price(conv.Float64(prices[i*4+1])*1000/priceFactor) + low,
				High:   protocol.Price(conv.Float64(prices[i*4+2])*1000/priceFactor) + low,
				Low:    low,
				Close:  protocol.Price(conv.Float64(prices[i*4+3])*1000/priceFactor) + low,
				Volume: (conv.Int64(volumes[i]) + 50) / 100,
			})
			i++
		}
	}

	return ls, nil
}
