package instrument_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/instrument"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
	"github.com/kkulebaev/omnifolio/api/internal/testutil"
)

// setupDB creates a fresh database with all migrations applied.
func setupDB(t *testing.T) (*pgxpool.Pool, *storage.Queries) {
	t.Helper()
	pool := testutil.NewRawPool(t)
	if err := storage.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool, storage.New(pool)
}

// mustUser inserts a minimal user row and returns its ID.
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

// mustAccount inserts a manual account for userID and returns its ID.
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

// priceRow returns the price row for instrumentID, or nil if absent.
func priceRow(t *testing.T, ctx context.Context, q *storage.Queries, id uuid.UUID) *storage.Price {
	t.Helper()
	p, err := q.GetPrice(ctx, id)
	if err != nil {
		return nil
	}
	return &p
}

// TestCreatePersonal_OK verifies the happy path: user_id is set correctly and
// a price row is created with a recent fetched_at.
func TestCreatePersonal_OK(t *testing.T) {
	ctx := context.Background()
	_, q := setupDB(t)
	svc := instrument.NewService(q)
	userID := mustUser(t, ctx, q)

	in, err := svc.CreatePersonal(ctx, userID, instrument.CreateInput{
		Ticker:     "APARTMENT1",
		AssetClass: instrument.AssetClassRealEstate,
		Currency:   "RUB",
		Name:       "My Apartment",
	}, decimal.NewFromInt(5_000_000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if in.UserID == nil || *in.UserID != userID {
		t.Errorf("user_id = %v, want %v", in.UserID, userID)
	}

	p := priceRow(t, ctx, q, in.ID)
	if p == nil {
		t.Fatal("price row not found after CreatePersonal")
	}
	age := time.Since(p.FetchedAt.Time)
	if age > 5*time.Second {
		t.Errorf("price.fetched_at too old: %v", age)
	}
}

// TestCreatePersonal_InvalidAssetClass verifies that attempting to create a
// personal instrument with an exchange-only asset class returns ErrInvalidAssetClass
// and leaves the database unchanged.
func TestCreatePersonal_InvalidAssetClass(t *testing.T) {
	ctx := context.Background()
	_, q := setupDB(t)
	svc := instrument.NewService(q)
	userID := mustUser(t, ctx, q)

	_, err := svc.CreatePersonal(ctx, userID, instrument.CreateInput{
		Ticker:     "AAPL",
		AssetClass: instrument.AssetClassUSStock,
		Currency:   "USD",
		Name:       "Apple Inc",
	}, decimal.NewFromInt(100))

	if !errors.Is(err, instrument.ErrInvalidAssetClass) {
		t.Fatalf("got %v, want ErrInvalidAssetClass", err)
	}
	list, err := svc.List(ctx, userID, instrument.ListInput{Scope: instrument.ScopeMine, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 0 {
		t.Errorf("DB not clean: %d rows", list.Total)
	}
}

// TestCreatePersonal_ConflictPerUser verifies that:
//   - same (ticker, class) for the same user on a second call → ErrAlreadyExists
//   - same (ticker, class) for a different user → succeeds (per-user uniqueness)
func TestCreatePersonal_ConflictPerUser(t *testing.T) {
	ctx := context.Background()
	_, q := setupDB(t)
	svc := instrument.NewService(q)
	userA := mustUser(t, ctx, q)
	userB := mustUser(t, ctx, q)

	in := instrument.CreateInput{
		Ticker:     "DACHA",
		AssetClass: instrument.AssetClassRealEstate,
		Currency:   "RUB",
		Name:       "Dacha",
	}
	price := decimal.NewFromInt(1_000_000)

	if _, err := svc.CreatePersonal(ctx, userA, in, price); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := svc.CreatePersonal(ctx, userA, in, price); !errors.Is(err, instrument.ErrAlreadyExists) {
		t.Fatalf("second create for same user: got %v, want ErrAlreadyExists", err)
	}
	if _, err := svc.CreatePersonal(ctx, userB, in, price); err != nil {
		t.Fatalf("create for different user: %v", err)
	}
}

// TestSearch_OtherUserPersonalHidden verifies that a personal instrument owned
// by userA is not visible in userB's search results.
func TestSearch_OtherUserPersonalHidden(t *testing.T) {
	ctx := context.Background()
	_, q := setupDB(t)
	svc := instrument.NewService(q)
	userA := mustUser(t, ctx, q)
	userB := mustUser(t, ctx, q)

	if _, err := svc.CreatePersonal(ctx, userA, instrument.CreateInput{
		Ticker:     "APARTMENT",
		AssetClass: instrument.AssetClassRealEstate,
		Currency:   "RUB",
		Name:       "UserA Apartment",
	}, decimal.NewFromInt(1)); err != nil {
		t.Fatal(err)
	}

	results, err := svc.Search(ctx, userB, "APARTMENT")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("userB search returned %d results, want 0", len(results))
	}
}

// TestGet_OtherUserPersonal_404 verifies that accessing another user's personal
// instrument returns ErrNotFound (not 403, per §3.9 of design.md).
func TestGet_OtherUserPersonal_404(t *testing.T) {
	ctx := context.Background()
	_, q := setupDB(t)
	svc := instrument.NewService(q)
	userA := mustUser(t, ctx, q)
	userB := mustUser(t, ctx, q)

	instr, err := svc.CreatePersonal(ctx, userA, instrument.CreateInput{
		Ticker:     "FLAT",
		AssetClass: instrument.AssetClassRealEstate,
		Currency:   "RUB",
		Name:       "Flat",
	}, decimal.NewFromInt(1))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Get(ctx, userB, instr.ID); !errors.Is(err, instrument.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

// TestSetPrice_GlobalInstrument_404 verifies that SetPrice on a global
// instrument returns ErrNotFound and leaves the price unchanged.
func TestSetPrice_GlobalInstrument_404(t *testing.T) {
	ctx := context.Background()
	_, q := setupDB(t)
	svc := instrument.NewService(q)
	userA := mustUser(t, ctx, q)

	global, err := svc.CreateOrGet(ctx, instrument.CreateInput{
		Ticker:     "SBER",
		AssetClass: instrument.AssetClassRUStock,
		Currency:   "RUB",
		Name:       "Sberbank",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.UpsertGlobalPrice(ctx, storage.UpsertGlobalPriceParams{
		ID:    global.ID,
		Price: decimal.NewFromInt(300),
	}); err != nil {
		t.Fatal(err)
	}
	p0 := priceRow(t, ctx, q, global.ID)

	if err := svc.SetPrice(ctx, userA, global.ID, decimal.NewFromInt(999)); !errors.Is(err, instrument.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}

	p1 := priceRow(t, ctx, q, global.ID)
	if p0 == nil || p1 == nil {
		t.Fatal("missing price row")
	}
	if !p1.Price.Equal(p0.Price) {
		t.Errorf("price changed: %v → %v", p0.Price, p1.Price)
	}
}

// TestSetPrice_OtherUserPersonal_NoSideEffect verifies the SQL-level ownership
// predicate in UpsertPersonalPrice: userB cannot modify userA's price, and the
// price remains unchanged.
func TestSetPrice_OtherUserPersonal_NoSideEffect(t *testing.T) {
	ctx := context.Background()
	_, q := setupDB(t)
	svc := instrument.NewService(q)
	userA := mustUser(t, ctx, q)
	userB := mustUser(t, ctx, q)

	instr, err := svc.CreatePersonal(ctx, userA, instrument.CreateInput{
		Ticker:     "GARAGE",
		AssetClass: instrument.AssetClassVehicle,
		Currency:   "RUB",
		Name:       "Garage",
	}, decimal.NewFromInt(500_000))
	if err != nil {
		t.Fatal(err)
	}
	p0 := priceRow(t, ctx, q, instr.ID)

	if err := svc.SetPrice(ctx, userB, instr.ID, decimal.NewFromInt(999_999)); !errors.Is(err, instrument.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}

	p1 := priceRow(t, ctx, q, instr.ID)
	if p0 == nil || p1 == nil {
		t.Fatal("missing price row")
	}
	if !p1.Price.Equal(p0.Price) {
		t.Errorf("price changed by other user: %v → %v", p0.Price, p1.Price)
	}
}

// TestDelete_HasPositions_409 verifies that deleting an instrument that still
// has a position FK-reference returns ErrHasPositions.
func TestDelete_HasPositions_409(t *testing.T) {
	ctx := context.Background()
	_, q := setupDB(t)
	svc := instrument.NewService(q)
	userA := mustUser(t, ctx, q)
	accountID := mustAccount(t, ctx, q, userA)

	instr, err := svc.CreatePersonal(ctx, userA, instrument.CreateInput{
		Ticker:     "CAR",
		AssetClass: instrument.AssetClassVehicle,
		Currency:   "RUB",
		Name:       "My Car",
	}, decimal.NewFromInt(1_000_000))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreatePosition(ctx, storage.CreatePositionParams{
		AccountID:    accountID,
		InstrumentID: instr.ID,
		Quantity:     decimal.NewFromInt(1),
	}); err != nil {
		t.Fatalf("create position: %v", err)
	}

	if err := svc.DeletePersonal(ctx, userA, instr.ID); !errors.Is(err, instrument.ErrHasPositions) {
		t.Errorf("got %v, want ErrHasPositions", err)
	}
}

// TestDelete_OK verifies that deleting a personal instrument with no positions
// succeeds and the instrument no longer appears in List.
func TestDelete_OK(t *testing.T) {
	ctx := context.Background()
	_, q := setupDB(t)
	svc := instrument.NewService(q)
	userA := mustUser(t, ctx, q)

	instr, err := svc.CreatePersonal(ctx, userA, instrument.CreateInput{
		Ticker:     "BICYCLE",
		AssetClass: instrument.AssetClassVehicle,
		Currency:   "RUB",
		Name:       "My Bicycle",
	}, decimal.NewFromInt(50_000))
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.DeletePersonal(ctx, userA, instr.ID); err != nil {
		t.Errorf("delete: %v", err)
	}

	list, err := svc.List(ctx, userA, instrument.ListInput{Scope: instrument.ScopeMine, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 0 {
		t.Errorf("List after delete: %d items, want 0", list.Total)
	}
}

// TestAdminUpsertPrice_RejectsPersonal verifies that UpsertGlobalPrice (admin /
// cron path) returns rows_affected == 0 for a personal instrument and does not
// modify its price.
func TestAdminUpsertPrice_RejectsPersonal(t *testing.T) {
	ctx := context.Background()
	_, q := setupDB(t)
	svc := instrument.NewService(q)
	userA := mustUser(t, ctx, q)

	instr, err := svc.CreatePersonal(ctx, userA, instrument.CreateInput{
		Ticker:     "VILLA",
		AssetClass: instrument.AssetClassRealEstate,
		Currency:   "EUR",
		Name:       "Villa",
	}, decimal.NewFromInt(200_000))
	if err != nil {
		t.Fatal(err)
	}
	p0 := priceRow(t, ctx, q, instr.ID)

	rows, err := q.UpsertGlobalPrice(ctx, storage.UpsertGlobalPriceParams{
		ID:    instr.ID,
		Price: decimal.NewFromInt(999_999),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("UpsertGlobalPrice rows_affected = %d, want 0", rows)
	}

	p1 := priceRow(t, ctx, q, instr.ID)
	if p0 == nil || p1 == nil {
		t.Fatal("missing price row")
	}
	if !p1.Price.Equal(p0.Price) {
		t.Errorf("price changed by admin upsert: %v → %v", p0.Price, p1.Price)
	}
}

// TestCreateOrGet_RaceRecovery_GlobalOnly verifies that CreateOrGet creates a
// proper global (user_id IS NULL) row even when a personal instrument with the
// same ticker but a different asset_class already exists, demonstrating that
// the partial unique indexes are correctly scoped per ownership.
func TestCreateOrGet_RaceRecovery_GlobalOnly(t *testing.T) {
	ctx := context.Background()
	_, q := setupDB(t)
	svc := instrument.NewService(q)
	userX := mustUser(t, ctx, q)

	personal, err := svc.CreatePersonal(ctx, userX, instrument.CreateInput{
		Ticker:     "AAPL",
		AssetClass: instrument.AssetClassRealEstate,
		Currency:   "RUB",
		Name:       "Apple-named house",
	}, decimal.NewFromInt(1))
	if err != nil {
		t.Fatal(err)
	}

	global, err := svc.CreateOrGet(ctx, instrument.CreateInput{
		Ticker:     "AAPL",
		AssetClass: instrument.AssetClassUSStock,
		Currency:   "USD",
		Name:       "Apple Inc",
	})
	if err != nil {
		t.Fatalf("CreateOrGet: %v", err)
	}
	if global.ID == personal.ID {
		t.Error("CreateOrGet returned the personal row; expected a new global row")
	}
	if global.UserID != nil {
		t.Errorf("global.UserID = %v, want nil", global.UserID)
	}
	if global.AssetClass != instrument.AssetClassUSStock {
		t.Errorf("global.AssetClass = %q, want us_stock", global.AssetClass)
	}
}
