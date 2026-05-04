// Package bybit implements PositionSource for Bybit V5 REST API.
// Auth: HMAC-SHA256 signature in X-BAPI-SIGN header. We hand-roll signing rather
// than pull a third-party SDK.
package bybit

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
	apiBase     = "https://api.bybit.com"
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
	queryString := ""
	if params != nil {
		queryString = params.Encode()
	}
	urlStr := apiBase + path
	if queryString != "" {
		urlStr += "?" + queryString
	}

	return c.withRetry(ctx, func() (*http.Request, error) {
		timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
		payload := timestamp + creds.APIKey + recvWindow + queryString
		mac := hmac.New(sha256.New, []byte(creds.APISecret))
		mac.Write([]byte(payload))
		sig := hex.EncodeToString(mac.Sum(nil))

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return nil, fmt.Errorf("new request: %w", err)
		}
		req.Header.Set("X-BAPI-API-KEY", creds.APIKey)
		req.Header.Set("X-BAPI-TIMESTAMP", timestamp)
		req.Header.Set("X-BAPI-RECV-WINDOW", recvWindow)
		req.Header.Set("X-BAPI-SIGN", sig)
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
		// Retry on rate-limit and unspecified errors; bail on definitive ones.
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
		return fmt.Errorf("bybit HTTP %d: %s", res.StatusCode, string(body))
	}

	// Parse envelope first so retCode/retMsg always surfaces.
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}
	if err := mapRetCode(env.RetCode, env.RetMsg); err != nil {
		return err
	}

	if resp == nil {
		return nil
	}
	if err := json.Unmarshal(body, resp); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	return nil
}

// envelope is Bybit's standard response wrapper. We embed it in concrete result
// structs so do() can detect retCode.
type envelope struct {
	RetCode int    `json:"retCode"`
	RetMsg  string `json:"retMsg"`
}

// mapRetCode converts Bybit's business-level error codes to our source-level sentinels.
func mapRetCode(code int, msg string) error {
	if code == 0 {
		return nil
	}
	switch code {
	case 10003, 10004, 10005, 10006, 10010, 10017:
		// 10003: API key invalid
		// 10004: invalid signature
		// 10005: permission denied
		// 10006: too many visits (rate)
		// 10010: unmatched IP
		// 10017: invalid api-key permissions
		if code == 10006 {
			return source.ErrRateLimited
		}
		return source.ErrTokenInvalid
	}
	return errors.New("bybit: " + msg + " (retCode " + strconv.Itoa(code) + ")")
}
