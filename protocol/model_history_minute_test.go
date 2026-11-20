package protocol

import (
	"encoding/binary"
	"testing"
)

func minuteHistoryPayload(count int) []byte {
	bs := make([]byte, 6, 6+count*3)
	binary.LittleEndian.PutUint16(bs, uint16(count))
	for i := 0; i < count; i++ {
		bs = append(bs, 10, 0, 5)
	}
	return bs
}

func TestHistoryMinuteSessionClock(t *testing.T) {
	resp, err := MHistoryMinute.Decode(minuteHistoryPayload(240))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		index int
		want  string
	}{
		{0, "09:31"},
		{119, "11:30"},
		{120, "13:01"},
		{239, "15:00"},
	} {
		if got := resp.List[tc.index].Time; got != tc.want {
			t.Fatal(tc.index, got, tc.want)
		}
	}
}

func TestHistoryMinuteContinuousSession(t *testing.T) {
	continuous := TradingSession{Open: 570}
	resp, err := MHistoryMinute.Decode(minuteHistoryPayload(121), MinuteCache{Session: continuous})
	if err != nil {
		t.Fatal(err)
	}
	if resp.List[120].Time != "11:31" {
		t.Fatal("continuous 121st bar:", resp.List[120].Time)
	}
	// 零值时段无法推导钟面时应明确报错，而不是回退到沪深布局。
	if _, err := MHistoryMinute.Decode(minuteHistoryPayload(1), MinuteCache{Session: TradingSession{}}); err == nil {
		t.Fatal("zero session accepted")
	}
}
