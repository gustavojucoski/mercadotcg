package tcgdex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const baseURL = "https://api.tcgdex.net/v2"

// Client makes HTTP calls to the TCGDex REST API.
//
// Rate limiting: a simple ticker-based approach limits calls to ~1 req/s.
// Retry: up to 3 attempts with exponential backoff on 429 or 5xx responses.
type Client struct {
	http    *http.Client
	limiter <-chan time.Time
}

// New creates a TCGDex client. timeout applies to each individual HTTP request.
// The built-in rate limiter fires at 1 req/s to respect the API's recommendation.
func New(timeout time.Duration) *Client {
	// time.Tick is acceptable here because the client lives for the lifetime of
	// the process (import-catalog binary). No goroutine leak in this context.
	return &Client{
		http:    &http.Client{Timeout: timeout},
		limiter: time.Tick(time.Second), //nolint:staticcheck
	}
}

// ListSets returns all sets for the given language (e.g. "en", "pt-br").
// A 404 for a language means no sets are available in that language — returns nil, nil.
func (c *Client) ListSets(ctx context.Context, lang string) ([]SetSummary, error) {
	url := fmt.Sprintf("%s/%s/sets", baseURL, lang)
	var out []SetSummary
	if err := c.get(ctx, url, &out); err != nil {
		return nil, fmt.Errorf("tcgdex list sets (%s): %w", lang, err)
	}
	return out, nil
}

// GetSet returns the full set (including card list) for the given language and set ID.
// Returns nil, nil when the set does not exist in that language (404).
func (c *Client) GetSet(ctx context.Context, lang, setID string) (*Set, error) {
	url := fmt.Sprintf("%s/%s/sets/%s", baseURL, lang, setID)
	var out Set
	found, err := c.getOptional(ctx, url, &out)
	if err != nil {
		return nil, fmt.Errorf("tcgdex get set %s (%s): %w", setID, lang, err)
	}
	if !found {
		return nil, nil
	}
	return &out, nil
}

// GetCard returns the full card details for the given language and card ID (e.g. "sv01-1").
// Returns nil, nil when the card does not exist in that language (404).
func (c *Client) GetCard(ctx context.Context, lang, cardID string) (*Card, error) {
	url := fmt.Sprintf("%s/%s/cards/%s", baseURL, lang, cardID)
	var out Card
	found, err := c.getOptional(ctx, url, &out)
	if err != nil {
		return nil, fmt.Errorf("tcgdex get card %s (%s): %w", cardID, lang, err)
	}
	if !found {
		return nil, nil
	}
	return &out, nil
}

// ----------------------------------------------------------------------------
// Internal HTTP helpers
// ----------------------------------------------------------------------------

// get fetches a URL and decodes JSON into dst. Non-2xx responses are errors.
func (c *Client) get(ctx context.Context, url string, dst any) error {
	_, err := c.getOptional(ctx, url, dst)
	return err
}

// getOptional fetches a URL. Returns (false, nil) on 404, (true, nil) on 2xx,
// and (false, err) on any other error. Retries on 429/5xx with backoff.
func (c *Client) getOptional(ctx context.Context, url string, dst any) (bool, error) {
	const maxAttempts = 3
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Honour the rate limiter before every request.
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-c.limiter:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return false, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("http do (attempt %d): %w", attempt, err)
			if attempt < maxAttempts {
				c.backoff(ctx, attempt)
			}
			continue
		}

		if resp.StatusCode == http.StatusNotFound {
			resp.Body.Close()
			return false, nil
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("http %d from %s (attempt %d)", resp.StatusCode, url, attempt)
			if attempt < maxAttempts {
				c.backoff(ctx, attempt)
			}
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return false, fmt.Errorf("unexpected http %d from %s", resp.StatusCode, url)
		}

		if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
			resp.Body.Close()
			return false, fmt.Errorf("decode response from %s: %w", url, err)
		}
		resp.Body.Close()
		return true, nil
	}

	return false, fmt.Errorf("all %d attempts failed: %w", maxAttempts, lastErr)
}

// backoff sleeps for 2^attempt seconds (1s, 2s, 4s) or until context is cancelled.
func (c *Client) backoff(ctx context.Context, attempt int) {
	delay := time.Duration(1<<attempt) * time.Second
	select {
	case <-ctx.Done():
	case <-time.After(delay):
	}
}
