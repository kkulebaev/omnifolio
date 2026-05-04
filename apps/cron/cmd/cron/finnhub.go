package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	finnhubAPIBase   = "https://finnhub.io/api/v1"
	finnhubMaxConcur = 8
	finnhubRetries   = 3
)

type finnhubClient struct {
	apiKey string
	http   *http.Client
}

func newFinnhubClient(apiKey string) *finnhubClient {
	return &finnhubClient{apiKey: apiKey, http: &http.Client{Timeout: httpTimeout}}
}

type finnhubQuote struct {
	C float64 `json:"c"` // current price; 0 means symbol unknown
	T int64   `json:"t"` // unix timestamp (seconds)
}

func (f *finnhubClient) fetchAll(ctx context.Context, insts []instrumentItem, log *slog.Logger) []priceItem {
	out := make([]priceItem, 0, len(insts))
	if len(insts) == 0 {
		return out
	}
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, finnhubMaxConcur)
	)
	for _, inst := range insts {
		symbol := strings.ToUpper(strings.TrimSpace(inst.Ticker))
		if symbol == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(inst instrumentItem, symbol string) {
			defer wg.Done()
			defer func() { <-sem }()

			q, err := f.quote(ctx, symbol)
			if err != nil {
				log.Warn("finnhub: quote failed", "symbol", symbol, "err", err)
				return
			}
			if q.C == 0 {
				log.Warn("finnhub: zero price", "symbol", symbol)
				return
			}
			mu.Lock()
			out = append(out, priceItem{
				InstrumentID: inst.ID,
				Price:        formatFloat(q.C),
			})
			mu.Unlock()
		}(inst, symbol)
	}
	wg.Wait()
	return out
}

func (f *finnhubClient) quote(ctx context.Context, symbol string) (finnhubQuote, error) {
	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("token", f.apiKey)
	urlStr := finnhubAPIBase + "/quote?" + params.Encode()

	var lastErr error
	var out finnhubQuote
	for attempt := 0; attempt < finnhubRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			case <-time.After(time.Duration(1<<uint(attempt-1)) * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return out, err
		}
		req.Header.Set("Accept", "application/json")
		res, err := f.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		func() {
			defer res.Body.Close()
			if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
				lastErr = fmt.Errorf("finnhub auth: HTTP %d", res.StatusCode)
				return
			}
			if res.StatusCode == http.StatusTooManyRequests {
				lastErr = fmt.Errorf("finnhub rate limited")
				return
			}
			if res.StatusCode >= 400 {
				lastErr = fmt.Errorf("finnhub HTTP %d", res.StatusCode)
				return
			}
			if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
				lastErr = fmt.Errorf("decode: %w", err)
				return
			}
			lastErr = nil
		}()
		if lastErr == nil {
			return out, nil
		}
		if strings.Contains(lastErr.Error(), "auth:") {
			return out, lastErr
		}
	}
	return out, lastErr
}

func formatFloat(v float64) string {
	// finnhub returns prices with up to ~4 decimal digits; %g trims trailing zeros.
	return fmt.Sprintf("%g", v)
}
