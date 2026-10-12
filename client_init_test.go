package tdx

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestDialProtocolDefaults(t *testing.T) {
	for _, extended := range []bool{false, true} {
		name := "standard"
		if extended {
			name = "extended"
		}
		t.Run(name, func(t *testing.T) {
			a, b := net.Pipe()
			defer b.Close()
			frames := make(chan []byte, 1)
			done := make(chan struct{})
			go func() {
				defer close(done)
				h, body, err := readTestFrame(b)
				if err != nil {
					return
				}
				frames <- append(h, body...)
				io.Copy(io.Discard, b)
			}()
			dial := func(context.Context) (ios.ReadWriteCloser, string, error) { return a, "offline-init", nil }
			open := DialWith
			want, timeout := protocol.MConnect.Frame().Bytes(), 2*time.Second
			if extended {
				open = dialExHqWith
				want, timeout = protocol.MEx.FrameSetup().Bytes(), 10*time.Second
			}
			c, err := open(dial, WithRedial(false), WithLevel(LevelNone), WithDebug(false))
			if err != nil {
				a.Close()
				b.Close()
				<-done
				t.Fatal(err)
			}
			defer func() { c.CloseAll(); <-done }()
			if got := <-frames; !bytes.Equal(got, want) {
				t.Fatalf("handshake changed: got %x, want %x", got, want)
			}
			if c.waitTimeout != timeout {
				t.Fatalf("timeout=%s, want %s", c.waitTimeout, timeout)
			}
		})
	}
}
