// Package metadatahttp 提供从独立元数据 HTTP 服务刷新的代码表与股本缓存。
package metadatahttp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/injoyai/conv"
	"github.com/injoyai/tdx/v2/extend/codes"
	"github.com/injoyai/tdx/v2/extend/gbbq"
	query "github.com/injoyai/tdx/v2/internal/gbbq"
	"github.com/injoyai/tdx/v2/internal/metadata"
	"github.com/injoyai/tdx/v2/lib/update"
	"github.com/injoyai/tdx/v2/protocol"
)

var dataHTTPClient = &http.Client{Timeout: 30 * time.Second}

func DialCodesHTTP(address string, spec ...string) (*CodesHTTP, error) {
	c := &CodesHTTP{address: address, Base: codes.NewBase(), httpUpdates: newHTTPUpdates()}
	stop, err := update.StartTimer(conv.Default("0 20 9 * * *", spec...), 1, c)
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

func (h *httpUpdates) getJSON(url string, dest any) error {
	resp, err := h.request(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http code:%d", resp.StatusCode)
	}
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(bs, dest)
}

var _ codes.Interface = (*CodesHTTP)(nil)

type CodesHTTP struct {
	httpUpdates
	address string
	*codes.Base
}

func (c *CodesHTTP) Update() error {
	ls, err := c.getList("/codes")
	if err != nil {
		return err
	}
	c.Base.Update(ls)
	return nil
}

func (c *CodesHTTP) getList(path string) (codes.CodeModels, error) {
	ls := codes.CodeModels{}
	err := c.getJSON(c.address+path, &ls)
	return ls, err
}

func DialGbbqHTTP(address string, spec ...string) (*GbbqHTTP, error) {
	e := &GbbqHTTP{address: address, m: make(map[string][]*protocol.Gbbq), httpUpdates: newHTTPUpdates()}
	stop, err := update.StartTimer(conv.Default("0 20 9 * * *", spec...), 1, e)
	if err != nil {
		e.Close()
		return nil, err
	}
	e.stop = stop
	return e, nil
}

var _ gbbq.Interface = &GbbqHTTP{}

type GbbqHTTP struct {
	httpUpdates
	address string
	m       map[string][]*protocol.Gbbq
	mu      sync.RWMutex
}

func (g *GbbqHTTP) Update() error {
	m, err := g.get("/gbbqs")
	if err != nil {
		return err
	}
	g.mu.Lock()
	g.m = m
	g.mu.Unlock()
	return nil
}

func (g *GbbqHTTP) records(code string) []*protocol.Gbbq {
	key, err := metadata.CodeKey(code)
	if err != nil {
		return nil
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.m[key]
}

func (g *GbbqHTTP) GetEquity(code string, t time.Time) *protocol.Equity {
	return query.EquityAt(g.records(code), t)
}

func (g *GbbqHTTP) GetTurnover(code string, t time.Time, volume int64) float64 {
	return query.Turnover(g.GetEquity(code, t), volume)
}

func (g *GbbqHTTP) GetXRXDs(code string) protocol.XRXDs {
	return query.XRXDs(g.records(code))
}

func (g *GbbqHTTP) GetFactors(code string, ks protocol.Klines) []*protocol.Factor {
	return g.GetXRXDs(code).Pre(ks).Factors()
}

func (g *GbbqHTTP) get(path string) (map[string][]*protocol.Gbbq, error) {
	m := map[string][]*protocol.Gbbq{}
	err := g.getJSON(g.address+path, &m)
	return m, err
}
