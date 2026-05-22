package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/kkulebaev/omnifolio/api/internal/testutil"
)

// openMigratedDB creates a fresh database, applies all migrations and returns a
// *sql.DB suitable for goose down/up operations plus the pool cleanup registered
// by testutil.
func openMigratedDB(t *testing.T) *sql.DB {
	t.Helper()
	pool := testutil.NewRawPool(t)

	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("goose set dialect: %v", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { db.Close() })

	if err := goose.UpContext(context.Background(), db, "migrations"); err != nil {
		t.Fatalf("goose up: %v", err)
	}
	return db
}

// TestMigration_RoundTrip_0008 rolls migration 0008 down to version 7 and back
// up again, then asserts that all structural artifacts (column, constraints,
// indexes) are present.
func TestMigration_RoundTrip_0008(t *testing.T) {
	ctx := context.Background()
	db := openMigratedDB(t)

	if err := goose.DownToContext(ctx, db, "migrations", 7); err != nil {
		t.Fatalf("goose down to 7: %v", err)
	}
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		t.Fatalf("goose up: %v", err)
	}

	// Assert instruments.user_id column exists.
	var colExists bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'instruments' AND column_name = 'user_id'
		)
	`).Scan(&colExists); err != nil {
		t.Fatal(err)
	}
	if !colExists {
		t.Error("instruments.user_id column not found after migration 0008")
	}

	// Assert both CHECK constraints are present.
	var checkCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pg_constraint
		WHERE conrelid = 'instruments'::regclass
		  AND conname IN ('instruments_scope_class_check', 'instruments_asset_class_check')
	`).Scan(&checkCount); err != nil {
		t.Fatal(err)
	}
	if checkCount != 2 {
		t.Errorf("expected 2 CHECK constraints, got %d", checkCount)
	}

	// Assert both partial unique indexes are present.
	var idxCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM pg_indexes
		WHERE tablename = 'instruments'
		  AND indexname IN (
		      'instruments_ticker_asset_class_global_uidx',
		      'instruments_ticker_asset_class_user_uidx'
		  )
	`).Scan(&idxCount); err != nil {
		t.Fatal(err)
	}
	if idxCount != 2 {
		t.Errorf("expected 2 partial unique indexes, got %d", idxCount)
	}
}

// TestMigration_InvariantImmutability_Forward verifies that making a personal
// (user-owned) real_estate row global (user_id = NULL) violates the
// instruments_scope_class_check constraint.
func TestMigration_InvariantImmutability_Forward(t *testing.T) {
	ctx := context.Background()
	db := openMigratedDB(t)

	userID := uuid.Must(uuid.NewV7())
	instrID := uuid.Must(uuid.NewV7())

	if _, err := db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'hash')`,
		userID, fmt.Sprintf("%s@test.local", userID),
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if _, err := db.ExecContext(ctx,
		`INSERT INTO instruments (id, user_id, ticker, asset_class, currency, name)
		 VALUES ($1, $2, 'HOUSE', 'real_estate', 'RUB', 'House')`,
		instrID, userID,
	); err != nil {
		t.Fatalf("insert personal instrument: %v", err)
	}

	// Attempting to clear user_id must violate scope_class_check.
	_, err := db.ExecContext(ctx,
		`UPDATE instruments SET user_id = NULL WHERE id = $1`, instrID,
	)
	if err == nil {
		t.Fatal("expected constraint violation, got nil error")
	}
	if !strings.Contains(err.Error(), "instruments_scope_class_check") {
		t.Errorf("expected instruments_scope_class_check violation, got: %v", err)
	}
}

// TestMigration_InvariantImmutability_Reverse verifies that assigning a user_id
// to a global exchange instrument violates the instruments_scope_class_check
// constraint.
func TestMigration_InvariantImmutability_Reverse(t *testing.T) {
	ctx := context.Background()
	db := openMigratedDB(t)

	userID := uuid.Must(uuid.NewV7())
	instrID := uuid.Must(uuid.NewV7())

	if _, err := db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'hash')`,
		userID, fmt.Sprintf("%s@test.local", userID),
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	if _, err := db.ExecContext(ctx,
		`INSERT INTO instruments (id, user_id, ticker, asset_class, currency, name)
		 VALUES ($1, NULL, 'SBER', 'ru_stock', 'RUB', 'Sberbank')`,
		instrID,
	); err != nil {
		t.Fatalf("insert global instrument: %v", err)
	}

	// Attempting to set a user_id on a global instrument must violate scope_class_check.
	_, err := db.ExecContext(ctx,
		`UPDATE instruments SET user_id = $1 WHERE id = $2`, userID, instrID,
	)
	if err == nil {
		t.Fatal("expected constraint violation, got nil error")
	}
	if !strings.Contains(err.Error(), "instruments_scope_class_check") {
		t.Errorf("expected instruments_scope_class_check violation, got: %v", err)
	}
}
