package tdx

import (
	"bufio"
	"context"
	"io"
	"net"
	"sync"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2/protocol"
)

// frameWriter 串行化完整写入流程，包括底层客户端的统计字段。
type frameWriter struct {
	writer ios.MoreWriter
	conn   io.Closer
	gate   chan struct{}
}

// TCP 重复 Close 会报错，底层库因此跳过断开回调和读缓存清理。
type closeOnceConn struct {
	net.Conn
	once sync.Once
	err  error
}

func (c *closeOnceConn) Close() error {
	c.once.Do(func() { c.err = c.Conn.Close() })
	return c.err
}

func (cn *conn) cancelableDial(dial ios.DialFunc) ios.DialFunc {
	if dial == nil {
		return nil
	}
	cn.writeGate = make(chan struct{}, 1)
	return func(ctx context.Context) (ios.ReadWriteCloser, string, error) {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		r, key, err := dial(ctx)
		if err != nil {
			return r, key, err
		}
		if conn, ok := r.(net.Conn); ok {
			r = &closeOnceConn{Conn: conn}
		}
		// 底层 Dial 在连接回调之前替换共享状态；门控须从此处覆盖到回调。
		select {
		case cn.writeGate <- struct{}{}:
			if ctx.Err() == nil {
				return r, key, nil
			}
			<-cn.writeGate
		case <-ctx.Done():
		}
		_ = r.Close()
		return nil, "", ctx.Err()
	}
}

func (cn *conn) wrapWriter(native *client.Client) *frameWriter {
	// 底层 Close 会立即归还读缓存；独立缓存避免尚未退出的读取与缓存复用竞争。
	if r, ok := native.Origin().(io.Reader); ok {
		reader := bufio.NewReader(r)
		native.Event.OnReadFrom = func(io.Reader) ([]byte, error) { return protocol.ReadFrom(reader) }
	}
	cn.waitMu.Lock()
	if cn.writeGate == nil {
		cn.writeGate = make(chan struct{}, 1)
	}
	w := &frameWriter{writer: native.MoreWriter, conn: native.Origin(), gate: cn.writeGate}
	cn.writer = w
	if cn.MoreWriter == nil {
		outer := ios.NewMoreWrite(ios.WriteFunc(func(bs []byte) (int, error) {
			cn.waitMu.Lock()
			timeout := cn.waitTimeout
			w := cn.writer
			cn.waitMu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			return w.write(ctx, bs)
		}))
		outer.OnWrite = func(write func() error) error { return write() }
		cn.MoreWriter = outer
	}
	cn.waitMu.Unlock()
	return w
}

func (cn *conn) goHeartbeat(w *frameWriter, done <-chan struct{}, bs []byte) {
	// 心跳绑定当次连接的关闭信号，不能读取重连时会被重置的底层 Closer。
	go func() {
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				cn.waitMu.Lock()
				timeout := cn.waitTimeout
				cn.waitMu.Unlock()
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				_, err := w.write(ctx, bs)
				cancel()
				if err != nil {
					return
				}
			}
		}
	}()
}

func (w *frameWriter) write(ctx context.Context, bs []byte) (int, error) {
	select {
	case w.gate <- struct{}{}:
		defer func() { <-w.gate }()
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	return w.writeLocked(ctx, bs)
}

// writeLocked 也用于已经持有初始化门控的握手。
func (w *frameWriter) writeLocked(ctx context.Context, bs []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	// 发送中断可能留下半帧，必须关闭当次连接；排队或等待响应超时不关闭。
	aborted := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		if w.conn != nil {
			_ = w.conn.Close()
		}
		close(aborted)
	})
	n, err := w.writer.Write(bs)
	if !stop() {
		<-aborted
	}
	if ctx.Err() != nil {
		return n, ctx.Err()
	}
	if err == nil && n != len(bs) {
		err = io.ErrShortWrite
	}
	if err != nil && w.conn != nil {
		_ = w.conn.Close()
	}
	return n, err
}
