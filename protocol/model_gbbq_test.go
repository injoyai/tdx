package protocol

import (
	"testing"
)

func TestXRXD_FQ(t *testing.T) {
	p := Yuan(10)         //10元
	x := XRXD{Fenhong: 5} //10股分红5元
	p2 := x.FQ(p)
	t.Log("复权价:", p2)
}
