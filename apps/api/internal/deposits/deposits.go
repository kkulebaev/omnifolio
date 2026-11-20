package deposits

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/storage"
)

var ErrNotFound = errors.New("deposit: not found")

const (
	KindDeposit    = "deposit"
	KindWithdrawal = "withdrawal"
)

// Deposit is a single external cash flow. Amount is always positive; the
// direction lives in Kind.
type Deposit struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Date      time.Time
	Kind      string
	Currency  string
	Amount    decimal.Decimal
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Input struct {
	Date     time.Time
	Kind     string
	Currency string
	Amount   decimal.Decimal
}

type Service struct {
	q *storage.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: storage.New(pool)}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, in Input) (Deposit, error) {
	row, err := s.q.CreateDeposit(ctx, storage.CreateDepositParams{
		ID:       uuid.Must(uuid.NewV7()),
		UserID:   userID,
		Date:     pgtype.Date{Time: in.Date, Valid: true},
		Kind:     in.Kind,
		Currency: in.Currency,
		Amount:   in.Amount,
	})
	if err != nil {
		return Deposit{}, fmt.Errorf("create deposit: %w", err)
	}
	return toDeposit(row), nil
}

func (s *Service) Update(ctx context.Context, userID, depositID uuid.UUID, in Input) (Deposit, error) {
	row, err := s.q.UpdateDeposit(ctx, storage.UpdateDepositParams{
		ID:       depositID,
		UserID:   userID,
		Date:     pgtype.Date{Time: in.Date, Valid: true},
		Kind:     in.Kind,
		Currency: in.Currency,
		Amount:   in.Amount,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Deposit{}, ErrNotFound
	}
	if err != nil {
		return Deposit{}, fmt.Errorf("update deposit: %w", err)
	}
	return toDeposit(row), nil
}

func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]Deposit, error) {
	rows, err := s.q.ListDepositsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list deposits: %w", err)
	}
	out := make([]Deposit, len(rows))
	for i, r := range rows {
		out[i] = toDeposit(r)
	}
	return out, nil
}

func (s *Service) Delete(ctx context.Context, userID, depositID uuid.UUID) error {
	rows, err := s.q.DeleteDeposit(ctx, storage.DeleteDepositParams{
		ID:     depositID,
		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("delete deposit: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func toDeposit(row storage.Deposit) Deposit {
	return Deposit{
		ID:        row.ID,
		UserID:    row.UserID,
		Date:      row.Date.Time,
		Kind:      row.Kind,
		Currency:  row.Currency,
		Amount:    row.Amount,
		CreatedAt: row.CreatedAt.Time,
		UpdatedAt: row.UpdatedAt.Time,
	}
}
