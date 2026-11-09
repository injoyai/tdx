package tdx

import (
	"context"

	"github.com/injoyai/tdx/v2/protocol"
)

// WithContext returns a request view sharing c's connections and ownership.
// Native clients honor cancellation during requests and between pages without
// changing the original client or reconnect lifecycle. Custom clients may implement
// WithContext(context.Context) Client; otherwise their own request timeouts apply.
// Closing a view closes the shared client; pool borrowers must not close it.
func WithContext(ctx context.Context, c Client) Client {
	if ctx == nil {
		panic("tdx: nil context")
	}
	if bound, ok := c.(interface{ WithContext(context.Context) Client }); ok {
		return bound.WithContext(ctx)
	}
	return c
}

func requestContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

func (c *MarketClient) WithContext(ctx context.Context) Client {
	if ctx == nil {
		panic("tdx: nil context")
	}
	v := &MarketClient{marketState: c.marketState, requestCtx: ctx}
	v.queries = &queries{source: v}
	return v
}
func (c *MarketClient) requestContext() context.Context { return requestContext(c.requestCtx) }

func (c *StConn) WithContext(ctx context.Context) Client {
	if ctx == nil {
		panic("tdx: nil context")
	}
	v := &StConn{conn: c.conn, quoteState: c.quoteState, requestCtx: ctx}
	v.queries = &queries{source: v}
	return v
}
func (c *StConn) requestContext() context.Context { return requestContext(c.requestCtx) }
func (c *StConn) SendFrame(f *protocol.Frame, cache ...any) (any, error) {
	return c.conn.SendFrameContext(c.requestContext(), f, cache...)
}

func (c *ExConn) WithContext(ctx context.Context) Client {
	if ctx == nil {
		panic("tdx: nil context")
	}
	v := &ExConn{conn: c.conn, requestCtx: ctx}
	v.queries = &queries{source: v}
	return v
}
func (c *ExConn) requestContext() context.Context { return requestContext(c.requestCtx) }
func (c *ExConn) SendFrame(f *protocol.Frame, cache ...any) (any, error) {
	return c.conn.SendFrameContext(c.requestContext(), f, cache...)
}

func (q *queries) context() context.Context {
	if source, ok := q.source.(interface{ requestContext() context.Context }); ok {
		return source.requestContext()
	}
	return context.Background()
}
