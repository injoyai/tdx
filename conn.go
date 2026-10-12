package tdx

import (
	"context"
	"errors"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/injoyai/base/maps"
	"github.com/injoyai/base/maps/wait"
	"github.com/injoyai/conv"
	"github.com/injoyai/ios"
	"github.com/injoyai/ios/client"
	"github.com/injoyai/ios/module/common"
	"github.com/injoyai/logs"
	"github.com/injoyai/tdx/v2/protocol"
)

const (
	LevelNone  = common.LevelNone
	LevelDebug = common.LevelDebug
	LevelWrite = common.LevelWrite
	LevelRead  = common.LevelRead
	LevelInfo  = common.LevelInfo
	LevelError = common.LevelError
	LevelAll   = common.LevelAll
)

// WithDebug 是否打印通讯数据
func WithDebug(b ...bool) client.Option {
	return func(c *client.Client) {
		c.Logger.Debug(b...)
	}
}

func WithLevel(level int) client.Option {
	return func(c *client.Client) {
		c.Logger.SetLevel(level)
	}
}

// WithRedial 断线重连
func WithRedial(b ...bool) client.Option {
	return func(c *client.Client) {
		c.SetRedial(b...)
	}
}

// dialWith 两类行情共用连接生命周期，仅握手、心跳与默认超时不同。
func dialWith(dial ios.DialFunc, timeout time.Duration, setup, heartbeat func() *protocol.Frame, decode func(*protocol.Response, any) (any, error), op ...client.Option) (cli *conn, err error) {
	cli = &conn{
		Wait:        wait.New(timeout),
		waitTimeout: timeout,
		decode:      decode,
		m:           maps.NewSafe(),
	}

	cli.Client, err = client.Dial(cli.cancelableDial(dial), func(c *client.Client) {
		c.Logger.Debug(true)                           //关闭日志打印
		c.Logger.SetLevel(LevelInfo)                   //设置日志级别
		c.Logger.WithHEX()                             //以HEX显示
		c.SetOption(op...)                             //自定义选项
		c.Event.OnReadFrom = protocol.ReadFrom         //分包
		c.Event.OnDealMessage = cli.handlerDealMessage //解析数据并处理
		c.Event.OnConnected = func(c *client.Client) error {
			w := cli.wrapWriter(c)
			<-cli.writeGate // 连接初始化完成，允许写入。
			if _, err := cli.Write(setup().Bytes()); err != nil {
				c.Close()
				return err
			}
			cli.goHeartbeat(w, c.Closer.Done(), heartbeat().Bytes())
			return nil
		}
	})
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	cli.cancelRun = cancel
	go cli.Client.Run(ctx)

	return cli, nil
}

// conn 只负责传输、请求关联及连接生命周期，业务编解码由具体协议连接提供。
type conn struct {
	decode         func(*protocol.Response, any) (any, error)
	*client.Client              //客户端实例
	ios.MoreWriter              // 稳定写入口，重连只替换内部连接快照。
	Wait           *wait.Entity //内部响应关联；请求超时通过 SetTimeout 配置
	m              *maps.Safe   //有部分解析需要用到代码,返回数据获取不到,固请求的时候缓存下
	msgID          uint32       //消息id,使用SendFrame自动累加
	waitMu         sync.Mutex
	waitTimeout    time.Duration
	writer         *frameWriter
	writeGate      chan struct{}
	cancelRun      context.CancelFunc
}

// Close 取消整个连接生命周期，关闭后不再重连。
func (c *conn) Close() error {
	if c.cancelRun != nil {
		c.cancelRun()
		c.waitMu.Lock()
		w := c.writer
		c.waitMu.Unlock()
		if w != nil && w.conn != nil {
			_ = w.conn.Close() // 解除读取阻塞，由读取协程结束当次连接。
		}
		<-c.Client.Done()
	}
	return c.Client.Close()
}

// handlerDealMessage 处理服务器响应的数据
func (this *conn) handlerDealMessage(c *client.Client, msg ios.Acker) {

	defer func() {
		if e := recover(); e != nil {
			logs.Err(e)
			debug.PrintStack()
		}
	}()

	payload := msg.Payload()
	f, err := protocol.Decode(payload)
	if err != nil {
		logs.Err(err)
		if len(payload) >= 16 {
			key := conv.String(protocol.Uint32(payload[5:9]))
			this.waitMu.Lock()
			this.Wait.Done(key, nil, err)
			this.m.GetAndDel(key)
			this.waitMu.Unlock()
		}
		return
	}
	this.waitMu.Lock()
	defer this.waitMu.Unlock()
	if f.MsgID != 0 && !this.Wait.IsWait(conv.String(f.MsgID)) {
		return // 超时或写入失败的请求已清理，忽略迟到响应。
	}

	//从缓存中获取数据,响应数据中不同类型有不同的处理方式,但是响应无返回该类型,固根据消息id进行缓存
	val, _ := this.m.GetAndDel(conv.String(f.MsgID))

	resp, err := this.decode(f, val)

	if err != nil {
		logs.Err(err)
		this.Wait.Done(conv.String(f.MsgID), nil, err)
		return
	}

	this.Wait.Done(conv.String(f.MsgID), resp)

}

// SetTimeout 设置超时时间
func (this *conn) SetTimeout(t time.Duration) {
	this.waitMu.Lock()
	defer this.waitMu.Unlock()
	this.waitTimeout = t
	this.Wait.SetTimeout(t)
}

// SendFrame 发送数据,并等待响应
func (this *conn) SendFrame(f *protocol.Frame, cache ...any) (any, error) {
	f.MsgID = atomic.AddUint32(&this.msgID, 1)
	key := conv.String(f.MsgID)
	if len(cache) > 0 {
		this.m.Set(key, cache[0])
	}
	type result struct {
		value any
		err   error
	}
	done := make(chan result, 1)
	// 先注册完整回调再发送；注册和响应处理互斥，避免依赖内部的 list 竞争。
	this.waitMu.Lock()
	timeout := this.waitTimeout
	w := this.writer
	this.Wait.Async(key, func(value any, err error) { done <- result{value, err} }, 1, 0)
	this.waitMu.Unlock()
	defer func() {
		this.waitMu.Lock()
		defer this.waitMu.Unlock()
		this.Wait.Done(key, nil)
		this.m.GetAndDel(key)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
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
				return nil, errors.New("超时")
			}
			return nil, err
		}
	case <-ctx.Done():
		return nil, errors.New("超时")
	}
	select {
	case r := <-done:
		return r.value, r.err
	case <-ctx.Done():
		return nil, errors.New("超时")
	}
}

func (c *conn) transport() *conn { return c }
