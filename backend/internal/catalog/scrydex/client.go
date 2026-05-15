package scrydex

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	defaultBaseURL   = "https://api.scrydex.io"
	defaultReqPerSec = 2.0
	maxAttempts      = 3
	// Initial backoff before first retry. Each subsequent attempt doubles it:
	// attempt 1 → 500ms, attempt 2 → 1000ms.
	baseBackoff = 500 * time.Millisecond
)

// Client is a Scrydex catalog API client with built-in rate limiting and retry.
type Client struct {
	http    *http.Client
	apiKey  string
	teamID  string
	baseURL string
	limiter *rateLimiter
}

// New creates a Client with the given credentials and rate limit.
func New(apiKey, teamID, baseURL string, reqPerSec float64) *Client {
	return &Client{
		http:    &http.Client{Timeout: 30 * time.Second},
		apiKey:  apiKey,
		teamID:  teamID,
		baseURL: baseURL,
		limiter: newRateLimiter(reqPerSec),
	}
}

// NewFromEnv creates a Client from environment variables:
//
//	SCRYDEX_API_KEY   — required
//	SCRYDEX_TEAM      — required
//	SCRYDEX_BASE_URL  — optional, defaults to https://api.scrydex.io
//	SCRYDEX_RATE_LIMIT — optional float, defaults to 2.0 req/s
func NewFromEnv() (*Client, error) {
	apiKey := os.Getenv("SCRYDEX_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("scrydex: SCRYDEX_API_KEY is not set")
	}
	teamID := os.Getenv("SCRYDEX_TEAM")
	if teamID == "" {
		return nil, fmt.Errorf("scrydex: SCRYDEX_TEAM is not set")
	}

	baseURL := os.Getenv("SCRYDEX_BASE_URL")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	reqPerSec := defaultReqPerSec
	if raw := os.Getenv("SCRYDEX_RATE_LIMIT"); raw != "" {
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("scrydex: SCRYDEX_RATE_LIMIT %q is not a valid float: %w", raw, err)
		}
		reqPerSec = v
	}

	return New(apiKey, teamID, baseURL, reqPerSec), nil
}

// ListExpansions fetches all expansions, transparently handling pagination.
func (c *Client) ListExpansions(ctx context.Context) ([]Expansion, error) {
	var all []Expansion
	page := 1
	for {
		url := fmt.Sprintf("%s/pokemon/v1/expansions?page=%d", c.baseURL, page)
		var resp listExpansionsResponse
		if err := c.getJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("scrydex: list expansions page %d: %w", page, err)
		}
		all = append(all, resp.Data...)
		if !hasNextPage(page, resp.PageSize, resp.TotalCount) {
			break
		}
		page++
	}
	return all, nil
}

// ListCards fetches all cards for the given expansion ID, transparently
// handling pagination.
func (c *Client) ListCards(ctx context.Context, expansionID string) ([]Card, error) {
	var all []Card
	page := 1
	for {
		url := fmt.Sprintf("%s/pokemon/v1/expansions/%s/cards?page=%d", c.baseURL, expansionID, page)
		var resp listCardsResponse
		if err := c.getJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("scrydex: list cards for expansion %q page %d: %w", expansionID, page, err)
		}
		all = append(all, resp.Data...)
		if !hasNextPage(page, resp.PageSize, resp.TotalCount) {
			break
		}
		page++
	}
	return all, nil
}

// Close stops the rate limiter's internal ticker, releasing its goroutine.
// Call this when the Client is no longer needed (especially in tests).
func (c *Client) Close() {
	c.limiter.stop()
}

// ─── Internal HTTP helpers ────────────────────────────────────────────────────

// getJSON performs a GET with auth headers, rate limiting, and retry.
// On 429 or 5xx it retries with exponential backoff up to maxAttempts.
func (c *Client) getJSON(ctx context.Context, url string, dst any) error {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return fmt.Errorf("rate limiter: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		// Header casing is exact: X-Api-Key and X-Team-ID (not X-API-Key).
		req.Header.Set("X-Api-Key", c.apiKey)
		req.Header.Set("X-Team-ID", c.teamID)
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("http do (attempt %d): %w", attempt, err)
			log.Warn().Err(lastErr).Str("url", url).Int("attempt", attempt).Msg("scrydex: request failed, retrying")
			if attempt < maxAttempts {
				c.backoff(ctx, attempt)
			}
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			lastErr = fmt.Errorf("http %d (attempt %d)", resp.StatusCode, attempt)
			log.Warn().Int("status", resp.StatusCode).Str("url", url).Int("attempt", attempt).Msg("scrydex: retryable status, retrying")
			if attempt < maxAttempts {
				c.backoff(ctx, attempt)
			}
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("scrydex: GET %s: unexpected status %d: %s", url, resp.StatusCode, truncate(string(body), 200))
		}

		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			resp.Body.Close()
			return fmt.Errorf("scrydex: GET %s: decode response: %w", url, err)
		}
		resp.Body.Close()
		return nil
	}

	return fmt.Errorf("scrydex: GET %s: %w", url, lastErr)
}

// backoff sleeps for baseBackoff * 2^(attempt-1) or until ctx is cancelled.
// attempt=1 → 500ms, attempt=2 → 1000ms, attempt=3 → 2000ms.
func (c *Client) backoff(ctx context.Context, attempt int) {
	delay := baseBackoff * (1 << (attempt - 1))
	select {
	case <-ctx.Done():
	case <-time.After(delay):
	}
}

// hasNextPage returns true when there are more pages to fetch.
// Falls back gracefully when the API omits pagination fields (both zero):
// in that case we treat the first page as the only page.
func hasNextPage(page, pageSize, totalCount int) bool {
	if pageSize <= 0 || totalCount <= 0 {
		return false
	}
	return page*pageSize < totalCount
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
