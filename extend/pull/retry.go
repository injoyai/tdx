package pull

import (
	"context"
	"errors"
	"io"
	"net"
	"time"
)

func retryable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, net.ErrClosed) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return true
	}
	var temporary interface{ Temporary() bool }
	return errors.As(err, &temporary) && temporary.Temporary()
}

func (s *Service) retryWait(ctx context.Context, attempt int) error {
	delay := s.cfg.RetryDelay
	if delay <= 0 {
		delay = 5 * time.Second
	}
	for i := 0; i < attempt && delay < 30*time.Second; i++ {
		delay = min(delay*2, 30*time.Second)
	}
	timer := time.NewTimer(min(delay, 30*time.Second))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
