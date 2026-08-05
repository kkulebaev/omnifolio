package tinvest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kkulebaev/omnifolio/api/internal/source"
)

// TestTLSHandshake hits the real T-Invest endpoint (no token) and asserts the
// TLS layer succeeds: we expect an auth failure (ErrTokenInvalid), never an
// x509 "unknown authority" error. Skips when the network is unavailable.
func TestTLSHandshake(t *testing.T) {
	if testing.Short() {
		t.Skip("network test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c := NewClient()
	err := c.call(ctx, "invalid-token", "OperationsService", "GetPortfolio",
		map[string]string{"accountId": "0"}, nil)

	if err != nil && strings.Contains(err.Error(), "certificate signed by unknown authority") {
		t.Fatalf("TLS verification failed — embedded CA not trusted: %v", err)
	}
	if err != nil && !errors.Is(err, source.ErrTokenInvalid) {
		t.Logf("non-TLS error (acceptable, network/API dependent): %v", err)
	}
}
