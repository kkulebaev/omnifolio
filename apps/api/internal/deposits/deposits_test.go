package deposits_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/deposits"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
	"github.com/kkulebaev/omnifolio/api/internal/testutil"
)

func setup(t *testing.T) (*storage.Queries, *deposits.Service) {
	t.Helper()
	pool := testutil.NewRawPool(t)
	if err := storage.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return storage.New(pool), deposits.NewService(pool)
}

func mustUser(t *testing.T, q *storage.Queries) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := q.CreateUser(context.Background(), storage.CreateUserParams{
		ID:              id,
		Email:           id.String() + "@test.local",
		PasswordHash:    "x",
		DisplayCurrency: "RUB",
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return id
}

func TestDeposits_Update(t *testing.T) {
	ctx := context.Background()
	q, svc := setup(t)
	owner := mustUser(t, q)
	stranger := mustUser(t, q)

	d, err := svc.Create(ctx, owner, deposits.Input{
		Date:     time.Date(2025, 3, 14, 0, 0, 0, 0, time.UTC),
		Kind:     deposits.KindDeposit,
		Currency: "RUB",
		Amount:   decimal.NewFromInt(50000),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	in := deposits.Input{
		Date:     time.Date(2025, 3, 20, 0, 0, 0, 0, time.UTC),
		Kind:     deposits.KindWithdrawal,
		Currency: "USD",
		Amount:   decimal.RequireFromString("123.45"),
	}

	if _, err := svc.Update(ctx, stranger, d.ID, in); !errors.Is(err, deposits.ErrNotFound) {
		t.Fatalf("foreign update: err = %v, want ErrNotFound", err)
	}
	if _, err := svc.Update(ctx, owner, uuid.Must(uuid.NewV7()), in); !errors.Is(err, deposits.ErrNotFound) {
		t.Fatalf("missing update: err = %v, want ErrNotFound", err)
	}

	got, err := svc.Update(ctx, owner, d.ID, in)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !got.Date.Equal(in.Date) || got.Kind != in.Kind || got.Currency != in.Currency || !got.Amount.Equal(in.Amount) {
		t.Fatalf("update result = %+v, want %+v", got, in)
	}

	list, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !list[0].Amount.Equal(in.Amount) || list[0].Kind != deposits.KindWithdrawal {
		t.Fatalf("list after update = %+v", list)
	}
	if strangerList, _ := svc.List(ctx, stranger); len(strangerList) != 0 {
		t.Fatalf("stranger sees %d deposits", len(strangerList))
	}
}

func TestDeposits_ListOrderedByDateDesc(t *testing.T) {
	ctx := context.Background()
	q, svc := setup(t)
	owner := mustUser(t, q)
	for _, d := range []string{"2025-01-10", "2025-03-05", "2025-02-20"} {
		dt, _ := time.Parse("2006-01-02", d)
		if _, err := svc.Create(ctx, owner, deposits.Input{
			Date: dt, Kind: deposits.KindDeposit, Currency: "RUB", Amount: decimal.NewFromInt(1),
		}); err != nil {
			t.Fatal(err)
		}
	}
	list, err := svc.List(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range list {
		got = append(got, d.Date.Format("2006-01-02"))
	}
	want := []string{"2025-03-05", "2025-02-20", "2025-01-10"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}
