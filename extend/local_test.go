package extend

import (
	"testing"

	"github.com/injoyai/tdx/protocol"
)

func TestReadDay(t *testing.T) {
	ks, err := ReadDay("D:\\软件\\通达信\\", "sz000001")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("日线数量: %d", len(ks))
	for _, v := range ks[len(ks)-5:] {
		t.Logf("%s 开:%.3f 高:%.3f 低:%.3f 收:%.3f 额:%.0f 量:%d手",
			v.Time.Format("2006-01-02"),
			v.Open.Float64(), v.High.Float64(), v.Low.Float64(), v.Close.Float64(),
			v.Amount.Float64(), v.Volume)
	}
}

func TestReadMinute(t *testing.T) {
	for _, typ := range []int{MinuteType1, MinuteType5} {
		ks, err := ReadMinute("D:\\软件\\通达信\\", "sz000001", typ)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("type=%d 数量: %d", typ, len(ks))
		for _, v := range ks[len(ks)-3:] {
			t.Logf("  %s 开:%.3f 高:%.3f 低:%.3f 收:%.3f 额:%.0f 量:%d手",
				v.Time.Format("2006-01-02 15:04"),
				v.Open.Float64(), v.High.Float64(), v.Low.Float64(), v.Close.Float64(),
				v.Amount.Float64(), v.Volume)
		}
	}
}

func TestReadDayBJ(t *testing.T) {
	ks, err := ReadDay("D:\\软件\\通达信\\", "bj920000")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("北交所日线数量: %d", len(ks))
	if len(ks) > 0 {
		last := ks[len(ks)-1]
		t.Logf("  最新: %s 收:%.3f", last.Time.Format("2006-01-02"), last.Close.Float64())
	}
}

var _ = protocol.ExchangeSZ