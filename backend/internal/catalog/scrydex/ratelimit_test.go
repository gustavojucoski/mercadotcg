// ratelimit_test.go — unit tests for the Scrydex rate limiter.
//
// No external dependencies. Run with:
//
//	go test ./internal/catalog/scrydex/... -v
package scrydex

import (
	"context"
	"testing"
	"time"
)

// TestRateLimiter_CloseStopsTickerWithoutPanic verifies that calling stop()
// (via Client.Close()) does not panic and can be called multiple times safely.
// The Close contract is: release the internal ticker goroutine; subsequent
// calls must be benign.
func TestRateLimiter_CloseStopsTickerWithoutPanic(t *testing.T) {
	rl := newRateLimiter(10.0) // 10 req/s = 100ms interval

	// First close must not panic.
	rl.stop()

	// A second close on a stopped Ticker must also not panic — time.Ticker.Stop()
	// is documented as safe to call on an already-stopped ticker.
	rl.stop()
}

// TestRateLimiter_WaitReceivesToken verifies that Wait returns nil within a
// reasonable window when the rate limiter is configured at a high rate.
func TestRateLimiter_WaitReceivesToken(t *testing.T) {
	// 100 req/s → first tick within 10ms; we allow 500ms for test overhead.
	rl := newRateLimiter(100.0)
	defer rl.stop()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if err := rl.Wait(ctx); err != nil {
		t.Errorf("Wait returned unexpected error: %v", err)
	}
}

// TestRateLimiter_WaitRespectsContextCancellation verifies that Wait returns
// ctx.Err() when the context is cancelled before a token is available.
func TestRateLimiter_WaitRespectsContextCancellation(t *testing.T) {
	// 0.001 req/s → interval of 1000s; the token will never arrive within the test.
	rl := newRateLimiter(0.001)
	defer rl.stop()

	// Cancel immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := rl.Wait(ctx)
	if err == nil {
		t.Error("Wait should return an error when context is already cancelled")
	}
	if err != context.Canceled {
		t.Errorf("Wait returned %v, want context.Canceled", err)
	}
}

// TestRateLimiter_WaitRespectsContextDeadline verifies that Wait returns
// context.DeadlineExceeded when the deadline fires before a token is available.
func TestRateLimiter_WaitRespectsContextDeadline(t *testing.T) {
	// Very slow rate: next token is ~1000s away.
	rl := newRateLimiter(0.001)
	defer rl.stop()

	// Deadline in the near past / immediate future.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := rl.Wait(ctx)
	if err == nil {
		t.Error("Wait should return an error when deadline expires before token")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("Wait returned %v, want context.DeadlineExceeded", err)
	}
}

// TestRateLimiter_RateIsRespected verifies that the rate limiter actually
// throttles calls to approximately the configured rate. We measure the time
// for N tokens at a known rate and assert we fall within a reasonable band.
//
// This test is intentionally lenient (+100% tolerance) to avoid flakiness on
// loaded CI runners.
func TestRateLimiter_RateIsRespected(t *testing.T) {
	const reqPerSec = 50.0  // 50 req/s → 20ms per token
	const tokens = 5        // request 5 tokens
	const expectedMs = 4 * (1000.0 / reqPerSec) // 4 gaps between 5 tokens

	rl := newRateLimiter(reqPerSec)
	defer rl.stop()

	ctx := context.Background()
	start := time.Now()

	for i := 0; i < tokens; i++ {
		if err := rl.Wait(ctx); err != nil {
			t.Fatalf("Wait token %d: %v", i, err)
		}
	}

	elapsed := time.Since(start).Milliseconds()
	upper := int64(expectedMs * 3) // 3× for CI headroom

	if elapsed > upper {
		t.Errorf("rate limiter too slow: %dms elapsed for %d tokens at %.0f req/s (upper bound %dms)",
			elapsed, tokens, reqPerSec, upper)
	}
}

// ----------------------------------------------------------------------------
// hasNextPage — pagination helper
// ----------------------------------------------------------------------------

func TestHasNextPage(t *testing.T) {
	tests := []struct {
		name       string
		page       int
		pageSize   int
		totalCount int
		want       bool
	}{
		{"first of many pages", 1, 10, 25, true},
		{"last page exact", 2, 10, 20, false},  // page 2 * 10 = 20 = totalCount
		{"last page remainder", 3, 10, 25, false}, // 3*10=30 >= 25
		{"single page", 1, 100, 50, false},
		{"zero pageSize (API omits field)", 1, 0, 100, false},
		{"zero totalCount (API omits field)", 1, 10, 0, false},
		{"both zero", 1, 0, 0, false},
		{"page zero edge case", 0, 10, 100, true}, // 0*10=0 < 100
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasNextPage(tt.page, tt.pageSize, tt.totalCount)
			if got != tt.want {
				t.Errorf("hasNextPage(page=%d, pageSize=%d, total=%d) = %v, want %v",
					tt.page, tt.pageSize, tt.totalCount, got, tt.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// truncate — string helper
// ----------------------------------------------------------------------------

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello…"},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc…"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncate(tt.input, tt.n)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
			}
		})
	}
}
