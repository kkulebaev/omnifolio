package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	bybitAPIBase = "https://api.bybit.com"
	bybitRetries = 3
)

type bybitClient struct {
	http *http.Client
}

func newBybitClient() *bybitClient {
	return &bybitClient{http: &http.Client{Timeout: httpTimeout}}
}

type bybitEnvelope struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
}

type bybitInstrument struct {
	Symbol    string `json:"symbol"`
	BaseCoin  string `json:"baseCoin"`
	QuoteCoin string `json:"quoteCoin"`
	Status    string `json:"status"`
}

type bybitInstrumentsResponse struct {
	bybitEnvelope
	Result struct {
		List           []bybitInstrument `json:"list"`
		NextPageCursor string            `json:"nextPageCursor"`
	} `json:"result"`
}

// fetchUSDTSpotInstruments lists every USDT-quoted spot pair currently trading.
// Stablecoins (USDT/USDC/...) are filtered out since the api maps them to
// asset_class=cash, not crypto.
func (b *bybitClient) fetchUSDTSpotInstruments(ctx context.Context) ([]bybitInstrument, error) {
	var out []bybitInstrument
	cursor := ""
	for {
		params := url.Values{}
		params.Set("category", "spot")
		params.Set("limit", "1000")
		if cursor != "" {
			params.Set("cursor", cursor)
		}
		var resp bybitInstrumentsResponse
		if err := b.get(ctx, "/v5/market/instruments-info", params, &resp); err != nil {
			return nil, err
		}
		if resp.RetCode != 0 {
			return nil, fmt.Errorf("bybit instruments-info: retCode=%d msg=%q", resp.RetCode, resp.RetMsg)
		}
		for _, ins := range resp.Result.List {
			if !strings.EqualFold(ins.QuoteCoin, "USDT") {
				continue
			}
			if !strings.EqualFold(ins.Status, "Trading") {
				continue
			}
			base := strings.ToUpper(strings.TrimSpace(ins.BaseCoin))
			if base == "" || isCryptoStablecoin(base) {
				continue
			}
			out = append(out, bybitInstrument{
				Symbol:    strings.ToUpper(ins.Symbol),
				BaseCoin:  base,
				QuoteCoin: "USDT",
				Status:    ins.Status,
			})
		}
		if resp.Result.NextPageCursor == "" {
			break
		}
		cursor = resp.Result.NextPageCursor
	}
	return out, nil
}

type bybitTicker struct {
	Symbol    string `json:"symbol"`
	LastPrice string `json:"lastPrice"`
}

type bybitTickersResponse struct {
	bybitEnvelope
	Result struct {
		List []bybitTicker `json:"list"`
	} `json:"result"`
}

// fetchSpotTickers returns a symbol→lastPrice map for every spot pair in one
// call (the endpoint isn't paginated by symbol).
func (b *bybitClient) fetchSpotTickers(ctx context.Context) (map[string]string, error) {
	params := url.Values{}
	params.Set("category", "spot")
	var resp bybitTickersResponse
	if err := b.get(ctx, "/v5/market/tickers", params, &resp); err != nil {
		return nil, err
	}
	if resp.RetCode != 0 {
		return nil, fmt.Errorf("bybit tickers: retCode=%d msg=%q", resp.RetCode, resp.RetMsg)
	}
	out := make(map[string]string, len(resp.Result.List))
	for _, t := range resp.Result.List {
		out[strings.ToUpper(t.Symbol)] = t.LastPrice
	}
	return out, nil
}

// fetchPrices looks up last prices for the given crypto instruments by deriving
// the Bybit pair as `{ticker}USDT`. Instruments without a matching pair are skipped.
func (b *bybitClient) fetchPrices(ctx context.Context, targets []instrumentItem, log *slog.Logger) []priceItem {
	if len(targets) == 0 {
		return nil
	}
	tickers, err := b.fetchSpotTickers(ctx)
	if err != nil {
		log.Warn("bybit: tickers fetch failed", "err", err)
		return nil
	}
	out := make([]priceItem, 0, len(targets))
	for _, i := range targets {
		sym := strings.ToUpper(strings.TrimSpace(i.Ticker)) + "USDT"
		price, ok := tickers[sym]
		if !ok {
			log.Warn("bybit: pair not listed", "symbol", sym)
			continue
		}
		if price == "" || price == "0" {
			log.Warn("bybit: zero price", "symbol", sym)
			continue
		}
		out = append(out, priceItem{InstrumentID: i.ID, Price: price})
	}
	return out
}

func (b *bybitClient) get(ctx context.Context, path string, params url.Values, resp any) error {
	qs := ""
	if params != nil {
		qs = params.Encode()
	}
	urlStr := bybitAPIBase + path
	if qs != "" {
		urlStr += "?" + qs
	}

	var lastErr error
	for attempt := 0; attempt < bybitRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(1<<uint(attempt-1)) * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return fmt.Errorf("new request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		res, err := b.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("http do: %w", err)
			continue
		}
		body, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read body: %w", readErr)
			continue
		}
		switch {
		case res.StatusCode == http.StatusTooManyRequests:
			lastErr = fmt.Errorf("bybit %s: rate limited", path)
			continue
		case res.StatusCode >= 500:
			lastErr = fmt.Errorf("bybit %s: HTTP %d", path, res.StatusCode)
			continue
		case res.StatusCode >= 400:
			return fmt.Errorf("bybit %s: HTTP %d: %s", path, res.StatusCode, string(body))
		}
		if err := json.Unmarshal(body, resp); err != nil {
			return fmt.Errorf("unmarshal: %w", err)
		}
		return nil
	}
	return lastErr
}

// isCryptoStablecoin matches the api's bybit/position_source list — these
// coins land in asset_class=cash, not crypto.
func isCryptoStablecoin(coin string) bool {
	switch coin {
	case "USDT", "USDC", "BUSD", "TUSD", "DAI", "USD":
		return true
	}
	return false
}
