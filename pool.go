package tdx

import (
	"context"
	"errors"
	"sync"

	"github.com/injoyai/base/safe"
)

type (
	ClientPool interface {
		Get() (Client, error)
		Put(c Client)
		Do(fn func(c Client) error) error
		Go(fn func(c Client)) error
	}
	DialPoolFunc = func() (ClientPool, error)
)

// NewPool 简易版本的连接池
func NewPool(dial func() (Client, error), number int) (*Pool, error) {
	if number <= 0 {
		number = 1
	}
	ch := make(chan Client, number)
	p := &Pool{ch: ch, Closer: safe.NewCloser()}
	p.SetCloseFunc(func(error) error {
		p.mu.Lock()
		close(ch)
		p.mu.Unlock()
		var err error
		for c := range ch {
			err = errors.Join(err, c.Close())
		}
		return err
	})
	for i := 0; i < number; i++ {
		c, err := dial()
		if err != nil {
			_ = p.Close()
			return nil, err
		}
		c = clientOrNil(c)
		if c == nil {
			_ = p.Close()
			return nil, errors.New("连接工厂返回空客户端")
		}
		p.ch <- c
	}
	return p, nil
}

type Pool struct {
	ch chan Client
	mu sync.Mutex // 归还与关闭 channel 互斥；借用中的连接归还后再关闭。
	*safe.Closer
}

func (p *Pool) Get() (Client, error) {
	return p.GetContext(context.Background())
}

// GetContext 等待空闲连接；取消后不再借用连接。
func (p *Pool) GetContext(ctx context.Context) (Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.Done():
		return nil, p.Err()
	case c, ok := <-p.ch:
		if !ok {
			return nil, p.Err()
		}
		if err := ctx.Err(); err != nil {
			p.Put(c)
			return nil, err
		}
		select {
		case <-p.Done():
			_ = c.Close()
			return nil, p.Err()
		default:
			return c, nil
		}
	}
}

func (p *Pool) Put(c Client) {
	p.mu.Lock()
	// 已关闭时不能进入发送分支，即使 select 同时发现两个就绪分支。
	select {
	case <-p.Done():
		p.mu.Unlock()
		_ = c.Close()
		return
	default:
	}
	select {
	case <-p.Done():
		p.mu.Unlock()
		_ = c.Close()
	case p.ch <- c:
		p.mu.Unlock()
	}
}

func (p *Pool) Do(fn func(c Client) error) error {
	return p.DoContext(context.Background(), fn)
}

// DoContext 支持取消连接等待；已经开始的 fn 仍由其自身超时控制。
func (p *Pool) DoContext(ctx context.Context, fn func(c Client) error) error {
	c, err := p.GetContext(ctx)
	if err != nil {
		return err
	}
	defer p.Put(c)
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(c)
}

func (p *Pool) Go(fn func(c Client)) error {
	c, err := p.Get()
	if err != nil {
		return err
	}
	go func(c Client) {
		defer p.Put(c)
		fn(c)
	}(c)
	return nil
}
