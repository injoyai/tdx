package tdx

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2/protocol"
)

func TestMarketClientCloseCancelsInitialRetry(t *testing.T) {
	previous, previousEx := Hosts, ExHosts
	listener := localDialListener(t)
	addr := listener.Addr().String()
	listener.Close()
	Hosts, ExHosts = Addrs{addr}, Addrs{addr}
	t.Cleanup(func() { Hosts, ExHosts = previous, previousEx })
	for _, exchange := range []protocol.Exchange{protocol.ExchangeSZ, protocol.ExchangeHK} {
		t.Run(exchange.String(), func(t *testing.T) {
			started := make(chan struct{})
			var once sync.Once
			c := newMarketClient(func(native *client.Client) {
				native.Event.OnReconnect = func(int) (time.Duration, error) {
					once.Do(func() { close(started) })
					return time.Hour, nil
				}
			}, WithDebug(false), WithLevel(LevelNone))
			queryDone := make(chan error, 1)
			go func() { _, err := c.connection(exchange); queryDone <- err }()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("initial retry did not start")
			}
			closeDone := make(chan error, 1)
			go func() { closeDone <- c.Close() }()
			select {
			case err := <-closeDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("Close did not cancel initial retry")
			}
			select {
			case err := <-queryDone:
				if !errors.Is(err, net.ErrClosed) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("initial query did not return")
			}
		})
	}
}

func TestDialContextCancelsHandshake(t *testing.T) {
	for _, ex := range []bool{false, true} {
		t.Run(map[bool]string{false: "standard", true: "extended"}[ex], func(t *testing.T) {
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			dial := func(context.Context) (ios.ReadWriteCloser, string, error) { return a, "pipe", nil }
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			finished := make(chan error, 1)
			go func() {
				var err error
				if ex {
					_, err = dialEx(ctx, dial, WithRedial(false), WithDebug(false))
				} else {
					_, err = dialSt(ctx, dial, WithRedial(false), WithDebug(false))
				}
				finished <- err
			}()
			select {
			case err := <-finished:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal("handshake ignored context", err)
				}
			case <-time.After(time.Second):
				t.Fatal("handshake hung after cancellation")
			}
		})
	}
}

func TestHandshakeBeforeConcurrentWrites(t *testing.T) {
	for _, ex := range []bool{false, true} {
		t.Run(map[bool]string{false: "standard", true: "extended"}[ex], func(t *testing.T) {
			var dials, setups atomic.Int32
			var servers sync.WaitGroup
			first := make(chan net.Conn, 1)
			frames := make(chan uint16, 8)
			started, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			setup := func() *protocol.Frame {
				if setups.Add(1) == 2 {
					close(started)
					<-release
				}
				if ex {
					return protocol.MEx.FrameSetup()
				}
				return protocol.MConnect.Frame()
			}
			dial := func(context.Context) (ios.ReadWriteCloser, string, error) {
				a, b := net.Pipe()
				n := dials.Add(1)
				if n == 1 {
					first <- b
				}
				servers.Add(1)
				go func() {
					defer servers.Done()
					defer b.Close()
					for {
						h, _, err := readTestFrame(b)
						if err != nil {
							return
						}
						if n > 1 {
							frames <- binary.LittleEndian.Uint16(h[10:12])
						}
					}
				}()
				return a, "pipe", nil
			}
			c, err := dialWith(context.Background(), dial, time.Second, setup, protocol.MHeart.Frame, func(*protocol.Response, any) (any, error) { return nil, nil }, WithRedial(), WithDebug(false))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { once.Do(func() { close(release) }); c.Close(); servers.Wait() })
			(<-first).Close()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("reconnect did not reach handshake")
			}
			written := make(chan error, 1)
			go func() {
				_, err := c.Write((&protocol.Frame{MsgID: 1, Type: protocol.TypeCount}).Bytes())
				written <- err
			}()
			select {
			case typ := <-frames:
				t.Fatalf("frame %04x escaped before handshake", typ)
			case <-time.After(50 * time.Millisecond):
			}
			once.Do(func() { close(release) })
			want := uint16(protocol.TypeConnect)
			if ex {
				want = protocol.TypeExSetup
			}
			for _, typ := range []uint16{want, protocol.TypeCount} {
				select {
				case got := <-frames:
					if got != typ {
						t.Fatalf("frame=%04x want=%04x", got, typ)
					}
				case <-time.After(time.Second):
					t.Fatal("missing frame")
				}
			}
			if err := <-written; err != nil {
				t.Fatal(err)
			}
		})
	}
}
