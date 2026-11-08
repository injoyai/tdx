// Package httpclient 提供从独立元数据 HTTP 服务刷新的代码表与股本缓存。
package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/injoyai/conv"
	"github.com/injoyai/tdx/v2/extend/metadata"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/lib/update"
	"github.com/injoyai/tdx/v2/protocol"
)

var dataHTTPClient = &http.Client{Timeout: 30 * time.Second}

func DialCodesHTTP(address string, spec ...string) (*CodesHTTP, error) {
	c := &CodesHTTP{address: address, Codes: metadata.NewCodes(nil), httpUpdates: newHTTPUpdates()}
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

type CodesHTTP struct {
	httpUpdates
	address string
	*metadata.Codes
}

func (c *CodesHTTP) Update() error {
	ls, err := c.getList("/codes")
	if err != nil {
		return err
	}
	c.Codes.Update(ls)
	return nil
}

func (c *CodesHTTP) getList(path string) (model.CodeModels, error) {
	ls := model.CodeModels{}
	err := c.getJSON(c.address+path, &ls)
	return ls, err
}

func DialGbbqHTTP(address string, spec ...string) (*GbbqHTTP, error) {
	e := &GbbqHTTP{address: address, httpUpdates: newHTTPUpdates()}
	stop, err := update.StartTimer(conv.Default("0 20 9 * * *", spec...), 1, e)
	if err != nil {
		e.Close()
		return nil, err
	}
	e.stop = stop
	return e, nil
}

type GbbqHTTP struct {
	httpUpdates
	address  string
	snapshot *metadata.Gbbq
	mu       sync.RWMutex
}

func (g *GbbqHTTP) Update() error {
	m, err := g.get("/gbbqs")
	if err != nil {
		return err
	}
	var records []*model.Gbbq
	for code, rows := range m {
		key, err := model.StandardCode(code)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row == nil {
				return fmt.Errorf("metadatahttp: nil event for %s", code)
			}
			rowKey, err := model.StandardCode(row.Code)
			if err != nil {
				return err
			}
			if rowKey != key {
				return fmt.Errorf("metadatahttp: event code %q does not match %q", row.Code, code)
			}
		}
		records = append(records, rows...)
	}
	snapshot, err := metadata.NewGbbq(records)
	if err != nil {
		return err
	}
	g.mu.Lock()
	g.snapshot = snapshot
	g.mu.Unlock()
	return nil
}

func (g *GbbqHTTP) query() *metadata.Gbbq {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.snapshot == nil {
		return &metadata.Gbbq{}
	}
	return g.snapshot
}

func (g *GbbqHTTP) GetEquity(code string, t time.Time) *protocol.Equity {
	return g.query().GetEquity(code, t)
}

func (g *GbbqHTTP) Turnover(code string, t time.Time, volume int64) (float64, bool) {
	return g.query().Turnover(code, t, volume)
}

func (g *GbbqHTTP) GetXRXDs(code string) protocol.XRXDs {
	return g.query().GetXRXDs(code)
}

func (g *GbbqHTTP) GetFactors(code string, ks protocol.Klines) []*protocol.Factor {
	return g.query().GetFactors(code, ks)
}

func (g *GbbqHTTP) get(path string) (map[string][]*protocol.Gbbq, error) {
	m := map[string][]*protocol.Gbbq{}
	err := g.getJSON(g.address+path, &m)
	return m, err
}
