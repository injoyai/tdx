package integration_test

import (
	"encoding/binary"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend"
	"github.com/injoyai/tdx/v2/extend/local"
	"github.com/injoyai/tdx/v2/extend/metadata"
	"github.com/injoyai/tdx/v2/extend/model"
	"github.com/injoyai/tdx/v2/extend/pull"
	_ "github.com/injoyai/tdx/v2/extend/pull/market"
	"github.com/injoyai/tdx/v2/protocol"
)

type consistencyClient struct{ tdx.Client }

func (*consistencyClient) Close() error { return nil }

func (*consistencyClient) GetKlineUntil(category uint8, code string, stop func(tdx.Kline) bool) (*tdx.KlineResp, error) {
	i, _ := protocol.ParseInstrument(code)
	k := &protocol.ExKline{Datetime: "2026-10-20 15:00", Open: 10, High: 10, Low: 10, Close: 10, Position: 100000, Trade: 100, Amount: float64(math.Float32frombits(100000))}
	if !protocol.IsIntraday(category) {
		k.Datetime = k.Datetime[:10]
	}
	return &tdx.KlineResp{Instrument: i, Count: 1, List: []tdx.Kline{{Time: k.Datetime, Open: k.Open, High: k.High, Low: k.Low, Close: k.Close, Volume: 100, Extended: k}}}, nil
}

func consistencyDay(n int) time.Time { return time.Date(2026, 10, n, 15, 0, 0, 0, protocol.Location()) }

func (*consistencyClient) GetGbbqAll() (map[string][]*protocol.Gbbq, error) {
	return map[string][]*protocol.Gbbq{"sz000001": {{Code: "sz000001", Time: consistencyDay(20), Category: 1, C1: 10}}}, nil
}

func (*consistencyClient) GetKlineDayAll(code string) (*tdx.KlineResp, error) {
	i, err := protocol.ParseInstrument(code)
	if err != nil {
		return nil, err
	}
	r := &tdx.KlineResp{Instrument: i}
	for _, d := range []int{19, 20, 21} {
		k := &protocol.Kline{Time: consistencyDay(d), Open: 10, High: 10, Low: 10, Close: 10}
		r.List = append(r.List, tdx.Kline{Time: k.Time.Format(time.DateOnly), Volume: k.Volume, Standard: k})
	}
	r.Count = len(r.List)
	return r, nil
}

func TestConsistencyAdjustmentCodeForms(t *testing.T) {
	g, err := metadata.NewGbbq([]*model.Gbbq{{Code: "sz000001", Time: consistencyDay(20), Category: 1, C1: 10}})
	if err != nil {
		t.Fatal(err)
	}
	response, err := (&consistencyClient{}).GetKlineDayAll("sz000001")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := response.Klines()
	want := g.QFQ("sz000001", raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"SZ000001", "sz.000001", "000001.SZ"} {
		got := g.QFQ(code, raw)
		if err != nil {
			t.Fatal(err)
		}
		if got[0].Close != want[0].Close {
			t.Errorf("%s: first adjusted close=%v; sz000001=%v", code, got[0].Close, want[0].Close)
		}
	}
}

func TestConsistencyCalendarReverseSameWindow(t *testing.T) {
	w, err := metadata.NewWorkday(model.WorkdaySnapshot{Market: protocol.MarketAStock, Source: "fixture", From: "2026-10-19", Through: "2026-10-21", Dates: []string{"2026-10-19", "2026-10-20", "2026-10-21"}})
	if err != nil {
		t.Fatal(err)
	}
	var forward, reverse []string
	for d := range w.Iter(consistencyDay(19), consistencyDay(21)) {
		forward = append(forward, d.Format(time.DateOnly))
	}
	for d := range w.Iter(consistencyDay(19), consistencyDay(21), true) {
		reverse = append(reverse, d.Format(time.DateOnly))
	}
	if !reflect.DeepEqual(forward, []string{"2026-10-19", "2026-10-20"}) {
		t.Fatal(forward)
	}
	if !reflect.DeepEqual(reverse, []string{"2026-10-20", "2026-10-19"}) {
		t.Errorf("forward=%v; reverse=%v", forward, reverse)
	}
	for _, end := range []time.Time{consistencyDay(19).Add(-time.Hour), consistencyDay(19), consistencyDay(19).Add(time.Hour), consistencyDay(21).Add(-time.Hour), consistencyDay(21).Add(time.Nanosecond)} {
		var want []time.Time
		seq := w.Iter(consistencyDay(19), end)
		for at := range seq {
			want = append(want, at)
		}
		var again []time.Time
		for at := range seq {
			again = append(again, at)
		}
		if !reflect.DeepEqual(want, again) {
			t.Fatal("iterator cannot be reused", end)
		}
		for i, j := 0, len(want)-1; i < j; i, j = i+1, j-1 {
			want[i], want[j] = want[j], want[i]
		}
		var got []time.Time
		for at := range w.Iter(consistencyDay(19), end, true) {
			got = append(got, at)
		}
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("window ending %s: reverse=%v want=%v", end, got, want)
		}
	}
}

func TestConsistencyIncomeDateZone(t *testing.T) {
	ks := protocol.Klines{{Time: consistencyDay(19), Close: 10}, {Time: consistencyDay(20), Close: 11}}
	for _, loc := range []*time.Location{protocol.Location(), time.UTC} {
		start := time.Date(2026, 10, 19, 0, 0, 0, 0, loc)
		got, err := extend.DoIncomes(ks, start, 1)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 {
			t.Errorf("start date %s: got %d results, expected 1", start, len(got))
			continue
		}
		if rate, available := got[0].RiseRate(); !available || rate != 0.1 {
			t.Errorf("%s: rate=%v available=%v", loc, rate, available)
		}
	}
}

func TestConsistencyCodeCacheCanonicalOutput(t *testing.T) {
	c, err := metadata.NewCodes(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Update(model.CodeModels{{Exchange: "sz", Code: "000001", Name: "sample"}}); err != nil {
		t.Error(err)
	}
	i, _ := protocol.ParseInstrument("sz000001")
	if got := c.GetName(i.String()); got != "sample" {
		t.Errorf("GetName(%q)=%q; GetName(sz000001)=%q", i.String(), got, c.GetName("sz000001"))
	}
}

func TestConsistencyUnknownExtendedAmountStorage(t *testing.T) {
	p, err := tdx.NewPool(func() (tdx.Client, error) { return &consistencyClient{}, nil }, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	s, err := pull.New(&pull.Config{Store: testStore(t.TempDir()), Codes: []string{"cffIF2609"}, StartAt: "20261019", Day: true, ExPool: p})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	code, err := pull.ParseCode("cffIF2609")
	if err != nil {
		t.Fatal(err)
	}
	u, _ := pull.Get(code.Market.String())
	if err := u.FetchDay(s, code); err != nil {
		t.Fatal(err)
	}
	rows, err := s.QueryDay(code, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatal(len(rows))
	}
	if rows[0].Amount != nil {
		t.Errorf("position bits=100000 stored as monetary Amount=%v", rows[0].Amount)
	}
	if err := u.FetchMinute(s, code); err != nil {
		t.Fatal(err)
	}
	minutes, err := s.QueryMinute(code, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(minutes) != 1 || minutes[0].Amount != nil {
		t.Fatal("unconfirmed minute amount stored", minutes)
	}
}

func TestConsistencyIndexConsumers(t *testing.T) {
	k := &protocol.Kline{Time: consistencyDay(19), Open: 10.8, High: 10.8, Low: 10.8, Close: 10.8, Volume: 20000, Amount: 216000}
	r := &tdx.KlineResp{Instrument: protocol.Instrument{Exchange: protocol.ExchangeSH, Code: "000001"}, List: []tdx.Kline{{Standard: k, Volume: k.Volume}}}
	raw, err := r.StandardResponse()
	if err != nil {
		t.Fatal(err)
	}
	if raw.List[0] != k || raw.List[0].Volume != 20000 {
		t.Fatal("raw HTTP contract changed")
	}
	ks, err := r.Klines()
	if err != nil {
		t.Fatal(err)
	}
	if got := ks.VWAP(); got != 10.8 {
		t.Errorf("20000 shares, amount 216000 yuan: VWAP=%v, want 10.8 yuan", got)
	}
	bs, err := local.WriteDay("sh000001", ks)
	if err != nil {
		t.Fatal(err)
	}
	if k.Volume != 20000 || ks[0] == k {
		t.Fatal("conversion mutated source")
	}
	if got := binary.LittleEndian.Uint32(bs[24:28]); got != 200 {
		t.Errorf("file volume=%d lots, want 200 lots", got)
	}
}
