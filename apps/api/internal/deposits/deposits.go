package deposits

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/storage"
)

var ErrNotFound = errors.New("deposit: not found")

type Deposit struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Month     time.Time
	Amount    decimal.Decimal
	CreatedAt time.Time
}

type Service struct {
	q *storage.Queries
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{q: storage.New(pool)}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, month time.Time, amount decimal.Decimal) (Deposit, error) {
	row, err := s.q.CreateDeposit(ctx, storage.CreateDepositParams{
		ID:     uuid.Must(uuid.NewV7()),
		UserID: userID,
		Month:  pgtype.Date{Time: month, Valid: true},
		Amount: amount,
	})
	if err != nil {
		return Deposit{}, fmt.Errorf("create deposit: %w", err)
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
		Month:     row.Month.Time,
		Amount:    row.Amount,
		CreatedAt: row.CreatedAt.Time,
	}
}
