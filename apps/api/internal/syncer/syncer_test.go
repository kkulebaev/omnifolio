package syncer_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/crypto"
	"github.com/kkulebaev/omnifolio/api/internal/source"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
	"github.com/kkulebaev/omnifolio/api/internal/syncer"
	"github.com/kkulebaev/omnifolio/api/internal/testutil"
)

type fakeSource struct {
	positions []source.Position
	seeds     map[string]source.InstrumentSeed
}

func (f *fakeSource) ListSubAccounts(context.Context, []byte) ([]source.SubAccount, error) {
	return nil, nil
}

func (f *fakeSource) Sync(context.Context, []byte, string) ([]source.Position, error) {
	return f.positions, nil
}

func (f *fakeSource) ResolveInstrument(_ context.Context, _ []byte, nativeID string) (source.InstrumentSeed, error) {
	seed, ok := f.seeds[nativeID]
	if !ok {
		return source.InstrumentSeed{}, source.ErrInstrumentUnknown
	}
	return seed, nil
}

type fixture struct {
	q       *storage.Queries
	svc     *syncer.Service
	src     *fakeSource
	account uuid.UUID
}

func setup(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	pool := testutil.NewRawPool(t)
	if err := storage.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	q := storage.New(pool)

	user := uuid.Must(uuid.NewV7())
	if _, err := q.CreateUser(ctx, storage.CreateUserParams{
		ID: user, Email: user.String() + "@test.local", PasswordHash: "x", DisplayCurrency: "RUB",
	}); err != nil {
		t.Fatal(err)
	}
	account := uuid.Must(uuid.NewV7())
	if _, err := q.CreateAccount(ctx, storage.CreateAccountParams{
		ID: account, UserID: user, SourceType: "tinvest", Name: "T-Invest",
	}); err != nil {
		t.Fatal(err)
	}

	enc, err := crypto.NewEncryptor(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	ct, nonce, err := enc.Encrypt([]byte(`{"token":"t","tinvestAccountId":"sub"}`), account[:])
	if err != nil {
		t.Fatal(err)
	}
	if err := q.UpsertAccountCredentials(ctx, storage.UpsertAccountCredentialsParams{
		AccountID: account, Ciphertext: ct, Nonce: nonce, KeyVersion: int32(enc.KeyVersion()),
	}); err != nil {
		t.Fatal(err)
	}

	src := &fakeSource{seeds: map[string]source.InstrumentSeed{}}
	reg := source.NewRegistry()
	reg.Positions["tinvest"] = src
	return fixture{q: q, svc: syncer.NewService(pool, enc, reg, slog.Default()), src: src, account: account}
}

func (f fixture) positionCount(t *testing.T) int64 {
	t.Helper()
	n, err := f.q.CountAccountPositions(context.Background(), f.account)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Unknown FIGIs go through UpsertInstrumentBySeed, which failed with 42P10
// once 0008 made the global ticker index partial.
func TestSync_ResolvesUnknownInstrument(t *testing.T) {
	ctx := context.Background()
	f := setup(t)

	if _, err := f.q.CreateInstrument(ctx, storage.CreateInstrumentParams{
		ID: uuid.Must(uuid.NewV7()), Ticker: "SBER", AssetClass: "ru_stock", Currency: "RUB", Name: "Сбербанк",
	}); err != nil {
		t.Fatal(err)
	}
	f.src.seeds["FIGI_SBER"] = source.InstrumentSeed{Ticker: "SBER", AssetClass: "ru_stock", Currency: "RUB", Name: "Сбербанк"}
	f.src.seeds["FIGI_NEW"] = source.InstrumentSeed{Ticker: "NEWCO", AssetClass: "ru_stock", Currency: "RUB", Name: "New Co"}
	f.src.positions = []source.Position{
		{NativeInstrumentID: "FIGI_SBER", Quantity: decimal.NewFromInt(10)},
		{NativeInstrumentID: "FIGI_NEW", Quantity: decimal.NewFromInt(5)},
	}

	if err := f.svc.Sync(ctx, f.account); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if n := f.positionCount(t); n != 2 {
		t.Fatalf("positions = %d, want 2", n)
	}
	sber, err := f.q.GetInstrumentByExternalID(ctx, storage.GetInstrumentByExternalIDParams{Source: "tinvest", NativeID: "FIGI_SBER"})
	if err != nil {
		t.Fatal(err)
	}
	if sber.Name != "Сбербанк" {
		t.Errorf("FIGI_SBER mapped to %q, want the existing global SBER", sber.Name)
	}
}

func TestSync_EmptySnapshotKeepsPositions(t *testing.T) {
	ctx := context.Background()
	f := setup(t)

	f.src.seeds["FIGI_SBER"] = source.InstrumentSeed{Ticker: "SBER", AssetClass: "ru_stock", Currency: "RUB", Name: "Сбербанк"}
	f.src.positions = []source.Position{{NativeInstrumentID: "FIGI_SBER", Quantity: decimal.NewFromInt(10)}}
	if err := f.svc.Sync(ctx, f.account); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	f.src.positions = nil
	err := f.svc.Sync(ctx, f.account)
	if !errors.Is(err, syncer.ErrEmptySnapshot) {
		t.Fatalf("err = %v, want ErrEmptySnapshot", err)
	}
	if n := f.positionCount(t); n != 1 {
		t.Fatalf("positions = %d, want 1 kept", n)
	}
	acc, err := f.q.GetAccountByID(ctx, f.account)
	if err != nil {
		t.Fatal(err)
	}
	if acc.LastSyncStatus == nil || *acc.LastSyncStatus != "failed" {
		t.Errorf("last_sync_status = %v, want failed", acc.LastSyncStatus)
	}
}

func TestSync_EmptySnapshotOnEmptyAccountSucceeds(t *testing.T) {
	f := setup(t)
	if err := f.svc.Sync(context.Background(), f.account); err != nil {
		t.Fatalf("sync: %v", err)
	}
}
