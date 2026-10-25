package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

type responseClient struct {
	tdx.Client
	missing bool
}

func (*responseClient) Close() error                            { return nil }
func (*responseClient) GetCount(protocol.Exchange) (int, error) { return 70000, nil }
func (c *responseClient) GetTrade(string, uint16, uint16) (*tdx.TradeResp, error) {
	r := &tdx.TradeResp{Count: 1, List: []tdx.Trade{{}}}
	if !c.missing {
		r.List[0].Standard = &protocol.Trade{Price: 10, Volume: 2}
	}
	return r, nil
}
func (c *responseClient) GetTradeAll(string) (*tdx.TradeResp, error) {
	return c.GetTrade("", 0, 1)
}
func (c *responseClient) GetHistoryTrade(string, string, uint16, uint16) (*tdx.TradeResp, error) {
	return c.GetTrade("", 0, 1)
}
func (c *responseClient) GetHistoryTradeDay(string, string) (*tdx.TradeResp, error) {
	return c.GetTrade("", 0, 1)
}
func (c *responseClient) GetMinute(string) (*tdx.MinuteResp, error) {
	r := &tdx.MinuteResp{Count: 1, List: []tdx.Minute{{}}}
	if !c.missing {
		r.List[0].Standard = &protocol.PriceNumber{Time: "09:31", Price: 10, Number: 2}
	}
	return r, nil
}
func (c *responseClient) GetHistoryMinute(string, string) (*tdx.MinuteResp, error) {
	return c.GetMinute("")
}

func TestHTTPReturnsUnifiedData(t *testing.T) {
	for _, missing := range []bool{false, true} {
		p := handlerPool(t, &responseClient{missing: missing})
		mux := http.NewServeMux()
		(&Server{pool: p}).registerRoutes(mux)
		for _, path := range []string{
			"/trade?code=sz000001&start=0&count=1", "/trade/all?code=sz000001",
			"/trade/history?date=20261020&code=sz000001&start=0&count=1", "/trade/history/day?date=20261020&code=sz000001",
			"/minute?code=sz000001", "/minute/history?date=20261020&code=sz000001",
		} {
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
			var r struct {
				Code int
				Data *struct {
					Count int
					List  []json.RawMessage
				}
			}
			if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil || w.Code != http.StatusOK {
				t.Fatalf("%s: status=%d body=%s err=%v", path, w.Code, w.Body, err)
			}
			if missing {
				if r.Code != 0 || r.Data == nil || r.Data.Count != 1 {
					t.Fatal("unified data requires optional raw data", path, w.Body)
				}
			} else if r.Code != 0 || r.Data == nil || r.Data.Count != 1 || len(r.Data.List) != 1 || string(r.Data.List[0]) == "null" {
				t.Fatal("invalid standard response", path, w.Body)
			}
		}
	}
}

func TestHTTPCountKeepsIntegerRange(t *testing.T) {
	mux := http.NewServeMux()
	(&Server{pool: handlerPool(t, &responseClient{})}).registerRoutes(mux)
	if got := httpData[int](t, mux, "/count?exchange=sz"); got != 70000 {
		t.Fatal("count was narrowed to uint16", got)
	}
}
