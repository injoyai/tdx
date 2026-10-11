package protocol

import (
	"testing"
)

func TestHistoryTradeFrame(t *testing.T) {
	f, err := MHistoryTrade.Frame("20241028", "sz000001", 0, 100)
	if err != nil {
		t.Error(err)
		return
	}
	if got, want := f.Bytes().HEX(), "0c000000000112001200b50f84da3401000030303030303100006400"; got != want {
		t.Fatalf("frame=%s, want %s", got, want)
	}
}
