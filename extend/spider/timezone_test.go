package spider

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/protocol"
)

type timezoneDailyClient struct {
	tdx.Client
	k *protocol.Kline
}

func (c *timezoneDailyClient) GetKlineDayAll(string) (*tdx.KlineResp, error) {
	return &tdx.KlineResp{Count: 1, List: []tdx.Kline{{Standard: c.k}}}, nil
}

func TestTHSUsesExchangeTimezone(t *testing.T) {
	old := time.Local
	time.Local = time.UTC
	defer func() { time.Local = old }()
	previous := dataHTTPClient
	defer func() { dataHTTPClient = previous }()
	valid := `callback({"total":1,"sortYear":[[2026,1]],"priceFactor":100,"price":"1000,1,2,3","dates":"0105","volumn":"100"});`
	dataHTTPClient = &http.Client{Transport: dataRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(valid)), Header: make(http.Header)}, nil
	})}
	raw := &protocol.Kline{Time: time.Date(2026, 1, 5, 0, 0, 0, 0, protocol.Location()), Close: 10030, Amount: 987654321}
	full, err := GetTHSDayKlineFull("sz000001", &timezoneDailyClient{k: raw})
	if err != nil {
		t.Fatal(err)
	}
	if full[1][0].Amount != raw.Amount || full[2][0].Amount != raw.Amount {
		t.Errorf("THS amount join: raw=%d, QFQ=%d, HFQ=%d, rawTime=%s, THSTime=%s", raw.Amount, full[1][0].Amount, full[2][0].Amount, raw.Time, full[1][0].Time)
	}
}
