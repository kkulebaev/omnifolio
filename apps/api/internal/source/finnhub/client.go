// Package finnhub implements a price-only source using Finnhub's REST API
// (https://finnhub.io/docs/api/quote). One quote endpoint per symbol;
// auth is a shared API key, not per-user creds.
package finnhub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/kkulebaev/omnifolio/api/internal/source"
)

const (
	apiBase     = "https://finnhub.io/api/v1"
	httpTimeout = 10 * time.Second
)

type Client struct {
	http   *http.Client
	apiKey string
}

func NewClient(apiKey string) *Client {
	return &Client{
		http:   &http.Client{Timeout: httpTimeout},
		apiKey: apiKey,
	}
}

// quote fetches a single symbol with retry/backoff. Auth errors fail-fast.
func (c *Client) quote(ctx context.Context, symbol string) (quoteResponse, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("token", c.apiKey)
	urlStr := apiBase + "/quote?" + params.Encode()

	var out quoteResponse
	err := c.withRetry(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return nil, fmt.Errorf("new request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		return req, nil
	}, &out)
	return out, err
}

func (c *Client) withRetry(ctx context.Context, build func() (*http.Request, error), resp any) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(1<<uint(attempt-1)) * time.Second):
			}
		}
		req, err := build()
		if err != nil {
			return err
		}
		err = c.do(req, resp)
		if err == nil {
			return nil
		}
		lastErr = err
		if errors.Is(err, source.ErrTokenInvalid) {
			return err
		}
	}
	return lastErr
}

func (c *Client) do(req *http.Request, resp any) error {
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http do: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	switch {
	case res.StatusCode == http.StatusUnauthorized, res.StatusCode == http.StatusForbidden:
		return source.ErrTokenInvalid
	case res.StatusCode == http.StatusTooManyRequests:
		return source.ErrRateLimited
	case res.StatusCode >= 400:
		return fmt.Errorf("finnhub HTTP %d: %s", res.StatusCode, string(body))
	}

	if resp == nil {
		return nil
	}
	if err := json.Unmarshal(body, resp); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	return nil
}
