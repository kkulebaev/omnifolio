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
	ErrNotFound          = errors.New("instrument: not found")
	ErrAlreadyExists     = errors.New("instrument: already exists")
	ErrHasPositions      = errors.New("instrument: has positions")
	ErrInvalidAssetClass = errors.New("instrument: invalid asset class for scope")
)

const (
	AssetClassRUStock    = "ru_stock"
	AssetClassRUBond     = "ru_bond"
	AssetClassRUETF      = "ru_etf"
	AssetClassUSStock    = "us_stock"
	AssetClassUSETF      = "us_etf"
	AssetClassCrypto     = "crypto"
	AssetClassRealEstate = "real_estate"
	AssetClassVehicle    = "vehicle"
	AssetClassOtherAsset = "other_asset"
)

// Personal-only asset classes. Members of this set are the only ones a user may
// create / rename / delete through the user-facing API.
var personalAssetClasses = map[string]struct{}{
	AssetClassRealEstate: {},
	AssetClassVehicle:    {},
	AssetClassOtherAsset: {},
}

// Scope constants used by List for filtering.
const (
	ScopeMine   = "mine"
	ScopeGlobal = "global"
	ScopeAny    = ""
)

type Instrument struct {
	ID             uuid.UUID
	UserID         *uuid.UUID // nil = global catalog row
	Ticker         string
	AssetClass     string
	Currency       string
	Name           string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CurrentPrice   *decimal.Decimal
	PriceFetchedAt *time.Time
}

// Scope returns "personal" if the instrument is owned by a user, else "global".
func (i Instrument) Scope() string {
	if i.UserID != nil {
		return "personal"
	}
	return "global"
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

// CreateOrGet implements idempotent create of a GLOBAL instrument: if a global
// row with the same (LOWER(ticker), asset_class) already exists — returns it;
// otherwise creates a new one. Personal instruments must go through CreatePersonal.
func (s *Service) CreateOrGet(ctx context.Context, in CreateInput) (Instrument, error) {
	in.Ticker = strings.ToUpper(strings.TrimSpace(in.Ticker))
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.Name = strings.TrimSpace(in.Name)

	existing, err := s.q.GetGlobalInstrumentByTickerAssetClass(ctx, storage.GetGlobalInstrumentByTickerAssetClassParams{
		Lower:      in.Ticker,
		AssetClass: in.AssetClass,
	})
	if err == nil {
		return toInstrumentFromGlobalRow(existing), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Instrument{}, fmt.Errorf("lookup: %w", err)
	}

	row, err := s.q.CreateInstrument(ctx, storage.CreateInstrumentParams{
		ID:         uuid.Must(uuid.NewV7()),
		UserID:     uuid.NullUUID{Valid: false},
		Ticker:     in.Ticker,
		AssetClass: in.AssetClass,
		Currency:   in.Currency,
		Name:       in.Name,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// race: someone created the same global instrument between SELECT and INSERT
			existing, lookupErr := s.q.GetGlobalInstrumentByTickerAssetClass(ctx, storage.GetGlobalInstrumentByTickerAssetClassParams{
				Lower:      in.Ticker,
				AssetClass: in.AssetClass,
			})
			if lookupErr != nil {
				return Instrument{}, fmt.Errorf("post-conflict lookup: %w", lookupErr)
			}
			return toInstrumentFromGlobalRow(existing), nil
		}
		return Instrument{}, fmt.Errorf("create instrument: %w", err)
	}
	return toInstrumentFromCreateRow(row), nil
}

// CreatePersonal inserts a user-owned instrument (real_estate / vehicle /
// other_asset only) and seeds its price atomically.
func (s *Service) CreatePersonal(ctx context.Context, userID uuid.UUID, in CreateInput, initialPrice decimal.Decimal) (Instrument, error) {
	if _, ok := personalAssetClasses[in.AssetClass]; !ok {
		return Instrument{}, ErrInvalidAssetClass
	}

	in.Ticker = strings.ToUpper(strings.TrimSpace(in.Ticker))
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.Name = strings.TrimSpace(in.Name)

	id := uuid.Must(uuid.NewV7())
	row, err := s.q.CreateInstrument(ctx, storage.CreateInstrumentParams{
		ID:         id,
		UserID:     uuid.NullUUID{UUID: userID, Valid: true},
		Ticker:     in.Ticker,
		AssetClass: in.AssetClass,
		Currency:   in.Currency,
		Name:       in.Name,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			switch pgErr.Code {
			case "23505":
				return Instrument{}, ErrAlreadyExists
			case "23514":
				return Instrument{}, ErrInvalidAssetClass
			}
		}
		return Instrument{}, fmt.Errorf("create personal instrument: %w", err)
	}

	rows, err := s.q.UpsertPersonalPrice(ctx, storage.UpsertPersonalPriceParams{
		ID:     id,
		Price:  initialPrice,
		UserID: uuid.NullUUID{UUID: userID, Valid: true},
	})
	if err != nil {
		return Instrument{}, fmt.Errorf("seed price: %w", err)
	}
	if rows == 0 {
		// The instrument we just inserted has vanished or doesn't belong to the
		// caller — should be impossible given the prior INSERT inside this call.
		return Instrument{}, fmt.Errorf("seed price: instrument missing after insert")
	}

	out := toInstrumentFromCreateRow(row)
	out.CurrentPrice = &initialPrice
	now := time.Now()
	out.PriceFetchedAt = &now
	return out, nil
}

// Rename updates the name and/or ticker of a personal instrument the user owns.
func (s *Service) Rename(ctx context.Context, userID, instrumentID uuid.UUID, name, ticker *string) (Instrument, error) {
	var tickerArg *string
	if ticker != nil {
		t := strings.ToUpper(strings.TrimSpace(*ticker))
		tickerArg = &t
	}
	var nameArg *string
	if name != nil {
		n := strings.TrimSpace(*name)
		nameArg = &n
	}

	row, err := s.q.UpdateInstrumentMeta(ctx, storage.UpdateInstrumentMetaParams{
		ID:     instrumentID,
		UserID: uuid.NullUUID{UUID: userID, Valid: true},
		Ticker: tickerArg,
		Name:   nameArg,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Instrument{}, ErrNotFound
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Instrument{}, ErrAlreadyExists
		}
		return Instrument{}, fmt.Errorf("update instrument: %w", err)
	}
	return toInstrumentFromUpdateRow(row), nil
}

// DeletePersonal removes a user-owned instrument. Returns ErrHasPositions when
// positions still reference it (FK ON DELETE RESTRICT raises 23503).
func (s *Service) DeletePersonal(ctx context.Context, userID, instrumentID uuid.UUID) error {
	rows, err := s.q.DeletePersonalInstrument(ctx, storage.DeletePersonalInstrumentParams{
		ID:     instrumentID,
		UserID: uuid.NullUUID{UUID: userID, Valid: true},
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return ErrHasPositions
		}
		return fmt.Errorf("delete personal instrument: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPrice upserts the current price for a user-owned instrument.
func (s *Service) SetPrice(ctx context.Context, userID, instrumentID uuid.UUID, price decimal.Decimal) error {
	rows, err := s.q.UpsertPersonalPrice(ctx, storage.UpsertPersonalPriceParams{
		ID:     instrumentID,
		Price:  price,
		UserID: uuid.NullUUID{UUID: userID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("upsert personal price: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// Get returns an instrument by id. Personal rows owned by a different user are
// treated as not found (404, not 403, per design.md §3.9).
func (s *Service) Get(ctx context.Context, userID, id uuid.UUID) (Instrument, error) {
	row, err := s.q.GetInstrumentByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Instrument{}, ErrNotFound
		}
		return Instrument{}, fmt.Errorf("get instrument: %w", err)
	}
	if row.UserID.Valid && row.UserID.UUID != userID {
		return Instrument{}, ErrNotFound
	}
	return toInstrumentFromGetRow(row), nil
}

func (s *Service) Search(ctx context.Context, userID uuid.UUID, q string) ([]Instrument, error) {
	rows, err := s.q.SearchInstruments(ctx, storage.SearchInstrumentsParams{
		Column1: &q,
		UserID:  uuid.NullUUID{UUID: userID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	out := make([]Instrument, len(rows))
	for i, row := range rows {
		out[i] = toInstrumentFromSearchRow(row)
	}
	return out, nil
}

type ListInput struct {
	Q          string
	AssetClass string
	Scope      string
	Limit      int32
	Offset     int32
}

type ListResult struct {
	Items []Instrument
	Total int64
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, in ListInput) (ListResult, error) {
	caller := uuid.NullUUID{UUID: userID, Valid: userID != uuid.Nil}
	rows, err := s.q.ListInstruments(ctx, storage.ListInstrumentsParams{
		Q:          in.Q,
		AssetClass: in.AssetClass,
		Scope:      in.Scope,
		CallerID:   caller,
		Lim:        in.Limit,
		Off:        in.Offset,
	})
	if err != nil {
		return ListResult{}, fmt.Errorf("list: %w", err)
	}
	total, err := s.q.CountInstruments(ctx, storage.CountInstrumentsParams{
		Q:          in.Q,
		AssetClass: in.AssetClass,
		Scope:      in.Scope,
		CallerID:   caller,
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

func nullableUserID(n uuid.NullUUID) *uuid.UUID {
	if !n.Valid {
		return nil
	}
	v := n.UUID
	return &v
}

func toInstrumentFromGlobalRow(row storage.GetGlobalInstrumentByTickerAssetClassRow) Instrument {
	return Instrument{
		ID:         row.ID,
		UserID:     nullableUserID(row.UserID),
		Ticker:     row.Ticker,
		AssetClass: row.AssetClass,
		Currency:   row.Currency,
		Name:       row.Name,
		CreatedAt:  row.CreatedAt.Time,
		UpdatedAt:  row.UpdatedAt.Time,
	}
}

func toInstrumentFromCreateRow(row storage.CreateInstrumentRow) Instrument {
	return Instrument{
		ID:         row.ID,
		UserID:     nullableUserID(row.UserID),
		Ticker:     row.Ticker,
		AssetClass: row.AssetClass,
		Currency:   row.Currency,
		Name:       row.Name,
		CreatedAt:  row.CreatedAt.Time,
		UpdatedAt:  row.UpdatedAt.Time,
	}
}

func toInstrumentFromGetRow(row storage.GetInstrumentByIDRow) Instrument {
	return Instrument{
		ID:         row.ID,
		UserID:     nullableUserID(row.UserID),
		Ticker:     row.Ticker,
		AssetClass: row.AssetClass,
		Currency:   row.Currency,
		Name:       row.Name,
		CreatedAt:  row.CreatedAt.Time,
		UpdatedAt:  row.UpdatedAt.Time,
	}
}

func toInstrumentFromSearchRow(row storage.SearchInstrumentsRow) Instrument {
	return Instrument{
		ID:         row.ID,
		UserID:     nullableUserID(row.UserID),
		Ticker:     row.Ticker,
		AssetClass: row.AssetClass,
		Currency:   row.Currency,
		Name:       row.Name,
		CreatedAt:  row.CreatedAt.Time,
		UpdatedAt:  row.UpdatedAt.Time,
	}
}

func toInstrumentFromUpdateRow(row storage.UpdateInstrumentMetaRow) Instrument {
	return Instrument{
		ID:         row.ID,
		UserID:     nullableUserID(row.UserID),
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
		UserID:     nullableUserID(row.UserID),
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
