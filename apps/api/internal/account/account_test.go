package account_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/kkulebaev/omnifolio/api/internal/account"
	"github.com/kkulebaev/omnifolio/api/internal/crypto"
	"github.com/kkulebaev/omnifolio/api/internal/source"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
	"github.com/kkulebaev/omnifolio/api/internal/testutil"
)

// fakeSource accepts only tokens/keys listed in valid and exposes a single
// T-Invest sub-account "sub-1".
type fakeSource struct {
	valid map[string]bool
}

func (f *fakeSource) ListSubAccounts(_ context.Context, creds []byte) ([]source.SubAccount, error) {
	var c struct {
		Token  string `json:"token"`
		APIKey string `json:"apiKey"`
	}
	if err := json.Unmarshal(creds, &c); err != nil {
		return nil, err
	}
	if !f.valid[c.Token+c.APIKey] {
		return nil, source.ErrTokenInvalid
	}
	return []source.SubAccount{{ID: "sub-1", Name: "Брокерский", Type: "BROKER"}}, nil
}

func (f *fakeSource) Sync(context.Context, []byte, string) ([]source.Position, error) {
	return nil, nil
}

func (f *fakeSource) ResolveInstrument(context.Context, []byte, string) (source.InstrumentSeed, error) {
	return source.InstrumentSeed{}, source.ErrInstrumentUnknown
}

type fixture struct {
	svc  *account.Service
	q    *storage.Queries
	enc  *crypto.Encryptor
	src  *fakeSource
	user uuid.UUID
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
	enc, err := crypto.NewEncryptor(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	src := &fakeSource{valid: map[string]bool{}}
	reg := source.NewRegistry()
	reg.Positions[account.TypeTInvest] = src
	reg.Positions[account.TypeBybit] = src
	return fixture{svc: account.NewService(pool, enc, reg), q: q, enc: enc, src: src, user: user}
}

func (f fixture) storedCreds(t *testing.T, id uuid.UUID) map[string]string {
	t.Helper()
	row, err := f.q.GetAccountCredentials(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := f.enc.Decrypt(row.Ciphertext, row.Nonce, id[:])
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	if err := json.Unmarshal(plain, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUpdateCredentials_TInvestKeepsSubAccount(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	f.src.valid["old-token"] = true
	acc, err := f.svc.Create(ctx, f.user, account.CreateInput{
		Name: "T", Type: account.TypeTInvest, TInvestToken: "old-token", TInvestAccountID: "sub-1",
	})
	if err != nil {
		t.Fatal(err)
	}

	f.src.valid["new-token"] = true
	got, err := f.svc.UpdateCredentials(ctx, f.user, acc.ID, account.CredentialsInput{TInvestToken: "new-token"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.LastSyncStatus == nil || *got.LastSyncStatus != "pending" {
		t.Errorf("status = %v, want pending", got.LastSyncStatus)
	}
	creds := f.storedCreds(t, acc.ID)
	if creds["token"] != "new-token" || creds["tinvestAccountId"] != "sub-1" {
		t.Errorf("stored creds = %v", creds)
	}
}

func TestUpdateCredentials_RejectedKeyLeavesOldCreds(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	f.src.valid["old-key"] = true
	acc, err := f.svc.Create(ctx, f.user, account.CreateInput{
		Name: "B", Type: account.TypeBybit, BybitAPIKey: "old-key", BybitAPISecret: "old-secret",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.svc.UpdateCredentials(ctx, f.user, acc.ID, account.CredentialsInput{APIKey: "bad-key", APISecret: "bad-secret"})
	if !errors.Is(err, account.ErrTokenInvalid) {
		t.Fatalf("err = %v, want ErrTokenInvalid", err)
	}
	if creds := f.storedCreds(t, acc.ID); creds["apiKey"] != "old-key" {
		t.Errorf("apiKey = %q, want old-key kept", creds["apiKey"])
	}
}

func TestUpdateCredentials_ManualAccount(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	acc, err := f.svc.Create(ctx, f.user, account.CreateInput{Name: "M", Type: account.TypeManual})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.UpdateCredentials(ctx, f.user, acc.ID, account.CredentialsInput{APIKey: "k"}); !errors.Is(err, account.ErrNotBrokerage) {
		t.Fatalf("err = %v, want ErrNotBrokerage", err)
	}
}

func TestUpdateCredentials_OtherUsersAccount(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	f.src.valid["k"] = true
	acc, err := f.svc.Create(ctx, f.user, account.CreateInput{Name: "B", Type: account.TypeBybit, BybitAPIKey: "k", BybitAPISecret: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.UpdateCredentials(ctx, uuid.Must(uuid.NewV7()), acc.ID, account.CredentialsInput{APIKey: "k", APISecret: "s"}); !errors.Is(err, account.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
