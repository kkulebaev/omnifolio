package fx_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/fx"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
	"github.com/kkulebaev/omnifolio/api/internal/testutil"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func setupFX(t *testing.T) *fx.Service {
	t.Helper()
	ctx := context.Background()
	pool := testutil.NewRawPool(t)
	if err := storage.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	q := storage.New(pool)
	rates := []struct {
		date, from, to string
		rate           float64
	}{
		{"2025-01-10", "USD", "RUB", 100},
		{"2025-01-20", "USD", "RUB", 110},
		{"2025-01-12", "EUR", "RUB", 120},
		{"2025-01-02", "RUB", "EUR", 0.01},
	}
	for _, r := range rates {
		if err := q.UpsertFxRate(ctx, storage.UpsertFxRateParams{
			Date:    pgtype.Date{Time: day(r.date), Valid: true},
			FromCcy: r.from,
			ToCcy:   r.to,
			Rate:    decimal.NewFromFloat(r.rate),
		}); err != nil {
			t.Fatalf("upsert fx: %v", err)
		}
	}
	return fx.NewService(q, slog.Default())
}

func TestFX_GetRateAt(t *testing.T) {
	svc := setupFX(t)
	ctx := context.Background()

	tests := []struct {
		name       string
		from, to   string
		at         string
		wantRate   string
		wantApprox bool
	}{
		{"exact date", "USD", "RUB", "2025-01-20", "110", false},
		{"latest earlier rate", "USD", "RUB", "2025-01-15", "100", false},
		{"after all rates", "USD", "RUB", "2025-03-01", "110", false},
		{"only later rate is approximated", "USD", "RUB", "2025-01-05", "100", true},
		{"stablecoin parity", "USDT", "RUB", "2025-01-15", "100", false},
		{"inverse", "RUB", "USD", "2025-01-15", "0.01", false},
		{"triangulated via RUB", "EUR", "USD", "2025-01-15", "1.2", false},
		{"triangulated with one leg later", "EUR", "USD", "2025-01-11", "1.2", true},
		{"same currency", "RUB", "RUB", "2000-01-01", "1", false},
		{"direct exact", "EUR", "RUB", "2025-01-15", "120", false},
		// Direct EUR→RUB exists only from 01-12; the inverse on 01-02 is exact.
		{"exact inverse beats later direct", "EUR", "RUB", "2025-01-11", "100", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rate, approx, err := svc.GetRateAt(ctx, tc.from, tc.to, day(tc.at))
			if err != nil {
				t.Fatalf("GetRateAt: %v", err)
			}
			if !rate.Equal(decimal.RequireFromString(tc.wantRate)) {
				t.Errorf("rate = %s, want %s", rate, tc.wantRate)
			}
			if approx != tc.wantApprox {
				t.Errorf("approximated = %v, want %v", approx, tc.wantApprox)
			}
		})
	}
}

func TestFX_GetRateAtUnavailable(t *testing.T) {
	svc := setupFX(t)
	_, _, err := svc.GetRateAt(context.Background(), "CNY", "RUB", day("2025-01-15"))
	if !errors.Is(err, fx.ErrRateUnavailable) {
		t.Fatalf("err = %v, want ErrRateUnavailable", err)
	}
}

func TestFX_GetRateUsesLatest(t *testing.T) {
	svc := setupFX(t)
	rate, err := svc.GetRate(context.Background(), "USD", "RUB")
	if err != nil {
		t.Fatal(err)
	}
	if !rate.Equal(decimal.NewFromInt(110)) {
		t.Fatalf("rate = %s, want 110", rate)
	}
}
