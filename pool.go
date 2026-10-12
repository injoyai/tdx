package tdx

import (
	"errors"
	"sync"

	"github.com/injoyai/base/safe"
)

type (
	IPool interface {
		Get() (Client, error)
		Put(c Client)
		Do(fn func(c Client) error) error
		Go(fn func(c Client)) error
	}
	DialPoolFunc = func() (IPool, error)
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

func (this *Pool) Get() (Client, error) {
	select {
	case <-this.Done():
		return nil, this.Err()
	case c, ok := <-this.ch:
		if !ok {
			return nil, this.Err()
		}
		select {
		case <-this.Done():
			_ = c.Close()
			return nil, this.Err()
		default:
			return c, nil
		}
	}
}

func (this *Pool) Put(c Client) {
	this.mu.Lock()
	// 已关闭时不能进入发送分支，即使 select 同时发现两个就绪分支。
	select {
	case <-this.Done():
		this.mu.Unlock()
		_ = c.Close()
		return
	default:
	}
	select {
	case <-this.Done():
		this.mu.Unlock()
		_ = c.Close()
	case this.ch <- c:
		this.mu.Unlock()
	}
}

func (this *Pool) Do(fn func(c Client) error) error {
	c, err := this.Get()
	if err != nil {
		return err
	}
	defer this.Put(c)
	return fn(c)
}

func (this *Pool) Go(fn func(c Client)) error {
	c, err := this.Get()
	if err != nil {
		return err
	}
	go func(c Client) {
		defer this.Put(c)
		fn(c)
	}(c)
	return nil
}
