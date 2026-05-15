package scrydex

import (
	"context"
	"time"
)

type rateLimiter struct {
	ticker *time.Ticker
	ch     <-chan time.Time
}

func newRateLimiter(reqPerSec float64) *rateLimiter {
	interval := time.Duration(float64(time.Second) / reqPerSec)
	t := time.NewTicker(interval)
	return &rateLimiter{ticker: t, ch: t.C}
}

// Wait blocks until a token is available or ctx is cancelled.
func (r *rateLimiter) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.ch:
		return nil
	}
}

func (r *rateLimiter) stop() {
	r.ticker.Stop()
}
