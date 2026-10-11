package tdx

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/injoyai/base/maps"
	"github.com/injoyai/base/maps/wait"
	"github.com/injoyai/conv"
	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2/protocol"
)

type frameTestWriter func([]byte) (int, error)

func (w frameTestWriter) Write(data []byte) (int, error) { return w(data) }

func TestSendFrameLifecycle(t *testing.T) {
	for _, mode := range []string{"immediate", "timeout", "write failure"} {
		t.Run(mode, func(t *testing.T) {
			c := &Client{Client: client.New(nil), Wait: wait.New(time.Second), m: maps.NewSafe()}
			c.SetTimeout(10 * time.Millisecond)
			c.MoreWriter = ios.NewMoreWrite(frameTestWriter(func(data []byte) (int, error) {
				if mode == "write failure" {
					return 0, io.ErrClosedPipe
				}
				if mode == "immediate" {
					key := conv.String(binary.LittleEndian.Uint32(data[1:5]))
					c.waitMu.Lock()
					c.Wait.Done(key, "ok")
					c.waitMu.Unlock()
				}
				return len(data), nil
			}))
			f := &protocol.Frame{Type: protocol.TypeTrade}
			value, err := c.SendFrame(f, "cached request")
			switch mode {
			case "immediate":
				if err != nil || value != "ok" {
					t.Fatal(value, err)
				}
			case "timeout":
				if err == nil || err.Error() != "超时" {
					t.Fatalf("expected timeout, got %v", err)
				}
			case "write failure":
				if !errors.Is(err, io.ErrClosedPipe) {
					t.Fatalf("expected write failure, got %v", err)
				}
			}
			key := conv.String(f.MsgID)
			if c.Wait.IsWait(key) {
				t.Fatal("request callback leaked")
			}
			if _, ok := c.m.Get(key); ok {
				t.Fatal("request cache leaked")
			}
		})
	}
}

func TestSendFrameConcurrentImmediateResponses(t *testing.T) {
	c := &Client{Client: client.New(nil), Wait: wait.New(time.Second), m: maps.NewSafe()}
	c.SetTimeout(time.Second)
	writer := ios.NewMoreWrite(frameTestWriter(func(data []byte) (int, error) {
		id := binary.LittleEndian.Uint32(data[1:5])
		c.waitMu.Lock()
		c.Wait.Done(conv.String(id), id)
		c.waitMu.Unlock()
		return len(data), nil
	}))
	writer.OnWrite = func(write func() error) error { return write() }
	c.MoreWriter = writer
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f := &protocol.Frame{Type: protocol.TypeCount}
			value, err := c.SendFrame(f)
			if err != nil || value != f.MsgID {
				t.Errorf("response=%v, message ID=%d, error=%v", value, f.MsgID, err)
			}
		}()
	}
	wg.Wait()
}
