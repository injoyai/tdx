package integration_test

import (
	"encoding/json"
	"testing"

	"github.com/injoyai/tdx/v2"
	"github.com/injoyai/tdx/v2/extend/manage"
	"github.com/injoyai/tdx/v2/extend/pull"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestJSONUsesCurrentFieldNames(t *testing.T) {
	for _, tc := range []struct {
		value            any
		current, removed string
	}{
		{tdx.Kline{}, "Time", "TradingDate"},
		{pull.KlineDay{}, "Time", "TradingDate"},
		{pull.KlineMinute{}, "Amount", "AmountUnknown"},
		{protocol.Quote{}, "Reserved0", "ReversedBytes0"},
		{pull.Config{}, "Tracker", "Updated"},
		{manage.Manage{}, "ClientPool", "IPool"},
	} {
		data, err := json.Marshal(tc.value)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			t.Fatal(err)
		}
		if _, ok := fields[tc.current]; !ok {
			t.Fatalf("%T is missing %s: %s", tc.value, tc.current, data)
		}
		if _, ok := fields[tc.removed]; ok {
			t.Fatalf("%T still exports %s", tc.value, tc.removed)
		}
	}
}
