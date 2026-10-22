package extend

import (
	"os"
	"testing"
)

func TestNewSpiderTHS(t *testing.T) {
	if os.Getenv("TDX_SPIDER_LIVE") != "1" {
		t.Skip("set TDX_SPIDER_LIVE=1 to run live spider tests")
	}
	ls, err := GetTHSDayKline("sz000001", THSHFQ)
	if err != nil {
		t.Error(err)
		return
	}
	for _, v := range ls {
		t.Log(v)
	}
}
