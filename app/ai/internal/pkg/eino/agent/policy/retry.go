package policy

import (
	"context"
	"time"
)

type RetryInput struct {
	Attempt int
	Err     error
}

type RetryPolicy interface {
	ShouldRetry(ctx context.Context, input *RetryInput) (bool, error)
}

type RetryPolicyFunc func(ctx context.Context, input *RetryInput) (bool, error)

func (f RetryPolicyFunc) ShouldRetry(ctx context.Context, input *RetryInput) (bool, error) {
	return f(ctx, input)
}

type NoRetryPolicy struct{}

func (p NoRetryPolicy) ShouldRetry(ctx context.Context, input *RetryInput) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return false, nil
}

type FixedRetryPolicy struct {
	MaxAttempts int
	Delay       time.Duration
}

func (p FixedRetryPolicy) ShouldRetry(ctx context.Context, input *RetryInput) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if input == nil || input.Err == nil {
		return false, nil
	}
	maxAttempts := p.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	if input.Attempt >= maxAttempts {
		return false, nil
	}
	if p.Delay <= 0 {
		return true, nil
	}
	timer := time.NewTimer(p.Delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-timer.C:
		return true, nil
	}
}
