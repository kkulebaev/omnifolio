// Package snapshot writes daily snapshots of the aggregated portfolio
// into portfolio_snapshots. See docs/design.md §3.7 and §4.4.
package snapshot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/portfolio"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
)

type Service struct {
	q         *storage.Queries
	portfolio *portfolio.Service
	log       *slog.Logger
}

func NewService(q *storage.Queries, p *portfolio.Service, log *slog.Logger) *Service {
	return &Service{q: q, portfolio: p, log: log}
}

// RunDaily computes and stores a snapshot for every user. One user's failure
// does not abort the others; per-user errors are logged and joined into the
// returned error so the scheduler can report a non-zero result.
func (s *Service) RunDaily(ctx context.Context) error {
	ids, err := s.q.ListUserIDs(ctx)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}

	var errs []error
	for _, uid := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.RunForUser(ctx, uid); err != nil {
			s.log.Error("snapshot: user failed", "user_id", uid, "err", err)
			errs = append(errs, fmt.Errorf("user %s: %w", uid, err))
		}
	}
	return errors.Join(errs...)
}

// RunForUser computes the user's portfolio in their current display_currency
// and UPSERT-s today's snapshot. Skips users with no positions or fully stale
// portfolios (grand_total == 0).
func (s *Service) RunForUser(ctx context.Context, userID uuid.UUID) error {
	user, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("load user: %w", err)
	}

	pf, err := s.portfolio.Compute(ctx, userID, user.DisplayCurrency)
	if err != nil {
		return fmt.Errorf("compute: %w", err)
	}

	if len(pf.Positions) == 0 {
		s.log.Debug("snapshot: skip empty portfolio", "user_id", userID)
		return nil
	}
	if pf.Summary.GrandTotal.IsZero() {
		s.log.Warn("snapshot: skip fully stale portfolio", "user_id", userID)
		return nil
	}

	byAssetClass, err := encodeAmounts(pf.Summary.ByAssetClass)
	if err != nil {
		return fmt.Errorf("encode by_asset_class: %w", err)
	}
	byCurrency, err := encodeAmounts(pf.Summary.ByCurrency)
	if err != nil {
		return fmt.Errorf("encode by_currency: %w", err)
	}
	byAccount, err := encodeAmounts(pf.Summary.ByAccount)
	if err != nil {
		return fmt.Errorf("encode by_account: %w", err)
	}

	return s.q.UpsertPortfolioSnapshot(ctx, storage.UpsertPortfolioSnapshotParams{
		UserID:          userID,
		SnapshotDate:    todayUTC(),
		DisplayCurrency: pf.Summary.DisplayCurrency,
		GrandTotal:      pf.Summary.GrandTotal,
		ByAssetClass:    byAssetClass,
		ByCurrency:      byCurrency,
		ByAccount:       byAccount,
	})
}

func encodeAmounts(m map[string]decimal.Decimal) ([]byte, error) {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v.String()
	}
	return json.Marshal(out)
}

func todayUTC() pgtype.Date {
	now := time.Now().UTC()
	return pgtype.Date{
		Time:  time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC),
		Valid: true,
	}
}
