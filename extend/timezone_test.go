package extend

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
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

func TestAdaptersUseExchangeTimezone(t *testing.T) {
	old := time.Local
	time.Local = time.UTC
	defer func() { time.Local = old }()
	at := time.Date(2026, 10, 20, 9, 31, 0, 0, protocol.Location())
	data, err := WriteMinute5("sz000001", protocol.Klines{{Time: at, Open: 10000, High: 10000, Low: 10000, Close: 10000, Volume: 1, Amount: 1000000}})
	if err != nil {
		t.Fatal(err)
	}
	base := filepath.Join("..", "output", "unification-optimization", "fixtures")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, "timezone-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	file := filepath.Join(dir, "vipdoc", "sz", "fzline", "sz000001.lc5")
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	local, err := ReadMinute5(dir, "sz000001")
	if err != nil {
		t.Fatal(err)
	}
	day, err := WriteDay("sz000001", protocol.Klines{{Time: at, Open: 10000, High: 10000, Low: 10000, Close: 10000, Volume: 1}})
	if err != nil {
		t.Fatal(err)
	}
	dayfile := filepath.Join(dir, "vipdoc", "sz", "lday", "sz000001.day")
	if err := os.MkdirAll(filepath.Dir(dayfile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dayfile, day, 0600); err != nil {
		t.Fatal(err)
	}
	daily, err := ReadDay(dir, "sz000001")
	if err != nil {
		t.Fatal(err)
	}
	wantDay := time.Date(2026, 10, 20, 0, 0, 0, 0, protocol.Location())
	if len(daily) != 1 || !daily[0].Time.Equal(wantDay) {
		t.Fatalf("local day time=%v, want %s", daily, wantDay)
	}
	online := protocol.GetTime([4]byte(data[:4]), protocol.TypeKline5Minute)
	if !local[0].Time.Equal(online) {
		t.Errorf("same minute clock: local=%s, protocol=%s, Unix delta=%d seconds", local[0].Time, online, local[0].Time.Unix()-online.Unix())
	}
	trades := protocol.Trades{{Time: at, Price: 10000, Volume: 1}}
	bars := trades.Klines()
	if bars[0].Time.Format(time.DateOnly) != at.Format(time.DateOnly) {
		t.Errorf("Trades.Klines: input=%s, first=%s; day changed", at, bars[0].Time)
	}
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
