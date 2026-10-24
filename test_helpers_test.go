package tdx

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
)

// testClient lets protocol-independent transport tests inspect the shared core.
type testClient struct {
	protocolClient
	*conn
}

func (c *testClient) Close() error { return c.protocolClient.Close() }

func (c *testClient) SetTimeout(t time.Duration) { c.protocolClient.SetTimeout(t) }

func (c *testClient) Done() <-chan struct{} { return c.conn.Done() }

func testDialWith(dial ios.DialFunc, ex bool, options ...client.Option) (*testClient, error) {
	var c protocolClient
	var err error
	if ex {
		c, err = dialExWith(dial, options...)
	} else {
		c, err = DialWith(dial, options...)
	}
	if err != nil {
		return nil, err
	}
	core := c.(interface{ transport() *conn }).transport()
	return &testClient{protocolClient: c, conn: core}, nil
}

func readTestFrame(r io.Reader) ([]byte, []byte, error) {
	h := make([]byte, 12)
	if _, err := io.ReadFull(r, h); err != nil {
		return nil, nil, err
	}
	n := int(binary.LittleEndian.Uint16(h[6:8])) - 2
	if n < 0 {
		return nil, nil, io.ErrUnexpectedEOF
	}
	b := make([]byte, n)
	_, err := io.ReadFull(r, b)
	return h, b, err
}

func writeTestResponse(w io.Writer, h, data []byte) error {
	r := make([]byte, 16+len(data))
	copy(r, []byte{0xb1, 0xcb, 0x74, 0, 0x1c})
	copy(r[5:9], h[1:5])
	copy(r[10:12], h[10:12])
	binary.LittleEndian.PutUint16(r[12:14], uint16(len(data)))
	binary.LittleEndian.PutUint16(r[14:16], uint16(len(data)))
	copy(r[16:], data)
	_, err := w.Write(r)
	return err
}

func pipeTestClient(t *testing.T, ex bool, serve func(net.Conn), options ...client.Option) *testClient {
	t.Helper()
	a, b := net.Pipe()
	serverDone := make(chan struct{})
	go func() { defer close(serverDone); serve(b) }()
	dial := func(context.Context) (ios.ReadWriteCloser, string, error) { return a, "offline-test", nil }
	var c *testClient
	var err error
	op := append([]client.Option{WithRedial(false), WithLevel(LevelNone)}, options...)
	if ex {
		c, err = testDialWith(dial, true, op...)
	} else {
		c, err = testDialWith(dial, false, op...)
	}
	if err != nil {
		a.Close()
		b.Close()
		t.Fatal(err)
	}
	c.Logger.Debug(false)
	c.SetTimeout(time.Second)
	t.Cleanup(func() {
		c.Close()
		b.Close()
		<-serverDone
	})
	return c
}

func respondingTestClient(t *testing.T, ex bool, data func([]byte, []byte) []byte) *testClient {
	return pipeTestClient(t, ex, func(s net.Conn) {
		for {
			h, b, err := readTestFrame(s)
			if err != nil {
				return
			}
			if binary.LittleEndian.Uint32(h[1:5]) == 0 {
				continue // 握手与心跳没有等待者。
			}
			if err := writeTestResponse(s, h, data(h, b)); err != nil {
				return
			}
		}
	})
}

func (c *strictCloseConn) Close() error {
	if c.closes.Add(1) > 1 {
		return net.ErrClosed
	}
	return c.Conn.Close()
}
