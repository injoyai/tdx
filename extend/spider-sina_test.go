package extend

import (
	"testing"
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
}
