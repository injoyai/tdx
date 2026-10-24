package spider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"
)

const (
	URLSinaFactorHFQ = "https://finance.sina.com.cn/realstock/company/%s/hfq.js"
	URLSinaFactorQFQ = "https://finance.sina.com.cn/realstock/company/%s/qfq.js"
	SinaQFQ          = "qfq"
	SinaHFQ          = "hfq"
)

func GetSinaFactorFull(code string) (Factors, error) {
	qfq, err := GetSinaFactor(code, SinaQFQ)
	if err != nil {
		return nil, err
	}
	m := make(map[int64]float64)
	for _, v := range qfq {
		m[v.Date] = v.QFactor
	}
	<-time.After(time.Millisecond * 200)
	hfq, err := GetSinaFactor(code, SinaHFQ)
	if err != nil {
		return nil, err
	}
	for i, v := range hfq {
		hfq[i].QFactor = m[v.Date]
	}
	return hfq, nil
}

func GetSinaFactor(code string, adjustment string) (Factors, error) {
	if adjustment != SinaQFQ && adjustment != SinaHFQ {
		return nil, fmt.Errorf("must be qfq or hfq")
	}
	url := fmt.Sprintf(URLSinaFactorHFQ, code)
	if adjustment == SinaQFQ {
		url = fmt.Sprintf(URLSinaFactorQFQ, code)
	}
	resp, err := dataHTTPClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("新浪 HTTP 状态: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	body = bytes.TrimPrefix(body, []byte(fmt.Sprintf("var %s%s=", code, adjustment)))
	body = bytes.Split(body, []byte("/*"))[0]

	data := &sinaFactor{}
	if err := json.Unmarshal(body, data); err != nil {
		return nil, err
	}

	res := make([]*Factor, len(data.Data))
	for i, v := range data.Data {
		if v == nil {
			return nil, fmt.Errorf("新浪复权因子第 %d 条为空", i)
		}
		factor, err := strconv.ParseFloat(v.Factor, 64)
		if err != nil || math.IsNaN(factor) || math.IsInf(factor, 0) {
			return nil, fmt.Errorf("新浪复权因子第 %d 条数值无效: %q", i, v.Factor)
		}
		date, err := time.Parse(time.DateOnly, v.Date)
		if err != nil {
			return nil, err
		}
		f := &Factor{Date: date.Unix()}
		switch adjustment {
		case SinaQFQ:
			f.QFactor = factor
		case SinaHFQ:
			f.HFactor = factor
		}
		res[i] = f
	}

	return res, nil
}

type sinaFactor struct {
	Total int               `json:"total"`
	Data  []*sinaFactorData `json:"data"`
}

type sinaFactorData struct {
	Date   string `json:"d"`
	Factor string `json:"f"`
}
