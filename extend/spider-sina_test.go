package extend

import (
	"testing"
	"time"
)

func TestGetSinaFactor(t *testing.T) {
	res, err := GetSinaFactor("sz000001", Sina_HFQ)
	if err != nil {
		t.Fatal(err)
		return
	}
	for _, v := range res {
		t.Log(v)
	}
}

func TestGetSinaFactorFull(t *testing.T) {
	res, err := GetSinaFactorFull("sz000001")
	if err != nil {
		t.Fatal(err)
		return
	}
	for _, v := range res {
		t.Log(v)
	}
	t.Log(res.Get(time.Now().Unix()))
	t.Log(res.Get(time.Now().AddDate(-1, 0, 0).Unix()))
	t.Log(res.Get(time.Now().AddDate(-1, -2, 0).Unix()))
}
