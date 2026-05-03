// Package tinvest implements PositionSource and PriceProvider for T-Invest
// using their REST API (https://russianinvestments.github.io/investAPI/swagger-ui/).
//
// The REST API mirrors the gRPC contract — endpoints follow
//   POST https://invest-public-api.tinkoff.ru/rest/tinkoff.public.invest.api.contract.v1.<Service>/<Method>
// with JSON body and a Bearer token. We avoid the official Go SDK to skip its
// transitive grpc/protobuf dependencies.
package tinvest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kkulebaev/omnifolio/api/internal/source"
)

const apiBase = "https://invest-public-api.tinkoff.ru/rest/tinkoff.public.invest.api.contract.v1"

// Client is a thin REST wrapper. One instance can serve calls with different
// tokens — token is passed per call.
type Client struct {
	http *http.Client
}

func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 15 * time.Second}}
}

// errorResponse mirrors the gRPC-status payload Tinkoff returns on non-2xx.
type errorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Description string `json:"description"`
}

// call performs a Tinkoff REST RPC with up to 3 attempts on transient errors
// (5xx / network). Maps known statuses to source-level sentinels
// (ErrTokenInvalid, ErrRateLimited).
func (c *Client) call(ctx context.Context, token, service, method string, req, resp any) error {
	url := fmt.Sprintf("%s.%s/%s", apiBase, service, method)
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal req: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(1<<uint(attempt-1)) * time.Second):
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("new request: %w", err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+token)
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "application/json")

		res, err := c.http.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("http do: %w", err)
			continue // network error → retry
		}
		respBody, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read body: %w", readErr)
			continue
		}

		switch {
		case res.StatusCode == http.StatusUnauthorized, res.StatusCode == http.StatusForbidden:
			return source.ErrTokenInvalid
		case res.StatusCode == http.StatusTooManyRequests:
			lastErr = source.ErrRateLimited
			continue // retry with backoff
		case res.StatusCode >= 500:
			lastErr = fmt.Errorf("tinvest %s: HTTP %d", method, res.StatusCode)
			continue // 5xx → retry
		case res.StatusCode >= 400:
			var er errorResponse
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
			return fmt.Errorf("unmarshal resp: %w", err)
		}
		return nil
	}
	return lastErr
}

// Sentinel: distinguishes "not found" (e.g. instrument by FIGI) from other errors.
var ErrNotFound = errors.New("tinvest: not found")
