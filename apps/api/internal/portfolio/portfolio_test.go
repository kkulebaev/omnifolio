package portfolio_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/fx"
	"github.com/kkulebaev/omnifolio/api/internal/instrument"
	"github.com/kkulebaev/omnifolio/api/internal/portfolio"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
	"github.com/kkulebaev/omnifolio/api/internal/testutil"
)

// setupPortfolio creates a fresh migrated database and returns the pool,
// sqlc queries, instrument service, and portfolio service.
func setupPortfolio(t *testing.T) (*pgxpool.Pool, *storage.Queries, *instrument.Service, *portfolio.Service) {
	t.Helper()
	pool := testutil.NewRawPool(t)
	if err := storage.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	q := storage.New(pool)
	fxSvc := fx.NewService(q, slog.Default())
	portSvc := portfolio.NewService(q, fxSvc)
	instrSvc := instrument.NewService(q)
	return pool, q, instrSvc, portSvc
}

// mustUser inserts a user row and returns its ID.
func mustUser(t *testing.T, ctx context.Context, q *storage.Queries) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	_, err := q.CreateUser(ctx, storage.CreateUserParams{
		ID:              id,
		Email:           id.String() + "@test.local",
		PasswordHash:    "x",
		DisplayCurrency: "RUB",
	})
	if err != nil {
		t.Fatalf("mustUser: %v", err)
	}
	return id
}

// mustAccount inserts a manual account and returns its ID.
func mustAccount(t *testing.T, ctx context.Context, q *storage.Queries, userID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	_, err := q.CreateAccount(ctx, storage.CreateAccountParams{
		ID:         id,
		UserID:     userID,
		SourceType: "manual",
		Name:       "Test Account",
	})
	if err != nil {
		t.Fatalf("mustAccount: %v", err)
	}
	return id
}

// TestCompute_RealEstate_NeverStale verifies that a real_estate position with
// an intentionally old fetched_at (30 days) is never marked as stale.
func TestCompute_RealEstate_NeverStale(t *testing.T) {
	ctx := context.Background()
	pool, q, instrSvc, portSvc := setupPortfolio(t)
	userID := mustUser(t, ctx, q)
	accountID := mustAccount(t, ctx, q, userID)

	instr, err := instrSvc.CreatePersonal(ctx, userID, instrument.CreateInput{
		Ticker:     "REALESTATE1",
		AssetClass: instrument.AssetClassRealEstate,
		Currency:   "RUB",
		Name:       "My Flat",
	}, decimal.NewFromInt(8_000_000))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreatePosition(ctx, storage.CreatePositionParams{
		AccountID:    accountID,
		InstrumentID: instr.ID,
		Quantity:     decimal.NewFromInt(1),
	}); err != nil {
		t.Fatal(err)
	}

	// Make the price look very stale.
	if _, err := pool.Exec(ctx,
		`UPDATE prices SET fetched_at = now() - interval '30 days' WHERE instrument_id = $1`,
		instr.ID,
	); err != nil {
		t.Fatalf("update fetched_at: %v", err)
	}

	p, err := portSvc.Compute(ctx, userID, "RUB")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(p.Positions))
	}
	pos := p.Positions[0]
	if pos.PriceStale {
		t.Error("real_estate position should never be stale")
	}
	if pos.ValueNative == nil {
		t.Error("ValueNative should not be nil for real_estate with price")
	}
}

// TestCompute_RealEstate_NoPriceRow_DegradesGracefully verifies that deleting
// the price row for a real_estate position results in priceStale=true and nil
// value fields without a panic.
func TestCompute_RealEstate_NoPriceRow_DegradesGracefully(t *testing.T) {
	ctx := context.Background()
	pool, q, instrSvc, portSvc := setupPortfolio(t)
	userID := mustUser(t, ctx, q)
	accountID := mustAccount(t, ctx, q, userID)

	instr, err := instrSvc.CreatePersonal(ctx, userID, instrument.CreateInput{
		Ticker:     "REALESTATE2",
		AssetClass: instrument.AssetClassRealEstate,
		Currency:   "RUB",
		Name:       "Country House",
	}, decimal.NewFromInt(3_000_000))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreatePosition(ctx, storage.CreatePositionParams{
		AccountID:    accountID,
		InstrumentID: instr.ID,
		Quantity:     decimal.NewFromInt(1),
	}); err != nil {
		t.Fatal(err)
	}

	// Remove the price row entirely.
	if _, err := pool.Exec(ctx,
		`DELETE FROM prices WHERE instrument_id = $1`, instr.ID,
	); err != nil {
		t.Fatalf("delete price: %v", err)
	}

	p, err := portSvc.Compute(ctx, userID, "RUB")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(p.Positions))
	}
	pos := p.Positions[0]
	if !pos.PriceStale {
		t.Error("priceStale should be true when price row is absent")
	}
	if pos.ValueNative != nil {
		t.Errorf("ValueNative should be nil, got %v", pos.ValueNative)
	}
	if pos.ValueDisplay != nil {
		t.Errorf("ValueDisplay should be nil, got %v", pos.ValueDisplay)
	}
}

// TestCompute_RealEstate_FXConvertsToDisplayCcy verifies that a EUR-denominated
// real_estate position is correctly converted to USD using an inserted FX rate.
func TestCompute_RealEstate_FXConvertsToDisplayCcy(t *testing.T) {
	ctx := context.Background()
	_, q, instrSvc, portSvc := setupPortfolio(t)
	userID := mustUser(t, ctx, q)
	accountID := mustAccount(t, ctx, q, userID)

	instr, err := instrSvc.CreatePersonal(ctx, userID, instrument.CreateInput{
		Ticker:     "EURVILLA",
		AssetClass: instrument.AssetClassRealEstate,
		Currency:   "EUR",
		Name:       "European Villa",
	}, decimal.NewFromInt(100))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreatePosition(ctx, storage.CreatePositionParams{
		AccountID:    accountID,
		InstrumentID: instr.ID,
		Quantity:     decimal.NewFromInt(1),
	}); err != nil {
		t.Fatal(err)
	}

	// Insert EUR→USD FX rate.
	if err := q.UpsertFxRate(ctx, storage.UpsertFxRateParams{
		Date:    pgtype.Date{Time: time.Now(), Valid: true},
		FromCcy: "EUR",
		ToCcy:   "USD",
		Rate:    decimal.NewFromFloat(1.10),
	}); err != nil {
		t.Fatalf("upsert fx rate: %v", err)
	}

	p, err := portSvc.Compute(ctx, userID, "USD")
	if err != nil {
		t.Fatal(err)
	}
	// 100 EUR × 1.10 = 110 USD
	minExpected := decimal.NewFromInt(110)
	if p.Summary.GrandTotal.LessThan(minExpected) {
		t.Errorf("grand_total = %v, want >= %v", p.Summary.GrandTotal, minExpected)
	}
	if assetTotal, ok := p.Summary.ByAssetClass["real_estate"]; !ok || assetTotal.LessThan(minExpected) {
		t.Errorf("ByAssetClass[real_estate] = %v, want >= %v", assetTotal, minExpected)
	}
}

// TestCompute_OtherUserPersonalNotInPortfolio verifies that a personal
// instrument owned by userA does not appear in userB's portfolio.
func TestCompute_OtherUserPersonalNotInPortfolio(t *testing.T) {
	ctx := context.Background()
	_, q, instrSvc, portSvc := setupPortfolio(t)
	userA := mustUser(t, ctx, q)
	userB := mustUser(t, ctx, q)
	accountA := mustAccount(t, ctx, q, userA)

	instr, err := instrSvc.CreatePersonal(ctx, userA, instrument.CreateInput{
		Ticker:     "USERAPT",
		AssetClass: instrument.AssetClassRealEstate,
		Currency:   "RUB",
		Name:       "UserA Apartment",
	}, decimal.NewFromInt(1_000_000))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreatePosition(ctx, storage.CreatePositionParams{
		AccountID:    accountA,
		InstrumentID: instr.ID,
		Quantity:     decimal.NewFromInt(1),
	}); err != nil {
		t.Fatal(err)
	}

	pB, err := portSvc.Compute(ctx, userB, "RUB")
	if err != nil {
		t.Fatal(err)
	}
	if len(pB.Positions) != 0 {
		t.Errorf("userB portfolio has %d positions, want 0", len(pB.Positions))
	}
}
