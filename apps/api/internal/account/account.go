package account

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kkulebaev/omnifolio/api/internal/storage"
)

var (
	ErrNotFound         = errors.New("account: not found")
	ErrTypeNotSupported = errors.New("account: type not supported in this version")
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
	Name string
	Type string
}

type Service struct {
	q *storage.Queries
}

func NewService(q *storage.Queries) *Service {
	return &Service{q: q}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, in CreateInput) (Account, error) {
	if in.Type != TypeManual {
		return Account{}, ErrTypeNotSupported
	}

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
