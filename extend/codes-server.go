package extend

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/injoyai/conv"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/lib/gbbq"
	"github.com/robfig/cron/v3"
)

func ListenCodesAndEquityHTTP(port int, codesOption []tdx.CodesOption, equityOption []tdx.EquityOption) error {
	code, err := tdx.NewCodes(codesOption...)
	if err != nil {
		return nil
	}
	equity, err := tdx.NewEquity(equityOption...)
	if err != nil {
		return nil
	}
	succ := func(w http.ResponseWriter, data any) {
		w.WriteHeader(http.StatusOK)
		w.Write(conv.Bytes(data))
	}
	logs.Infof("[:%d] 开启HTTP服务...\n", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.RequestURI {
		case "/all":
			ls := code.GetStocks()
			ls = append(ls, code.GetETFs()...)
			ls = append(ls, code.GetIndexes()...)
			succ(w, ls)
		case "/stocks":
			succ(w, code.GetStocks())
		case "/etfs":
			succ(w, code.GetETFs())
		case "/indexes":
			succ(w, code.GetIndexes())
		case "/equities":
			succ(w, equity.All())
		default:
			http.NotFound(w, r)
		}
	}))
}

func ListenCodesHTTP(port int, op ...tdx.CodesOption) error {
	code, err := tdx.NewCodes(op...)
	if err != nil {
		return nil
	}
	succ := func(w http.ResponseWriter, data any) {
		w.WriteHeader(http.StatusOK)
		w.Write(conv.Bytes(data))
	}
	logs.Infof("[:%d] 开启HTTP服务...\n", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.RequestURI {
		case "/all":
			ls := code.GetStocks()
			ls = append(ls, code.GetETFs()...)
			ls = append(ls, code.GetIndexes()...)
			succ(w, ls)
		case "/stocks":
			succ(w, code.GetStocks())
		case "/etfs":
			succ(w, code.GetETFs())
		case "/indexes":
			succ(w, code.GetIndexes())
		default:
			http.NotFound(w, r)
		}
	}))
}

func DialCodesHTTP(address string, spec ...string) (c *CodesHTTP, err error) {
	c = &CodesHTTP{address: address, CodesBase: tdx.NewCodesBase()}
	cr := cron.New(cron.WithSeconds())
	_spec := conv.Default("0 20 9 * * *", spec...)
	_, err = cr.AddFunc(_spec, func() { logs.PrintErr(c.Update()) })
	if err != nil {
		return
	}
	err = c.Update()
	if err != nil {
		return
	}
	cr.Start()
	return c, nil
}

type CodesHTTP struct {
	address string
	*tdx.CodesBase
}

func (this *CodesHTTP) Update() error {
	ls, err := this.getList("/all")
	if err != nil {
		return err
	}
	this.CodesBase.Update(ls)
	return nil
}

func (this *CodesHTTP) getList(path string) (tdx.CodeModels, error) {
	resp, err := http.DefaultClient.Get(this.address + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http code:%d", resp.StatusCode)
	}
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	ls := tdx.CodeModels{}
	err = json.Unmarshal(bs, &ls)
	return ls, err
}

func DialEquityHTTP(address string, spec ...string) (e *EquityHTTP, err error) {
	e = &EquityHTTP{address: address, m: make(map[string]gbbq.Equities)}
	cr := cron.New(cron.WithSeconds())
	_spec := conv.Default("0 20 9 * * *", spec...)
	_, err = cr.AddFunc(_spec, func() { logs.PrintErr(e.Update()) })
	if err != nil {
		return
	}
	err = e.Update()
	if err != nil {
		return
	}
	cr.Start()
	return e, nil
}

var _ tdx.IEquity = &EquityHTTP{}

type EquityHTTP struct {
	address string
	m       map[string]gbbq.Equities
	mu      sync.RWMutex
}

func (this *EquityHTTP) Update() error {
	m, err := this.get("/equities")
	if err != nil {
		return err
	}
	this.mu.Lock()
	this.m = m
	this.mu.Unlock()
	return nil
}

func (this *EquityHTTP) Get(code string, t time.Time) *gbbq.Equity {
	if len(code) == 8 {
		code = code[2:]
	}
	this.mu.RLock()
	ls := this.m[code]
	this.mu.RUnlock()
	for _, v := range ls {
		if t.Unix() >= v.Date.Unix() {
			return v
		}
	}
	return nil
}

func (this *EquityHTTP) Turnover(code string, t time.Time, volume float64) float64 {
	x := this.Get(code, t)
	if x == nil {
		return 0
	}
	return x.Turnover(volume)
}

func (this *EquityHTTP) get(path string) (map[string]gbbq.Equities, error) {
	resp, err := http.DefaultClient.Get(this.address + path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http code:%d", resp.StatusCode)
	}
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	m := map[string]gbbq.Equities{}
	err = json.Unmarshal(bs, &m)
	return m, err
}
