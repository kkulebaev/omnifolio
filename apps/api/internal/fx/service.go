package fx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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
	rate, _, err := s.resolve(ctx, from, to, endOfTime, func(ctx context.Context, from, to string) (storage.FxRate, error) {
		return s.q.GetLatestFxRate(ctx, storage.GetLatestFxRateParams{FromCcy: from, ToCcy: to})
	})
	return rate, err
}

// GetRateAt is GetRate as of a historical date: it uses the latest rate on or
// before `at`, falling back to the nearest later one. approximated reports
// that fallback, i.e. some rate used is dated after `at`.
func (s *Service) GetRateAt(ctx context.Context, from, to string, at time.Time) (rate decimal.Decimal, approximated bool, err error) {
	day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
	rate, newest, err := s.resolve(ctx, from, to, day, func(ctx context.Context, from, to string) (storage.FxRate, error) {
		return s.q.GetFxRateAt(ctx, storage.GetFxRateAtParams{
			FromCcy: from,
			ToCcy:   to,
			At:      pgtype.Date{Time: day, Valid: true},
		})
	})
	if err != nil {
		return decimal.Zero, false, err
	}
	return rate, newest.After(day), nil
}

var endOfTime = time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)

type lookupFunc func(ctx context.Context, from, to string) (storage.FxRate, error)

type candidate struct {
	rate   decimal.Decimal
	newest time.Time
}

// resolve tries direct → inverse → RUB-triangulation using the given row
// lookup and returns the first candidate whose rows are all dated on or before
// exactBy. When every route only has later rates, the first one found wins.
// newest is the latest date among the rows used (zero for parity).
func (s *Service) resolve(ctx context.Context, from, to string, exactBy time.Time, lookup lookupFunc) (decimal.Decimal, time.Time, error) {
	from = normalizeCcy(from)
	to = normalizeCcy(to)
	if from == to {
		return decimal.NewFromInt(1), time.Time{}, nil
	}

	var fallback *candidate
	accept := func(c candidate) bool {
		if !c.newest.After(exactBy) {
			return true
		}
		if fallback == nil {
			fallback = &c
		}
		return false
	}
	find := func(from, to string) (storage.FxRate, bool, error) {
		r, err := lookup(ctx, from, to)
		if errors.Is(err, pgx.ErrNoRows) {
			return r, false, nil
		}
		if err != nil {
			return r, false, fmt.Errorf("fx lookup %s→%s: %w", from, to, err)
		}
		return r, true, nil
	}

	r, ok, err := find(from, to)
	if err != nil {
		return decimal.Zero, time.Time{}, err
	}
	if ok {
		if c := (candidate{r.Rate, r.Date.Time}); accept(c) {
			return c.rate, c.newest, nil
		}
	}

	r, ok, err = find(to, from)
	if err != nil {
		return decimal.Zero, time.Time{}, err
	}
	if ok && !r.Rate.IsZero() {
		if c := (candidate{decimal.NewFromInt(1).Div(r.Rate), r.Date.Time}); accept(c) {
			return c.rate, c.newest, nil
		}
	}

	// Everything in cbr.ru goes through RUB.
	if from != "RUB" && to != "RUB" {
		fromRUB, ok1, err := find(from, "RUB")
		if err != nil {
			return decimal.Zero, time.Time{}, err
		}
		toRUB, ok2, err := find(to, "RUB")
		if err != nil {
			return decimal.Zero, time.Time{}, err
		}
		if ok1 && ok2 && !toRUB.Rate.IsZero() {
			newest := fromRUB.Date.Time
			if toRUB.Date.Time.After(newest) {
				newest = toRUB.Date.Time
			}
			if c := (candidate{fromRUB.Rate.Div(toRUB.Rate), newest}); accept(c) {
				return c.rate, c.newest, nil
			}
		}
	}

	if fallback != nil {
		return fallback.rate, fallback.newest, nil
	}
	return decimal.Zero, time.Time{}, ErrRateUnavailable
}

func normalizeCcy(c string) string {
	switch c {
	case "USDT", "BUSD", "USDC":
		return "USD"
	default:
		return c
	}
}
