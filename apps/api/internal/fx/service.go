package fx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/storage"
)

var (
	ErrRateUnavailable = errors.New("fx: rate unavailable")
)

// Service provides FX rate lookups. Rates are populated by the cron service
// via POST /admin/fx; this service is read-only.
type Service struct {
	q   *storage.Queries
	log *slog.Logger
}

func NewService(q *storage.Queries, log *slog.Logger) *Service {
	return &Service{q: q, log: log}
}

// GetRate returns the conversion factor: amount in `from` × rate = amount in `to`.
// Special-case: USDT and BUSD treated as USD parity.
func (s *Service) GetRate(ctx context.Context, from, to string) (decimal.Decimal, error) {
	from = normalizeCcy(from)
	to = normalizeCcy(to)
	if from == to {
		return decimal.NewFromInt(1), nil
	}

	// Direct lookup
	if r, err := s.q.GetLatestFxRate(ctx, storage.GetLatestFxRateParams{FromCcy: from, ToCcy: to}); err == nil {
		return r.Rate, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return decimal.Zero, fmt.Errorf("fx lookup direct: %w", err)
	}

	// Inverse lookup
	if r, err := s.q.GetLatestFxRate(ctx, storage.GetLatestFxRateParams{FromCcy: to, ToCcy: from}); err == nil {
		if r.Rate.IsZero() {
			return decimal.Zero, ErrRateUnavailable
		}
		return decimal.NewFromInt(1).Div(r.Rate), nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return decimal.Zero, fmt.Errorf("fx lookup inverse: %w", err)
	}

	// Triangulate via RUB (everything in cbr.ru goes through RUB)
	if from != "RUB" && to != "RUB" {
		fromRUB, err1 := s.q.GetLatestFxRate(ctx, storage.GetLatestFxRateParams{FromCcy: from, ToCcy: "RUB"})
		toRUB, err2 := s.q.GetLatestFxRate(ctx, storage.GetLatestFxRateParams{FromCcy: to, ToCcy: "RUB"})
		if err1 == nil && err2 == nil && !toRUB.Rate.IsZero() {
			return fromRUB.Rate.Div(toRUB.Rate), nil
		}
	}

	return decimal.Zero, ErrRateUnavailable
}

func normalizeCcy(c string) string {
	switch c {
	case "USDT", "BUSD", "USDC":
		return "USD"
	default:
		return c
	}
}
