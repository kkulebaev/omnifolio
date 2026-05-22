// Package testutil provides helpers for integration tests that need a real
// Postgres database. It deliberately does NOT import the storage package so
// that test files in package storage can import testutil without creating a
// cycle.
package testutil

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

// NewRawPool creates an isolated, empty Postgres database whose name is derived
// from t.Name(), applies no migrations, and returns a ready pool. Cleanup
// (pool.Close + DROP DATABASE) is registered via t.Cleanup so callers never
// have to remember to tear down.
//
// The test is skipped when DATABASE_URL is not set, keeping CI green when no
// Postgres is available.
func NewRawPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	baseURL := os.Getenv("DATABASE_URL")
	if baseURL == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx := context.Background()
	dbName := testDBName(t.Name())
	adminDSN := swapDB(baseURL, "postgres")

	adminPool, err := pgxpool.New(ctx, adminDSN)
	if err != nil {
		t.Fatalf("testutil: admin connect: %v", err)
	}

	// Drop any leftover from a previous failed run, then create fresh.
	adminPool.Exec(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, dbName))
	if _, err := adminPool.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %q`, dbName)); err != nil {
		adminPool.Close()
		t.Fatalf("testutil: create db %q: %v", dbName, err)
	}
	adminPool.Close()

	testDSN := swapDB(baseURL, dbName)
	cfg, err := pgxpool.ParseConfig(testDSN)
	if err != nil {
		t.Fatalf("testutil: parse dsn: %v", err)
	}
	cfg.MaxConns = 5

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("testutil: connect %q: %v", dbName, err)
	}

	t.Cleanup(func() {
		pool.Close()
		ap, err := pgxpool.New(context.Background(), adminDSN)
		if err == nil {
			ap.Exec(context.Background(), fmt.Sprintf(`DROP DATABASE IF EXISTS %q WITH (FORCE)`, dbName))
			ap.Close()
		}
	})

	return pool
}

// testDBName converts a test name into a valid, short Postgres identifier.
func testDBName(testName string) string {
	name := strings.ToLower(nonAlnum.ReplaceAllString(testName, "_"))
	if len(name) > 55 {
		name = name[:55]
	}
	return "t_" + name
}

// swapDB returns the DSN with the database component replaced by newDB.
func swapDB(dsn, newDB string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		panic(fmt.Sprintf("testutil: invalid DSN %q: %v", dsn, err))
	}
	u.Path = "/" + newDB
	return u.String()
}
