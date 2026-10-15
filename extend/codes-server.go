package extend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/injoyai/conv"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

func ListenCodesAndGbbqHTTP(port int, codesOption []tdx.CodesOption, equityOption []tdx.GbbqOption) error {
	code, err := tdx.NewCodes(codesOption...)
	if err != nil {
		return err
	}
	defer code.Close()
	gbbq, err := tdx.NewGbbq(equityOption...)
	if err != nil {
		return err
	}
	defer gbbq.Close()
	logs.Infof("[:%d] 开启HTTP服务...\n", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), codesHandler(code, gbbq))
}

func ListenCodesHTTP(port int, op ...tdx.CodesOption) error {
	code, err := tdx.NewCodes(op...)
	if err != nil {
		return err
	}
	defer code.Close()
	logs.Infof("[:%d] 开启HTTP服务...\n", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), codesHandler(code, nil))
}

func codesHandler(code tdx.ICodes, gbbq *tdx.Gbbq) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data any
		switch r.URL.Path {
		case "/codes", "/all":
			ls := code.GetStocks()
			ls = append(ls, code.GetETFs()...)
			data = append(ls, code.GetIndexes()...)
		case "/stocks":
			data = code.GetStocks()
		case "/etfs":
			data = code.GetETFs()
		case "/indexes":
			data = code.GetIndexes()
		case "/gbbqs":
			if gbbq == nil {
				http.NotFound(w, r)
				return
			}
			data = gbbq.All()
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(conv.Bytes(data))
	})
}

func DialCodesHTTP(address string, spec ...string) (*CodesHTTP, error) {
	c := &CodesHTTP{address: address, CodesBase: tdx.NewCodesBase(), httpUpdates: newHTTPUpdates()}
	stop, err := tdx.StartTimer(conv.Default("0 20 9 * * *", spec...), 1, c)
	if err != nil {
		c.Close()
		return nil, err
	}
	c.stop = stop
	return c, nil
}

// httpUpdates 绑定每个缓存客户端的请求与定时任务，关闭时取消在途请求。
type httpUpdates struct {
	ctx    context.Context
	cancel context.CancelFunc
	stop   func()
	once   sync.Once
}

func newHTTPUpdates() httpUpdates {
	ctx, cancel := context.WithCancel(context.Background())
	return httpUpdates{ctx: ctx, cancel: cancel}
}

func (h *httpUpdates) Close() error {
	h.once.Do(func() {
		if h.cancel != nil {
			h.cancel()
		}
		if h.stop != nil {
			h.stop()
		}
	})
	return nil
}

func (h *httpUpdates) request(url string) (*http.Response, error) {
	ctx := h.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return dataHTTPClient.Do(req)
}

type CodesHTTP struct {
	httpUpdates
	address string
	*tdx.CodesBase
}

func (this *CodesHTTP) Update() error {
	ls, err := this.getList("/codes")
	if err != nil {
		return err
	}
	this.CodesBase.Update(ls)
	return nil
}

func (this *CodesHTTP) getList(path string) (tdx.CodeModels, error) {
	resp, err := this.request(this.address + path)
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

func DialGbbqHTTP(address string, spec ...string) (*GbbqHTTP, error) {
	e := &GbbqHTTP{address: address, m: make(map[string][]*protocol.Gbbq), httpUpdates: newHTTPUpdates()}
	stop, err := tdx.StartTimer(conv.Default("0 20 9 * * *", spec...), 1, e)
	if err != nil {
		e.Close()
		return nil, err
	}
	e.stop = stop
	return e, nil
}

var _ tdx.IGbbq = &GbbqHTTP{}

type GbbqHTTP struct {
	httpUpdates
	address string
	m       map[string][]*protocol.Gbbq
	mu      sync.RWMutex
}

func (this *GbbqHTTP) Update() error {
	m, err := this.get("/gbbqs")
	if err != nil {
		return err
	}
	this.mu.Lock()
	this.m = m
	this.mu.Unlock()
	return nil
}

func (this *GbbqHTTP) GetEquity(code string, t time.Time) *protocol.Equity {
	code = protocol.AddPrefix(code)
	this.mu.RLock()
	ls := this.m[code]
	this.mu.RUnlock()
	for i := len(ls) - 1; i >= 0; i-- {
		v := ls[i]
		if v.IsEquity() && t.Unix() >= tdx.IntegerDay(v.Time).Unix() {
			return v.Equity()
		}
	}
	return nil
}

func (this *GbbqHTTP) GetTurnover(code string, t time.Time, volume int64) float64 {
	x := this.GetEquity(code, t)
	if x == nil {
		return 0
	}
	return x.Turnover(volume)
}

func (this *GbbqHTTP) GetXRXDs(code string) protocol.XRXDs {
	code = protocol.AddPrefix(code)
	this.mu.RLock()
	ls := this.m[code]
	this.mu.RUnlock()
	res := protocol.XRXDs{}
	for _, v := range ls {
		if v.IsXRXD() {
			res = append(res, v.XRXD())
		}
	}
	return res
}

func (this *GbbqHTTP) GetFactors(code string, ks protocol.Klines) []*protocol.Factor {
	return this.GetXRXDs(code).Pre(ks).Factors()
}

func (this *GbbqHTTP) get(path string) (map[string][]*protocol.Gbbq, error) {
	resp, err := this.request(this.address + path)
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
	m := map[string][]*protocol.Gbbq{}
	err = json.Unmarshal(bs, &m)
	return m, err
}
