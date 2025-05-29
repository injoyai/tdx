package extend

import (
	"context"
	"testing"
)

func TestNewSpiderTHS(t *testing.T) {
	x := NewTHSDayKline()
	ls, err := x.Pull(context.Background(), "sz000001", THS_HFQ)
	if err != nil {
		t.Error(err)
		return
	}
	for _, v := range ls {
		t.Log(v)
	}
}
