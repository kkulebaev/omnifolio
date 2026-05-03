package position

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/account"
	"github.com/kkulebaev/omnifolio/api/internal/instrument"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
)

var (
	ErrAccountNotFound    = errors.New("position: account not found")
	ErrInstrumentNotFound = errors.New("position: instrument not found")
	ErrAlreadyExists      = errors.New("position: already exists")
	ErrNotFound           = errors.New("position: not found")
)

type Position struct {
	AccountID    uuid.UUID
	InstrumentID uuid.UUID
	Quantity     decimal.Decimal
	UpdatedAt    time.Time
}

type EnrichedPosition struct {
	Position
	Instrument instrument.Instrument
}

type Service struct {
	q        *storage.Queries
	accounts *account.Service
	insts    *instrument.Service
}

func NewService(q *storage.Queries, accounts *account.Service, insts *instrument.Service) *Service {
	return &Service{q: q, accounts: accounts, insts: insts}
}

// Create adds a new position to an account.
func (s *Service) Create(ctx context.Context, userID, accountID, instrumentID uuid.UUID, quantity decimal.Decimal) (Position, error) {
	if _, err := s.accounts.Get(ctx, userID, accountID); err != nil {
		if errors.Is(err, account.ErrNotFound) {
			return Position{}, ErrAccountNotFound
		}
		return Position{}, fmt.Errorf("ownership check: %w", err)
	}
	if _, err := s.insts.Get(ctx, instrumentID); err != nil {
		if errors.Is(err, instrument.ErrNotFound) {
			return Position{}, ErrInstrumentNotFound
		}
		return Position{}, fmt.Errorf("instrument lookup: %w", err)
	}

	row, err := s.q.CreatePosition(ctx, storage.CreatePositionParams{
		AccountID:    accountID,
		InstrumentID: instrumentID,
		Quantity:     quantity,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Position{}, ErrAlreadyExists
		}
		return Position{}, fmt.Errorf("create position: %w", err)
	}
	return toPosition(row), nil
}

// Update changes the quantity of an existing position.
func (s *Service) Update(ctx context.Context, userID, accountID, instrumentID uuid.UUID, quantity decimal.Decimal) (Position, error) {
	if _, err := s.accounts.Get(ctx, userID, accountID); err != nil {
		if errors.Is(err, account.ErrNotFound) {
			return Position{}, ErrAccountNotFound
		}
		return Position{}, fmt.Errorf("ownership check: %w", err)
	}

	row, err := s.q.UpdatePositionQuantity(ctx, storage.UpdatePositionQuantityParams{
		AccountID:    accountID,
		InstrumentID: instrumentID,
		Quantity:     quantity,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Position{}, ErrNotFound
		}
		return Position{}, fmt.Errorf("update position: %w", err)
	}
	return toPosition(row), nil
}

// Delete removes a position.
func (s *Service) Delete(ctx context.Context, userID, accountID, instrumentID uuid.UUID) error {
	if _, err := s.accounts.Get(ctx, userID, accountID); err != nil {
		if errors.Is(err, account.ErrNotFound) {
			return ErrAccountNotFound
		}
		return fmt.Errorf("ownership check: %w", err)
	}

	rows, err := s.q.DeletePosition(ctx, storage.DeletePositionParams{
		AccountID:    accountID,
		InstrumentID: instrumentID,
	})
	if err != nil {
		return fmt.Errorf("delete position: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// ListForAccount returns positions of an account, with embedded Instrument data.
func (s *Service) ListForAccount(ctx context.Context, userID, accountID uuid.UUID) ([]EnrichedPosition, error) {
	if _, err := s.accounts.Get(ctx, userID, accountID); err != nil {
		if errors.Is(err, account.ErrNotFound) {
			return nil, ErrAccountNotFound
		}
		return nil, fmt.Errorf("ownership check: %w", err)
	}

	rows, err := s.q.ListAccountPositionsWithInstrument(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}
	out := make([]EnrichedPosition, len(rows))
	for i, row := range rows {
		out[i] = EnrichedPosition{
			Position: Position{
				AccountID:    row.AccountID,
				InstrumentID: row.InstrumentID,
				Quantity:     row.Quantity,
				UpdatedAt:    row.UpdatedAt.Time,
			},
			Instrument: instrument.Instrument{
				ID:         row.IID,
				Ticker:     row.ITicker,
				AssetClass: row.IAssetClass,
				Currency:   row.ICurrency,
				Name:       row.IName,
				CreatedAt:  row.ICreatedAt.Time,
				UpdatedAt:  row.IUpdatedAt.Time,
			},
		}
	}
	return out, nil
}

func toPosition(row storage.Position) Position {
	return Position{
		AccountID:    row.AccountID,
		InstrumentID: row.InstrumentID,
		Quantity:     row.Quantity,
		UpdatedAt:    row.UpdatedAt.Time,
	}
}
