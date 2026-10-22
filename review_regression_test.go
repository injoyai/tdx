package tdx

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestCanceledDial(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, test := range []struct {
		name    string
		factory func([]string) ios.DialFunc
	}{{"host", NewHostDial}, {"random", NewRandomDial}, {"range", NewRangeDial}} {
		t.Run(test.name, func(t *testing.T) {
			c, _, err := test.factory([]string{listener.Addr().String()})(ctx)
			if c != nil {
				defer c.Close()
			}
			if !errors.Is(err, context.Canceled) {
				t.Errorf("canceled context still established TCP connection: error=%v", err)
			}
		})
	}
}

func TestRangeDialCancelsRetryWait(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	start := time.Now()
	c, _, err := NewRangeDial([]string{addr, addr})(ctx)
	cancel()
	if c != nil {
		c.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatal("retry wait ignored context", err, time.Since(start))
	}
}

func TestMalformedResponsesReachCaller(t *testing.T) {
	c := respondingTestClient(t, false, func(h, _ []byte) []byte {
		if binary.LittleEndian.Uint16(h[10:12]) == protocol.TypeQuote {
			return []byte{0, 0, 1, 0}
		}
		return []byte{1, 0}
	})
	for _, f := range []*protocol.Frame{protocol.MCode.Frame(protocol.ExchangeSH, 0), {Control: protocol.Control01, Type: protocol.TypeKline, Data: make([]byte, 28)}, {Control: protocol.Control01, Type: protocol.TypeQuote}} {
		start := time.Now()
		if _, err := c.SendFrame(f, protocol.KlineCache{Type: protocol.TypeKlineDay}); err == nil || time.Since(start) >= 500*time.Millisecond {
			t.Fatal("malformed response was swallowed until timeout", err)
		}
	}
}
