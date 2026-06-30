package client

import (
	"context"
	"errors"
	"time"
)

var ErrForced = errors.New("mock forced error")

type MockBehavior struct {
	Delay      time.Duration
	ForceError bool
}

func (b MockBehavior) Before(ctx context.Context) error {
	if b.Delay > 0 {
		timer := time.NewTimer(b.Delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	if b.ForceError {
		return ErrForced
	}
	return ctx.Err()
}
