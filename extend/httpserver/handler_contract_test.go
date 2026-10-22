package httpserver

import (
	"encoding/json"
	"errors"
	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type dedupClient struct {
	tdx.Client
	called, args string
	err          error
	nilData      bool
}

func (*dedupClient) Close() error { return nil }

func handlerPool(t *testing.T, c tdx.Client) *tdx.Pool {
	t.Helper()
	p, err := tdx.NewPool(func() (tdx.Client, error) { return c, nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func (c *dedupClient) GetStockCodeAll() ([]string, error) {
	c.called, c.args = "GetStockCodeAll", ""
	if c.nilData {
		return nil, c.err
	}
	return []string{"stock"}, c.err
}
func (c *dedupClient) GetETFCodeAll() ([]string, error) {
	c.called, c.args = "GetETFCodeAll", ""
	return []string{"etf"}, c.err
}
func (c *dedupClient) GetIndexCodeAll() ([]string, error) {
	c.called, c.args = "GetIndexCodeAll", ""
	return []string{"index"}, c.err
}
func (c *dedupClient) GetZHBFiles() (map[string][]byte, error) {
	c.called, c.args = "GetZHBFiles", ""
	return map[string][]byte{"file": {1, 2}}, c.err
}
func (c *dedupClient) GetTdxZs() ([]*protocol.TdxZs, error) {
	c.called, c.args = "GetTdxZs", ""
	return []*protocol.TdxZs{}, c.err
}
func (c *dedupClient) GetTdxBk() ([]*protocol.TdxBk, error) {
	c.called, c.args = "GetTdxBk", ""
	return []*protocol.TdxBk{}, c.err
}
func (c *dedupClient) GetTdxStat() ([]*protocol.TdxStat, error) {
	c.called, c.args = "GetTdxStat", ""
	return []*protocol.TdxStat{}, c.err
}
func (c *dedupClient) GetTdxStat2() ([]*protocol.TdxStat2, error) {
	c.called, c.args = "GetTdxStat2", ""
	return []*protocol.TdxStat2{}, c.err
}
func (c *dedupClient) GetXgsg() ([]*protocol.TdxXgsg, error) {
	c.called, c.args = "GetXgsg", ""
	return []*protocol.TdxXgsg{}, c.err
}
func (c *dedupClient) GetTdxHy() ([]*protocol.TdxHy, error) {
	c.called, c.args = "GetTdxHy", ""
	return []*protocol.TdxHy{}, c.err
}
func (c *dedupClient) GetSpBlock() ([]*protocol.SpBlock, error) {
	c.called, c.args = "GetSpBlock", ""
	return []*protocol.SpBlock{}, c.err
}
func (c *dedupClient) GetCallAuction(code string) (*protocol.CallAuctionResp, error) {
	c.called, c.args = "GetCallAuction", code
	if c.nilData {
		return nil, c.err
	}
	return &protocol.CallAuctionResp{}, c.err
}
func (c *dedupClient) GetGbbq(code string) (*protocol.GbbqResp, error) {
	c.called, c.args = "GetGbbq", code
	return &protocol.GbbqResp{}, c.err
}
func (c *dedupClient) GetBlockData(file string) ([]*protocol.Block, error) {
	c.called, c.args = "GetBlockData", file
	return []*protocol.Block{}, c.err
}
func (c *dedupClient) GetBlockDataWithIndex(file string) ([]*protocol.Block, error) {
	c.called, c.args = "GetBlockDataWithIndex", file
	return []*protocol.Block{}, c.err
}
func (c *dedupClient) GetBlockFileRaw(file string) ([]byte, error) {
	c.called, c.args = "GetBlockFileRaw", file
	return []byte{1, 2}, c.err
}
func (c *dedupClient) GetReportFile(file string) ([]byte, error) {
	c.called, c.args = "GetReportFile", file
	return []byte{3, 4}, c.err
}
func (c *dedupClient) GetFinanceInfo(ex protocol.Exchange, code string) (*protocol.FinanceInfo, error) {
	c.called, c.args = "GetFinanceInfo", ex.String()+"/"+code
	return &protocol.FinanceInfo{}, c.err
}
func (c *dedupClient) GetCompanyCategory(ex protocol.Exchange, code string) ([]protocol.CompanyCategory, error) {
	c.called, c.args = "GetCompanyCategory", ex.String()+"/"+code
	return []protocol.CompanyCategory{}, c.err
}
func TestHTTPHandlerContracts(t *testing.T) {
	for _, tc := range []struct {
		path, method, args string
		want               any
	}{
		{"/code/stocks", "GetStockCodeAll", "", []string{"stock"}},
		{"/code/etfs", "GetETFCodeAll", "", []string{"etf"}},
		{"/code/indexes", "GetIndexCodeAll", "", []string{"index"}},
		{"/zhb/files", "GetZHBFiles", "", map[string][]byte{"file": {1, 2}}},
		{"/tdx/zs", "GetTdxZs", "", []*protocol.TdxZs{}},
		{"/tdx/bk", "GetTdxBk", "", []*protocol.TdxBk{}},
		{"/tdx/stat", "GetTdxStat", "", []*protocol.TdxStat{}},
		{"/tdx/stat2", "GetTdxStat2", "", []*protocol.TdxStat2{}},
		{"/tdx/xgsg", "GetXgsg", "", []*protocol.TdxXgsg{}},
		{"/tdx/hy", "GetTdxHy", "", []*protocol.TdxHy{}},
		{"/spblock", "GetSpBlock", "", []*protocol.SpBlock{}},
		{"/call_auction?code=sh600000", "GetCallAuction", "sh600000", &protocol.CallAuctionResp{}},
		{"/gbbq?code=sh600000", "GetGbbq", "sh600000", &protocol.GbbqResp{}},
		{"/block/data?file=sample", "GetBlockData", "sample", []*protocol.Block{}},
		{"/block/data/index?file=sample", "GetBlockDataWithIndex", "sample", []*protocol.Block{}},
		{"/block/file?file=sample", "GetBlockFileRaw", "sample", []byte{1, 2}},
		{"/report/file?file=sample", "GetReportFile", "sample", []byte{3, 4}},
		{"/finance?exchange=sz&code=000001", "GetFinanceInfo", "sz/000001", &protocol.FinanceInfo{}},
		{"/company/category?exchange=sz&code=000001", "GetCompanyCategory", "sz/000001", []protocol.CompanyCategory{}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			for _, mode := range []string{"success", "client error", "pool error"} {
				c := &dedupClient{}
				p := handlerPool(t, c)
				if mode == "client error" {
					c.err = errors.New("offline failure")
				}
				if mode == "pool error" {
					p.Close()
				}
				mux := http.NewServeMux()
				(&Server{pool: p}).registerRoutes(mux)
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
				var got struct {
					Code int
					Msg  string
					Data json.RawMessage
				}
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != 200 {
					t.Fatal(w.Code, w.Body.String(), err)
				}
				if mode == "pool error" {
					if c.called != "" {
						t.Fatal("called client after pool error")
					}
				} else if c.called != tc.method || c.args != tc.args {
					t.Fatal(c.called, c.args, tc.method, tc.args)
				}
				if mode == "success" {
					want, err := json.Marshal(tc.want)
					if err != nil || got.Code != 0 || string(got.Data) != string(want) {
						t.Fatal(w.Body.String(), string(want), err)
					}
				} else {
					if got.Code != 1 || string(got.Data) != "null" || got.Msg == "" {
						t.Fatal(w.Body.String())
					}
				}
				t.Logf("fixture %s %s: %s", tc.path, mode, strings.TrimSpace(w.Body.String()))
				if strings.Contains(tc.path, "?") {
					c.called = ""
					bad := httptest.NewRecorder()
					mux.ServeHTTP(bad, httptest.NewRequest("GET", strings.Split(tc.path, "?")[0], nil))
					if bad.Code != 400 || c.called != "" {
						t.Fatal("missing parameters borrowed a connection", bad.Code, c.called)
					}
				}
			}
		})
	}
	for _, path := range []string{"/code/stocks", "/call_auction?code=sh600000"} {
		c := &dedupClient{nilData: true}
		mux := http.NewServeMux()
		(&Server{pool: handlerPool(t, c)}).registerRoutes(mux)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		var got struct {
			Code int
			Data json.RawMessage
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || got.Code != 0 || string(got.Data) != "null" {
			t.Fatal("typed nil changed", w.Body.String(), err)
		}
	}
}
