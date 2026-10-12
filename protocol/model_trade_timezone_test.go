package protocol

import (
	"encoding/json"
	"testing"
	"time"
)

// Minimal wire fixture with one trade at 10:22 China market wall-clock.
// The 2-byte minute count is 622 (10*60+22). Price delta and counters
// are encoded as zero/one varints; only the timestamp semantics are under test.
func intradayTradeTimeFixture() []byte {
	return []byte{
		1, 0, // trade count
		0x6e, 0x02, // 622 minutes => 10:22
		0, // price delta
		1, // volume
		1, // order count
		0, // trade direction
		0, // reserved
	}
}

func historyTradeTimeFixture() []byte {
	b := []byte{1, 0, 0, 0, 0, 0}            // count + four reserved header bytes
	return append(b, 0x6e, 0x02, 0, 1, 0, 0) // minute, price, volume, side, reserved
}

func assertMarketTime(t *testing.T, when time.Time, tradingDate string) {
	t.Helper()
	want := tradingDate + "T10:22:00+08:00"
	if got := when.Format(time.RFC3339); got != want {
		t.Fatalf("time must preserve exchange wall-clock with +08:00 offset: got %s, want %s", got, want)
	}
	if _, offset := when.Zone(); offset != 8*3600 {
		t.Fatalf("unexpected offset: %d", offset)
	}
	expectedUTC, err := time.Parse(time.RFC3339, tradingDate+"T02:22:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if !when.Equal(expectedUTC) {
		t.Fatalf("wrong instant in UTC: got %s, want %s", when.UTC(), expectedUTC)
	}
	encoded, err := json.Marshal(struct{ Time time.Time }{when})
	if err != nil {
		t.Fatal(err)
	}
	expectedJSON := `{"Time":"` + want + `"}`
	if string(encoded) != expectedJSON {
		t.Fatalf("timestamp serialization: got %s, want %s", encoded, expectedJSON)
	}
}

func TestIntradayTradeDecodeMarketTimezone(t *testing.T) {
	resp, err := (trade{}).Decode(intradayTradeTimeFixture(), TradeCache{
		Date: "20261012",
		Code: "sh600519",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count != 1 || len(resp.List) != 1 {
		t.Fatalf("expected one decoded record: %#v", resp)
	}
	assertMarketTime(t, resp.List[0].Time, "2026-10-12")
}

func TestHistoricalTradeDecodeMarketTimezone(t *testing.T) {
	resp, err := (historyTrade{}).Decode(historyTradeTimeFixture(), TradeCache{
		Date: "20261009",
		Code: "sh600519",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Count != 1 || len(resp.List) != 1 {
		t.Fatalf("expected one decoded record: %#v", resp)
	}
	assertMarketTime(t, resp.List[0].Time, "2026-10-09")
}
