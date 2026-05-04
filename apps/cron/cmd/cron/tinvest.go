package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	tinvestAPIBase     = "https://invest-public-api.tinkoff.ru/rest/tinkoff.public.invest.api.contract.v1"
	tinvestRetries     = 3
	tinvestPriceBatch  = 100 // GetLastPrices accepts up to 200 figis; stay conservative.
	tinvestClassMOEX   = "TQBR"
	tinvestStatusBase  = "INSTRUMENT_STATUS_BASE"
)

type tinvestClient struct {
	token string
	http  *http.Client
}

func newTinvestClient(token string) *tinvestClient {
	return &tinvestClient{token: token, http: &http.Client{Timeout: httpTimeout}}
}

// quotation is Tinkoff's representation of a precise decimal: integer part
// (units, signed string) + 9-digit fractional (nano). Example: 100.5 = {units:"100", nano:500_000_000}.
type quotation struct {
	Units string `json:"units"`
	Nano  int    `json:"nano"`
}

// toPriceString converts a non-negative quotation into a clean decimal string
// suitable for /admin/prices (the api parses with shopspring/decimal). Trailing
// fractional zeros are trimmed.
func (q quotation) toPriceString() string {
	units := q.Units
	if units == "" {
		units = "0"
	}
	if q.Nano <= 0 {
		return units
	}
	frac := strings.TrimRight(fmt.Sprintf("%09d", q.Nano), "0")
	if frac == "" {
		return units
	}
	return units + "." + frac
}

type tinvestShare struct {
	FIGI      string `json:"figi"`
	Ticker    string `json:"ticker"`
	ClassCode string `json:"classCode"`
	Currency  string `json:"currency"`
	Name      string `json:"name"`
}

type tinvestSharesRequest struct {
	InstrumentStatus string `json:"instrumentStatus"`
}

type tinvestSharesResponse struct {
	Instruments []tinvestShare `json:"instruments"`
}

// fetchMoexShares lists every share T-Invest exposes on MOEX main board (TQBR).
// We don't include other class_codes here — they cover non-Russian listings,
// pre-IPO, illiquid boards, etc., none of which we currently want to track.
func (c *tinvestClient) fetchMoexShares(ctx context.Context) ([]tinvestShare, error) {
	var resp tinvestSharesResponse
	if err := c.call(ctx, "InstrumentsService", "Shares",
		tinvestSharesRequest{InstrumentStatus: tinvestStatusBase}, &resp); err != nil {
		return nil, err
	}
	out := make([]tinvestShare, 0, len(resp.Instruments))
	for _, s := range resp.Instruments {
		if !strings.EqualFold(s.ClassCode, tinvestClassMOEX) {
			continue
		}
		if s.FIGI == "" || s.Ticker == "" {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

type tinvestLastPricesRequest struct {
	FIGI []string `json:"figi"`
}

type tinvestLastPrice struct {
	FIGI  string    `json:"figi"`
	Price quotation `json:"price"`
}

type tinvestLastPricesResponse struct {
	LastPrices []tinvestLastPrice `json:"lastPrices"`
}

type ruPriceTarget struct {
	InstrumentID uuid.UUID
	FIGI         string
}

// fetchPrices batches GetLastPrices calls and maps results back to instrument IDs.
// Prices that come back as 0 (no recent trade) are skipped with a warning.
func (c *tinvestClient) fetchPrices(ctx context.Context, targets []ruPriceTarget, log *slog.Logger) []priceItem {
	out := make([]priceItem, 0, len(targets))
	if len(targets) == 0 {
		return out
	}
	idByFIGI := make(map[string]uuid.UUID, len(targets))
	figis := make([]string, 0, len(targets))
	for _, t := range targets {
		if _, dup := idByFIGI[t.FIGI]; dup {
			continue
		}
		idByFIGI[t.FIGI] = t.InstrumentID
		figis = append(figis, t.FIGI)
	}

	for start := 0; start < len(figis); start += tinvestPriceBatch {
		end := start + tinvestPriceBatch
		if end > len(figis) {
			end = len(figis)
		}
		var resp tinvestLastPricesResponse
		if err := c.call(ctx, "MarketDataService", "GetLastPrices",
			tinvestLastPricesRequest{FIGI: figis[start:end]}, &resp); err != nil {
			log.Warn("tinvest: GetLastPrices failed", "batch", start/tinvestPriceBatch, "err", err)
			continue
		}
		for _, p := range resp.LastPrices {
			id, ok := idByFIGI[p.FIGI]
			if !ok {
				continue
			}
			price := p.Price.toPriceString()
			if price == "0" || price == "" {
				log.Warn("tinvest: zero price", "figi", p.FIGI)
				continue
			}
			out = append(out, priceItem{InstrumentID: id, Price: price})
		}
	}
	return out
}

type tinvestErrorResponse struct {
	Code        int    `json:"code"`
	Message     string `json:"message"`
	Description string `json:"description"`
}

// call performs a Tinkoff REST RPC with up to 3 attempts on transient errors
// (5xx / 429 / network). 4xx (other than 429) is terminal.
func (c *tinvestClient) call(ctx context.Context, service, method string, req, resp any) error {
	urlStr := fmt.Sprintf("%s.%s/%s", tinvestAPIBase, service, method)
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < tinvestRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(1<<uint(attempt-1)) * time.Second):
			}
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("new request: %w", err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json")

		res, err := c.http.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("http do: %w", err)
			continue
		}
		respBody, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read body: %w", readErr)
			continue
		}

		switch {
		case res.StatusCode == http.StatusUnauthorized, res.StatusCode == http.StatusForbidden:
			return fmt.Errorf("tinvest %s: HTTP %d (token rejected)", method, res.StatusCode)
		case res.StatusCode == http.StatusTooManyRequests:
			lastErr = fmt.Errorf("tinvest %s: rate limited", method)
			continue
		case res.StatusCode >= 500:
			lastErr = fmt.Errorf("tinvest %s: HTTP %d", method, res.StatusCode)
			continue
		case res.StatusCode >= 400:
			var er tinvestErrorResponse
			_ = json.Unmarshal(respBody, &er)
			msg := er.Message
			if er.Description != "" {
				msg = msg + ": " + er.Description
			}
			if msg == "" {
				msg = string(respBody)
			}
			return fmt.Errorf("tinvest %s: HTTP %d: %s", method, res.StatusCode, msg)
		}

		if resp == nil {
			return nil
		}
		if err := json.Unmarshal(respBody, resp); err != nil {
			return fmt.Errorf("unmarshal: %w", err)
		}
		return nil
	}
	return lastErr
}
