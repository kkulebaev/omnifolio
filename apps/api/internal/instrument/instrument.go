package instrument

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/storage"
)

var (
	ErrNotFound = errors.New("instrument: not found")
)

const (
	AssetClassRUStock = "ru_stock"
	AssetClassRUBond  = "ru_bond"
	AssetClassRUETF   = "ru_etf"
	AssetClassUSStock = "us_stock"
	AssetClassUSETF   = "us_etf"
	AssetClassCrypto  = "crypto"
)

type Instrument struct {
	ID             uuid.UUID
	Ticker         string
	AssetClass     string
	Currency       string
	Name           string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CurrentPrice   *decimal.Decimal
	PriceFetchedAt *time.Time
}

type CreateInput struct {
	Ticker     string
	AssetClass string
	Currency   string
	Name       string
}

type Service struct {
	q *storage.Queries
}

func NewService(q *storage.Queries) *Service {
	return &Service{q: q}
}

// CreateOrGet implements idempotent create: if an instrument with the same
// (LOWER(ticker), asset_class) already exists — returns it; otherwise creates
// a new one.
func (s *Service) CreateOrGet(ctx context.Context, in CreateInput) (Instrument, error) {
	in.Ticker = strings.ToUpper(strings.TrimSpace(in.Ticker))
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.Name = strings.TrimSpace(in.Name)

	existing, err := s.q.GetInstrumentByTickerAssetClass(ctx, storage.GetInstrumentByTickerAssetClassParams{
		Lower:      in.Ticker,
		AssetClass: in.AssetClass,
	})
	if err == nil {
		return toInstrument(existing), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Instrument{}, fmt.Errorf("lookup: %w", err)
	}

	row, err := s.q.CreateInstrument(ctx, storage.CreateInstrumentParams{
		ID:         uuid.Must(uuid.NewV7()),
		Ticker:     in.Ticker,
		AssetClass: in.AssetClass,
		Currency:   in.Currency,
		Name:       in.Name,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// race: someone created the same instrument between our SELECT and INSERT
			existing, lookupErr := s.q.GetInstrumentByTickerAssetClass(ctx, storage.GetInstrumentByTickerAssetClassParams{
				Lower:      in.Ticker,
				AssetClass: in.AssetClass,
			})
			if lookupErr != nil {
				return Instrument{}, fmt.Errorf("post-conflict lookup: %w", lookupErr)
			}
			return toInstrument(existing), nil
		}
		return Instrument{}, fmt.Errorf("create instrument: %w", err)
	}
	return toInstrument(row), nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Instrument, error) {
	row, err := s.q.GetInstrumentByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Instrument{}, ErrNotFound
		}
		return Instrument{}, fmt.Errorf("get instrument: %w", err)
	}
	return toInstrument(row), nil
}

func (s *Service) Search(ctx context.Context, q string) ([]Instrument, error) {
	rows, err := s.q.SearchInstruments(ctx, &q)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	out := make([]Instrument, len(rows))
	for i, row := range rows {
		out[i] = toInstrument(row)
	}
	return out, nil
}

type ListInput struct {
	Q          string
	AssetClass string
	Limit      int32
	Offset     int32
}

type ListResult struct {
	Items []Instrument
	Total int64
}

func (s *Service) List(ctx context.Context, in ListInput) (ListResult, error) {
	rows, err := s.q.ListInstruments(ctx, storage.ListInstrumentsParams{
		Q:          in.Q,
		AssetClass: in.AssetClass,
		Lim:        in.Limit,
		Off:        in.Offset,
	})
	if err != nil {
		return ListResult{}, fmt.Errorf("list: %w", err)
	}
	total, err := s.q.CountInstruments(ctx, storage.CountInstrumentsParams{
		Q:          in.Q,
		AssetClass: in.AssetClass,
	})
	if err != nil {
		return ListResult{}, fmt.Errorf("count: %w", err)
	}
	items := make([]Instrument, len(rows))
	for i, row := range rows {
		items[i] = toInstrumentFromListRow(row)
	}
	return ListResult{Items: items, Total: total}, nil
}

func toInstrument(row storage.Instrument) Instrument {
	return Instrument{
		ID:         row.ID,
		Ticker:     row.Ticker,
		AssetClass: row.AssetClass,
		Currency:   row.Currency,
		Name:       row.Name,
		CreatedAt:  row.CreatedAt.Time,
		UpdatedAt:  row.UpdatedAt.Time,
	}
}

func toInstrumentFromListRow(row storage.ListInstrumentsRow) Instrument {
	out := Instrument{
		ID:         row.ID,
		Ticker:     row.Ticker,
		AssetClass: row.AssetClass,
		Currency:   row.Currency,
		Name:       row.Name,
		CreatedAt:  row.CreatedAt.Time,
		UpdatedAt:  row.UpdatedAt.Time,
	}
	if row.CurrentPrice.Valid {
		v := row.CurrentPrice.Decimal
		out.CurrentPrice = &v
	}
	if row.PriceFetchedAt.Valid {
		t := row.PriceFetchedAt.Time
		out.PriceFetchedAt = &t
	}
	return out
}
