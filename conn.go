package tdx

import (
	"context"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/injoyai/base/maps"
	"github.com/injoyai/base/maps/wait"
	"github.com/injoyai/conv"
	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/tdx/v2/protocol"
)

// WithRedial 断线重连
func WithRedial(b ...bool) client.Option {
	return func(c *client.Client) {
		c.SetRedial(b...)
	}
}

// dialWith 两类行情共用连接生命周期，仅握手、心跳与默认超时不同。
func dialWith(ctx context.Context, dial ios.DialFunc, timeout time.Duration, setup, heartbeat func() *protocol.Frame, decode func(*protocol.Response, any) (any, error), op ...client.Option) (cli *conn, err error) {
	return dialWithContexts(ctx, ctx, dial, timeout, setup, heartbeat, decode, op...)
}

// DefaultDialTimeout bounds initial connection attempts, including retries and setup.
const DefaultDialTimeout = 30 * time.Second

func dialWithContexts(lifecycle, initial context.Context, dial ios.DialFunc, timeout time.Duration, setup, heartbeat func() *protocol.Frame, decode func(*protocol.Response, any) (any, error), op ...client.Option) (cli *conn, err error) {
	ctx := lifecycle
	ctx, cancel := context.WithCancel(ctx)
	connectCtx, stopConnect := context.WithTimeout(initial, DefaultDialTimeout)
	stopLifecycle := context.AfterFunc(ctx, stopConnect)
	defer stopConnect()
	defer stopLifecycle()
	var connecting atomic.Bool
	connecting.Store(true)
	cli = &conn{
		wait:        wait.New(timeout),
		waitTimeout: timeout,
		decode:      decode,
		m:           maps.NewSafe(),
		cancelRun:   cancel,
	}

	cli.native, err = client.DialContext(connectCtx, cli.cancelableDial(dial), func(c *client.Client) {
		c.Logger = newConnectionLogger(nil)
		c.SetOption(op...)                        //自定义选项
		c.Event.OnReadFrom = protocol.ReadFrom    //分包
		c.Event.OnDealMessage = cli.handleMessage //解析数据并处理
		c.Event.OnConnected = func(c *client.Client) error {
			defer func() { <-cli.writeGate }()
			w := cli.wrapWriter(c)
			cli.waitMu.Lock()
			timeout := cli.waitTimeout
			cli.waitMu.Unlock()
			parent := ctx
			if connecting.Load() {
				parent = connectCtx
			}
			handshake, stop := context.WithTimeout(parent, timeout)
			defer stop()
			// 初始化仍持有门控，握手直接写当次连接，完成后才允许业务帧。
			if _, err := w.writeLocked(handshake, setup().Bytes()); err != nil {
				c.Close()
				return err
			}
			cli.goHeartbeat(w, c.Closer.Done(), heartbeat().Bytes())
			return nil
		}
	})
	if err != nil {
		// 两个父上下文可能同时结束；生命周期回调不能把请求的截止错误降为普通取消。
		if initial.Err() != nil {
			err = initial.Err()
		} else if lifecycle.Err() != nil {
			err = lifecycle.Err()
		} else if connectCtx.Err() != nil {
			err = connectCtx.Err()
		}
		cancel()
		return nil, err
	}

	connecting.Store(false)
	go cli.native.Run(ctx)

	return cli, nil
}

// conn 只负责传输、请求关联及连接生命周期，业务编解码由具体协议连接提供。
type conn struct {
	decode      func(*protocol.Response, any) (any, error)
	native      *client.Client // 私有客户端，禁止从具体连接替换协议回调或传输。
	output      ios.MoreWriter // 稳定写入口，重连只替换内部连接快照。
	wait        *wait.Entity   //内部响应关联；请求超时通过 SetTimeout 配置
	m           *maps.Safe     //有部分解析需要用到代码,返回数据获取不到,固请求的时候缓存下
	msgID       uint32         //消息id,使用SendFrame自动累加
	waitMu      sync.Mutex
	waitTimeout time.Duration
	writer      *frameWriter
	writeGate   chan struct{}
	cancelRun   context.CancelFunc
}

// Close 取消整个连接生命周期，关闭后不再重连。
func (cn *conn) Close() error {
	if cn.cancelRun != nil {
		cn.cancelRun()
		cn.waitMu.Lock()
		w := cn.writer
		cn.waitMu.Unlock()
		if w != nil && w.conn != nil {
			_ = w.conn.Close() // 解除读取阻塞，由读取协程结束当次连接。
		}
		<-cn.native.Done()
	}
	return cn.native.Close()
}

// Done 在当前连接断开时关闭；重连后须重新取得信号。
func (cn *conn) Done() <-chan struct{} { return cn.native.Done() }

// Closed 报告当前连接是否已断开，不表示是否允许自动重连。
func (cn *conn) Closed() bool { return cn.native.Closed() }

// Err 返回当前连接的关闭原因。
func (cn *conn) Err() error { return cn.native.Err() }

// GetKey 返回当前连接地址标识。
func (cn *conn) GetKey() string { return cn.native.GetKey() }

// handleMessage 处理服务器响应的数据
func (cn *conn) handleMessage(c *client.Client, msg ios.Acker) {

	defer func() {
		if e := recover(); e != nil {
			cn.logger().Error("处理响应时发生异常", "panic", e, "stack", string(debug.Stack()))
		}
	}()

	payload := msg.Payload()
	f, err := protocol.Decode(payload)
	if err != nil {
		cn.logger().Error("解析响应帧失败", "err", err)
		if len(payload) >= 16 {
			key := conv.String(protocol.Uint32(payload[5:9]))
			cn.waitMu.Lock()
			cn.wait.Done(key, nil, err)
			cn.m.GetAndDel(key)
			cn.waitMu.Unlock()
		}
		return
	}
	cn.waitMu.Lock()
	defer cn.waitMu.Unlock()
	if f.MsgID != 0 && !cn.wait.IsWait(conv.String(f.MsgID)) {
		return // 超时或写入失败的请求已清理，忽略迟到响应。
	}

	//从缓存中获取数据,响应数据中不同类型有不同的处理方式,但是响应无返回该类型,固根据消息id进行缓存
	val, _ := cn.m.GetAndDel(conv.String(f.MsgID))

	resp, err := cn.decode(f, val)

	if err != nil {
		cn.logger().Error("解码行情响应失败", "msg_id", f.MsgID, "err", err)
		cn.wait.Done(conv.String(f.MsgID), nil, err)
		return
	}

	cn.wait.Done(conv.String(f.MsgID), resp)

}

// SetTimeout 设置超时时间
func (cn *conn) SetTimeout(t time.Duration) {
	cn.waitMu.Lock()
	defer cn.waitMu.Unlock()
	cn.waitTimeout = t
	cn.wait.SetTimeout(t)
}

// SendFrame 发送数据,并等待响应
func (cn *conn) SendFrame(f *protocol.Frame, cache ...any) (any, error) {
	return cn.SendFrameContext(context.Background(), f, cache...)
}

// SendFrameContext cancels this request. Aborting a partially written frame closes
// that transport; cancellation while waiting for a response keeps it reusable.
func (cn *conn) SendFrameContext(parent context.Context, f *protocol.Frame, cache ...any) (any, error) {
	if err := parent.Err(); err != nil {
		return nil, err
	}
	f.MsgID = atomic.AddUint32(&cn.msgID, 1)
	key := conv.String(f.MsgID)
	if len(cache) > 0 {
		cn.m.Set(key, cache[0])
	}
	type result struct {
		value any
		err   error
	}
	done := make(chan result, 1)
	// 先注册完整回调再发送；注册和响应处理互斥，避免依赖内部的 list 竞争。
	cn.waitMu.Lock()
	timeout := cn.waitTimeout
	w := cn.writer
	cn.wait.Async(key, func(value any, err error) { done <- result{value, err} }, 1, 0)
	cn.waitMu.Unlock()
	defer func() {
		cn.waitMu.Lock()
		defer cn.waitMu.Unlock()
		cn.wait.Done(key, nil)
		cn.m.GetAndDel(key)
	}()
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	timeoutError := func() error {
		if err := parent.Err(); err != nil {
			return err
		}
		return requestTimeout{}
	}
	data := f.Bytes()
	written := make(chan error, 1)
	go func() {
		_, err := w.write(ctx, data)
		written <- err
	}()
	select {
	case err := <-written:
		if err != nil {
			if ctx.Err() != nil {
				return nil, timeoutError()
			}
			return nil, err
		}
	case <-ctx.Done():
		return nil, timeoutError()
	}
	select {
	case r := <-done:
		return r.value, r.err
	case <-ctx.Done():
		return nil, timeoutError()
	}
}

type requestTimeout struct{}

func (requestTimeout) Error() string { return "超时" }
func (requestTimeout) Unwrap() error { return context.DeadlineExceeded }
func (requestTimeout) Timeout() bool { return true }
