package returns_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/deposits"
	"github.com/kkulebaev/omnifolio/api/internal/fx"
	"github.com/kkulebaev/omnifolio/api/internal/portfolio"
	"github.com/kkulebaev/omnifolio/api/internal/returns"
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

type fixture struct {
	q        *storage.Queries
	deposits *deposits.Service
	returns  *returns.Service
	user     uuid.UUID
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	pool := testutil.NewRawPool(t)
	if err := storage.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	q := storage.New(pool)
	fxSvc := fx.NewService(q, slog.Default())
	user := uuid.Must(uuid.NewV7())
	if _, err := q.CreateUser(ctx, storage.CreateUserParams{
		ID: user, Email: user.String() + "@test.local", PasswordHash: "x", DisplayCurrency: "RUB",
	}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertFxRate(ctx, storage.UpsertFxRateParams{
		Date: pgtype.Date{Time: day("2025-01-10"), Valid: true}, FromCcy: "USD", ToCcy: "RUB", Rate: decimal.NewFromInt(100),
	}); err != nil {
		t.Fatal(err)
	}
	return fixture{
		q:        q,
		deposits: deposits.NewService(pool),
		returns:  returns.NewService(q, portfolio.NewService(q, fxSvc), fxSvc, slog.Default()),
		user:     user,
	}
}

func (f fixture) flow(t *testing.T, date, kind, ccy, amount string) {
	t.Helper()
	if _, err := f.deposits.Create(context.Background(), f.user, deposits.Input{
		Date: day(date), Kind: kind, Currency: ccy, Amount: decimal.RequireFromString(amount),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReturns_ComputeTotals(t *testing.T) {
	f := setup(t)
	// USD flow predates every stored rate → converted at the later 100 and flagged.
	f.flow(t, "2025-01-05", deposits.KindDeposit, "USD", "100")
	f.flow(t, "2025-02-01", deposits.KindDeposit, "RUB", "5000")
	f.flow(t, "2025-03-01", deposits.KindWithdrawal, "RUB", "1000.50")

	r, err := f.returns.Compute(context.Background(), f.user, "RUB")
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, got decimal.Decimal, want string) {
		t.Helper()
		if !got.Equal(decimal.RequireFromString(want)) {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}
	check("TotalDeposited", r.TotalDeposited, "15000")
	check("TotalWithdrawn", r.TotalWithdrawn, "1000.50")
	check("NetInvested", r.NetInvested, "13999.50")
	check("CurrentValue", r.CurrentValue, "0")
	check("Profit", r.Profit, "-13999.50")
	if r.ProfitPct == nil || !r.ProfitPct.Equal(decimal.NewFromInt(-1)) {
		t.Errorf("ProfitPct = %v, want -1", r.ProfitPct)
	}
	if !r.FXApproximated {
		t.Error("FXApproximated = false, want true")
	}
	if r.FirstFlowDate == nil || !r.FirstFlowDate.Equal(day("2025-01-05")) {
		t.Errorf("FirstFlowDate = %v, want 2025-01-05", r.FirstFlowDate)
	}
	if r.TWR != nil || r.TWRReason == nil || *r.TWRReason != returns.ReasonNoSnapshots {
		t.Errorf("TWR = %v (reason %v), want nil with no_snapshots", r.TWR, r.TWRReason)
	}
}

func TestReturns_FutureFlowFoldedIntoToday(t *testing.T) {
	f := setup(t)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	f.flow(t, today.AddDate(0, 0, 1).Format("2006-01-02"), deposits.KindDeposit, "RUB", "100")

	r, err := f.returns.Compute(context.Background(), f.user, "RUB")
	if err != nil {
		t.Fatal(err)
	}
	if r.FirstFlowDate == nil || !r.FirstFlowDate.Equal(today) {
		t.Errorf("FirstFlowDate = %v, want %v", r.FirstFlowDate, today)
	}
	if !r.NetInvested.Equal(decimal.NewFromInt(100)) {
		t.Errorf("NetInvested = %s, want 100", r.NetInvested)
	}
}

func TestReturns_RateUnavailable(t *testing.T) {
	f := setup(t)
	f.flow(t, "2025-02-01", deposits.KindDeposit, "CNY", "100")

	_, err := f.returns.Compute(context.Background(), f.user, "RUB")
	var rateErr *returns.RateUnavailableError
	if !errors.As(err, &rateErr) || rateErr.From != "CNY" || rateErr.To != "RUB" {
		t.Fatalf("err = %v, want RateUnavailableError CNY→RUB", err)
	}
}

func TestReturns_TWRFromSnapshots(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	d0 := today.AddDate(0, 0, -20)
	d1 := today.AddDate(0, 0, -10)

	// Snapshots stored in RUB display but recomputed from native by_currency:
	// d0 = 100 USD × 100 = 10000 RUB; d1 = 100 USD + 1000 RUB = 11000 RUB.
	snap := func(d time.Time, byCcy map[string]string) {
		raw, _ := json.Marshal(byCcy)
		if err := f.q.UpsertPortfolioSnapshot(ctx, storage.UpsertPortfolioSnapshotParams{
			UserID: f.user, SnapshotDate: pgtype.Date{Time: d, Valid: true}, DisplayCurrency: "EUR",
			GrandTotal: decimal.Zero, ByAssetClass: []byte("{}"), ByCurrency: raw, ByAccount: []byte("{}"),
		}); err != nil {
			t.Fatal(err)
		}
	}
	snap(d0, map[string]string{"USD": "100"})
	snap(d1, map[string]string{"USD": "100", "RUB": "1000"})
	// 1000 RUB deposited between the snapshots: r1 = (11000 − 1000) / 10000 − 1 = 0.
	f.flow(t, d0.AddDate(0, 0, 5).Format("2006-01-02"), deposits.KindDeposit, "RUB", "1000")

	r, err := f.returns.Compute(ctx, f.user, "RUB")
	if err != nil {
		t.Fatal(err)
	}
	// The live point is the empty portfolio (0): V − F ≤ 0, so that period is
	// skipped rather than flipping the product; only r1 = 0 remains.
	if r.TWR == nil || !r.TWR.IsZero() {
		t.Fatalf("TWR = %v (reason %v), want 0", r.TWR, r.TWRReason)
	}
	if r.TWRFrom == nil || !r.TWRFrom.Equal(d0) {
		t.Errorf("TWRFrom = %v, want %v", r.TWRFrom, d0)
	}
	if r.TWRAnnualized != nil {
		t.Errorf("TWRAnnualized = %v, want nil for a 20-day period", r.TWRAnnualized)
	}
	if r.XIRR != nil || r.XIRRReason == nil || *r.XIRRReason != returns.ReasonInsufficientHistory {
		t.Errorf("XIRR = %v (reason %v), want nil with insufficient_history", r.XIRR, r.XIRRReason)
	}
}

func TestReturns_NetInvestedSeries(t *testing.T) {
	f := setup(t)
	f.flow(t, "2025-01-15", deposits.KindDeposit, "USD", "10")
	f.flow(t, "2025-02-01", deposits.KindDeposit, "RUB", "500")
	f.flow(t, "2025-03-01", deposits.KindWithdrawal, "RUB", "200")
	f.flow(t, "2025-03-02", deposits.KindDeposit, "CNY", "999")
	// Dated tomorrow (validation allows +1 day): folded into today, and so not
	// yet part of today's snapshot.
	tomorrow := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
	f.flow(t, tomorrow.Format("2006-01-02"), deposits.KindDeposit, "RUB", "7")

	got, partial, err := f.returns.NetInvestedSeries(context.Background(), f.user, []returns.SeriesPoint{
		{Date: day("2025-01-01"), Currency: "RUB"},
		{Date: day("2025-02-01"), Currency: "RUB"},
		{Date: day("2025-03-10"), Currency: "RUB"},
		{Date: day("2025-03-10"), Currency: "USD"},
		{Date: tomorrow.AddDate(0, 0, -1), Currency: "RUB"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !partial {
		t.Error("partial = false, want true: the CNY flow has no rate")
	}
	// CNY has no rate at all and is left out of the chart series.
	want := []string{"0", "1000", "1300", "13", "1300"}
	for i, w := range want {
		if !got[i].Equal(decimal.RequireFromString(w)) {
			t.Errorf("point %d = %s, want %s", i, got[i], w)
		}
	}
}
