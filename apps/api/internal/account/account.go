package account

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kkulebaev/omnifolio/api/internal/crypto"
	"github.com/kkulebaev/omnifolio/api/internal/source"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
)

var (
	ErrNotFound         = errors.New("account: not found")
	ErrTypeNotSupported = errors.New("account: type not supported in this version")
	ErrTokenInvalid     = errors.New("account: token invalid")
)

const (
	TypeManual  = "manual"
	TypeTInvest = "tinvest"
	TypeBybit   = "bybit"
)

type Account struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	SourceType     string
	Name           string
	LastSyncedAt   *time.Time
	LastSyncStatus *string
	LastSyncError  *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateInput struct {
	Name             string
	Type             string
	TInvestToken     string
	TInvestAccountID string
	BybitAPIKey      string
	BybitAPISecret   string
}

type Service struct {
	pool     *pgxpool.Pool
	q        *storage.Queries
	enc      *crypto.Encryptor
	registry *source.Registry
}

func NewService(pool *pgxpool.Pool, enc *crypto.Encryptor, registry *source.Registry) *Service {
	return &Service{pool: pool, q: storage.New(pool), enc: enc, registry: registry}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, in CreateInput) (Account, error) {
	switch in.Type {
	case TypeManual:
		row, err := s.q.CreateAccount(ctx, storage.CreateAccountParams{
			ID:         uuid.Must(uuid.NewV7()),
			UserID:     userID,
			SourceType: in.Type,
			Name:       in.Name,
		})
		if err != nil {
			return Account{}, fmt.Errorf("create account: %w", err)
		}
		return toAccount(row), nil
	case TypeTInvest:
		return s.createBrokerage(ctx, userID, in, encodeTInvestCreds(in))
	case TypeBybit:
		return s.createBrokerage(ctx, userID, in, encodeBybitCreds(in))
	default:
		return Account{}, ErrTypeNotSupported
	}
}

// createBrokerage handles the atomic create of a brokerage-typed account
// + its encrypted credentials in a single transaction.
func (s *Service) createBrokerage(ctx context.Context, userID uuid.UUID, in CreateInput, plainCreds []byte) (Account, error) {
	// Validate token + sub-account by querying the source first (rejects bad tokens early).
	if err := s.validateBrokerageCreds(ctx, in.Type, plainCreds); err != nil {
		return Account{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Account{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	id := uuid.Must(uuid.NewV7())
	row, err := qtx.CreateAccount(ctx, storage.CreateAccountParams{
		ID:         id,
		UserID:     userID,
		SourceType: in.Type,
		Name:       in.Name,
	})
	if err != nil {
		return Account{}, fmt.Errorf("create account row: %w", err)
	}

	ciphertext, nonce, err := s.enc.Encrypt(plainCreds, id[:])
	if err != nil {
		return Account{}, fmt.Errorf("encrypt creds: %w", err)
	}
	if err := qtx.UpsertAccountCredentials(ctx, storage.UpsertAccountCredentialsParams{
		AccountID:  id,
		Ciphertext: ciphertext,
		Nonce:      nonce,
		KeyVersion: int32(s.enc.KeyVersion()),
	}); err != nil {
		return Account{}, fmt.Errorf("insert credentials: %w", err)
	}

	// Mark pending so UI shows spinner while async sync runs.
	pending := "pending"
	if err := qtx.SetAccountSyncStatus(ctx, storage.SetAccountSyncStatusParams{
		ID:             id,
		LastSyncStatus: &pending,
	}); err != nil {
		return Account{}, fmt.Errorf("set pending: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Account{}, fmt.Errorf("commit: %w", err)
	}

	row.LastSyncStatus = &pending
	return toAccount(row), nil
}

// validateBrokerageCreds calls ListSubAccounts on the matching source to confirm
// the token + sub-account id are valid before we persist them.
func (s *Service) validateBrokerageCreds(ctx context.Context, sourceType string, plainCreds []byte) error {
	src, ok := s.registry.Positions[sourceType]
	if !ok {
		return ErrTypeNotSupported
	}
	subs, err := src.ListSubAccounts(ctx, plainCreds)
	if err != nil {
		if errors.Is(err, source.ErrTokenInvalid) {
			return ErrTokenInvalid
		}
		return fmt.Errorf("validate creds: %w", err)
	}

	wanted, err := extractSubAccountID(sourceType, plainCreds)
	if err != nil {
		return err
	}
	if wanted == "" {
		// Sources without sub-account selection (e.g. bybit) — accept any first.
		if len(subs) == 0 {
			return source.ErrSubAccountNotFound
		}
		return nil
	}
	for _, s := range subs {
		if s.ID == wanted {
			return nil
		}
	}
	return source.ErrSubAccountNotFound
}

// PreviewTInvest validates a bare token and returns its sub-accounts.
func (s *Service) PreviewTInvest(ctx context.Context, token string) ([]source.SubAccount, error) {
	src, ok := s.registry.Positions[TypeTInvest]
	if !ok {
		return nil, ErrTypeNotSupported
	}
	creds, _ := json.Marshal(map[string]string{"token": token})
	subs, err := src.ListSubAccounts(ctx, creds)
	if err != nil {
		if errors.Is(err, source.ErrTokenInvalid) {
			return nil, ErrTokenInvalid
		}
		return nil, fmt.Errorf("preview: %w", err)
	}
	return subs, nil
}

func (s *Service) Get(ctx context.Context, userID, accountID uuid.UUID) (Account, error) {
	row, err := s.q.GetAccountByUser(ctx, storage.GetAccountByUserParams{
		ID:     accountID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Account{}, ErrNotFound
		}
		return Account{}, fmt.Errorf("get account: %w", err)
	}
	return toAccount(row), nil
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]Account, error) {
	rows, err := s.q.ListAccountsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	out := make([]Account, len(rows))
	for i, row := range rows {
		out[i] = toAccount(row)
	}
	return out, nil
}

func (s *Service) Rename(ctx context.Context, userID, accountID uuid.UUID, name string) (Account, error) {
	row, err := s.q.UpdateAccountName(ctx, storage.UpdateAccountNameParams{
		ID:     accountID,
		UserID: userID,
		Name:   name,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Account{}, ErrNotFound
		}
		return Account{}, fmt.Errorf("update account: %w", err)
	}
	return toAccount(row), nil
}

func (s *Service) Delete(ctx context.Context, userID, accountID uuid.UUID) error {
	rows, err := s.q.DeleteAccount(ctx, storage.DeleteAccountParams{
		ID:     accountID,
		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

type ActiveCreds struct {
	AccountID  uuid.UUID
	SourceType string
	Plain      []byte
}

// LoadActiveBrokerageCreds returns decrypted credentials for all brokerage
// accounts owned by user. Used by pricecache to pick a valid token for cross-source
// price refresh.
func (s *Service) LoadActiveBrokerageCreds(ctx context.Context, userID uuid.UUID) ([]ActiveCreds, error) {
	rows, err := s.q.ListUserBrokerageAccounts(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list brokerage accounts: %w", err)
	}
	out := make([]ActiveCreds, 0, len(rows))
	for _, row := range rows {
		plain, err := s.enc.Decrypt(row.Ciphertext, row.Nonce, row.ID[:])
		if err != nil {
			continue
		}
		out = append(out, ActiveCreds{
			AccountID:  row.ID,
			SourceType: row.SourceType,
			Plain:      plain,
		})
	}
	return out, nil
}

func toAccount(row storage.Account) Account {
	a := Account{
		ID:             row.ID,
		UserID:         row.UserID,
		SourceType:     row.SourceType,
		Name:           row.Name,
		LastSyncStatus: row.LastSyncStatus,
		LastSyncError:  row.LastSyncError,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}
	if row.LastSyncedAt.Valid {
		t := row.LastSyncedAt.Time
		a.LastSyncedAt = &t
	}
	return a
}

func encodeTInvestCreds(in CreateInput) []byte {
	b, _ := json.Marshal(map[string]string{
		"token":            in.TInvestToken,
		"tinvestAccountId": in.TInvestAccountID,
	})
	return b
}

func encodeBybitCreds(in CreateInput) []byte {
	b, _ := json.Marshal(map[string]string{
		"apiKey":    in.BybitAPIKey,
		"apiSecret": in.BybitAPISecret,
	})
	return b
}

func extractSubAccountID(sourceType string, creds []byte) (string, error) {
	switch sourceType {
	case TypeTInvest:
		var c struct {
			TInvestAccountID string `json:"tinvestAccountId"`
		}
		if err := json.Unmarshal(creds, &c); err != nil {
			return "", fmt.Errorf("creds parse: %w", err)
		}
		if c.TInvestAccountID == "" {
			return "", source.ErrSubAccountNotFound
		}
		return c.TInvestAccountID, nil
	}
	return "", nil
}
