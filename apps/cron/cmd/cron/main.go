// Package main is the entrypoint for the price-refresh cron service. It runs
// once per invocation: GET /admin/instruments → fetch quotes from external
// providers → POST /admin/prices → exit. Designed for Railway's cron mode.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	httpTimeout       = 15 * time.Second
	finnhubAPIBase    = "https://finnhub.io/api/v1"
	finnhubMaxConcur  = 8
	finnhubRetries    = 3
)

type config struct {
	APIURL        string
	AdminAPIKey   string
	FinnhubAPIKey string
}

func loadConfig() (config, error) {
	cfg := config{
		APIURL:        strings.TrimRight(os.Getenv("API_URL"), "/"),
		AdminAPIKey:   os.Getenv("ADMIN_API_KEY"),
		FinnhubAPIKey: os.Getenv("FINNHUB_API_KEY"),
	}
	if cfg.APIURL == "" {
		return cfg, fmt.Errorf("API_URL is required")
	}
	if cfg.AdminAPIKey == "" {
		return cfg, fmt.Errorf("ADMIN_API_KEY is required")
	}
	return cfg, nil
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("run failed", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log.Info("cron: start", "api_url", cfg.APIURL)

	insts, err := listInstruments(ctx, cfg)
	if err != nil {
		return fmt.Errorf("list instruments: %w", err)
	}
	log.Info("cron: instruments fetched", "count", len(insts))

	var prices []priceItem

	if cfg.FinnhubAPIKey != "" {
		fh := newFinnhubClient(cfg.FinnhubAPIKey)
		eligible := filterByAssetClass(insts, "us_stock", "us_etf")
		log.Info("cron: querying finnhub", "count", len(eligible))
		got := fh.fetchAll(ctx, eligible, log)
		prices = append(prices, got...)
	} else {
		log.Warn("cron: FINNHUB_API_KEY not set; skipping us_stock/us_etf")
	}

	if len(prices) == 0 {
		log.Info("cron: nothing to upsert")
		return nil
	}

	resp, err := upsertPrices(ctx, cfg, prices)
	if err != nil {
		return fmt.Errorf("upsert prices: %w", err)
	}
	log.Info("cron: done", "updated", resp.Updated, "failed", resp.Failed, "duration", resp.Duration)
	if resp.Failed > 0 {
		for _, e := range resp.Errors {
			log.Warn("cron: upsert error", "msg", e)
		}
	}
	return nil
}

// ---------- API client ----------

type instrumentItem struct {
	ID         uuid.UUID `json:"id"`
	Ticker     string    `json:"ticker"`
	AssetClass string    `json:"assetClass"`
	Currency   string    `json:"currency"`
	Name       string    `json:"name"`
}

type instrumentsResponse struct {
	Items []instrumentItem `json:"items"`
}

type priceItem struct {
	InstrumentID uuid.UUID `json:"instrumentId"`
	Price        string    `json:"price"`
}

type upsertResponse struct {
	Updated  int      `json:"updated"`
	Failed   int      `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
	Duration string   `json:"duration"`
}

func adminClient() *http.Client {
	return &http.Client{Timeout: httpTimeout}
}

func listInstruments(ctx context.Context, cfg config) ([]instrumentItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.APIURL+"/admin/instruments", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AdminAPIKey)
	req.Header.Set("Accept", "application/json")
	res, err := adminClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("admin/instruments: HTTP %d", res.StatusCode)
	}
	var out instrumentsResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return out.Items, nil
}

func upsertPrices(ctx context.Context, cfg config, prices []priceItem) (upsertResponse, error) {
	body, err := json.Marshal(map[string]any{"prices": prices})
	if err != nil {
		return upsertResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.APIURL+"/admin/prices", bytes.NewReader(body))
	if err != nil {
		return upsertResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AdminAPIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := adminClient().Do(req)
	if err != nil {
		return upsertResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return upsertResponse{}, fmt.Errorf("admin/prices: HTTP %d", res.StatusCode)
	}
	var out upsertResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return upsertResponse{}, fmt.Errorf("decode: %w", err)
	}
	return out, nil
}

func filterByAssetClass(insts []instrumentItem, classes ...string) []instrumentItem {
	allowed := make(map[string]struct{}, len(classes))
	for _, c := range classes {
		allowed[c] = struct{}{}
	}
	out := make([]instrumentItem, 0, len(insts))
	for _, i := range insts {
		if _, ok := allowed[i.AssetClass]; ok {
			out = append(out, i)
		}
	}
	return out
}

// ---------- Finnhub ----------

type finnhubClient struct {
	apiKey string
	http   *http.Client
}

func newFinnhubClient(apiKey string) *finnhubClient {
	return &finnhubClient{apiKey: apiKey, http: &http.Client{Timeout: httpTimeout}}
}

type finnhubQuote struct {
	C float64 `json:"c"`  // current price; 0 means symbol unknown
	T int64   `json:"t"`  // unix timestamp (seconds)
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
		// Auth errors are terminal — don't retry.
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
