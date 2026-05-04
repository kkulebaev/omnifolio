// Package binance implements PositionSource for Binance Spot REST API.
// Auth: HMAC-SHA256 signature passed as a `signature` query parameter, with the
// API key in X-MBX-APIKEY. We hand-roll signing rather than pull a third-party SDK.
package binance

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/kkulebaev/omnifolio/api/internal/source"
)

const (
	apiBase     = "https://api.binance.com"
	recvWindow  = "5000"
	httpTimeout = 15 * time.Second
)

type Client struct {
	http *http.Client
}

func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: httpTimeout}}
}

// signedGet performs an authenticated GET with up to 3 attempts on transient
// errors. Re-signs each attempt to keep the timestamp inside recvWindow.
func (c *Client) signedGet(ctx context.Context, creds Credentials, path string, params url.Values, resp any) error {
	if params == nil {
		params = url.Values{}
	}

	return c.withRetry(ctx, func() (*http.Request, error) {
		// Re-build query each attempt with a fresh timestamp.
		q := url.Values{}
		for k, vs := range params {
			for _, v := range vs {
				q.Add(k, v)
			}
		}
		q.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
		q.Set("recvWindow", recvWindow)
		queryString := q.Encode()

		mac := hmac.New(sha256.New, []byte(creds.APISecret))
		mac.Write([]byte(queryString))
		sig := hex.EncodeToString(mac.Sum(nil))

		urlStr := apiBase + path + "?" + queryString + "&signature=" + sig
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return nil, fmt.Errorf("new request: %w", err)
		}
		req.Header.Set("X-MBX-APIKEY", creds.APIKey)
		req.Header.Set("Accept", "application/json")
		return req, nil
	}, resp)
}

// withRetry runs build+do up to 3 times; backs off on 5xx / network. Auth
// errors fail-fast.
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
	case res.StatusCode == http.StatusTeapot, res.StatusCode == http.StatusTooManyRequests:
		// 418 = IP auto-banned after rate-limit violations; treat as rate-limit.
		return source.ErrRateLimited
	case res.StatusCode >= 400:
		// Try to parse Binance's error envelope to map known codes.
		var env errorEnvelope
		if jsonErr := json.Unmarshal(body, &env); jsonErr == nil && env.Code != 0 {
			if mapped := mapErrorCode(env.Code); mapped != nil {
				return mapped
			}
			return fmt.Errorf("binance HTTP %d: code=%d msg=%q", res.StatusCode, env.Code, env.Msg)
		}
		return fmt.Errorf("binance HTTP %d: %s", res.StatusCode, string(body))
	}

	if resp == nil {
		return nil
	}
	if err := json.Unmarshal(body, resp); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	return nil
}

// mapErrorCode converts Binance's business-level error codes to our source-level sentinels.
// Code list: https://binance-docs.github.io/apidocs/spot/en/#error-codes
func mapErrorCode(code int) error {
	switch code {
	case -1003:
		// TOO_MANY_REQUESTS
		return source.ErrRateLimited
	case -2014, -2015, -1022, -1125:
		// -2014 API-key format invalid
		// -2015 invalid API-key, IP, or permissions for action
		// -1022 signature for this request is not valid
		// -1125 listenKey does not exist (treat as auth)
		return source.ErrTokenInvalid
	}
	return nil
}
