package portfolio

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/fx"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
)

type Service struct {
	q  *storage.Queries
	fx *fx.Service
}

func NewService(q *storage.Queries, fxSvc *fx.Service) *Service {
	return &Service{q: q, fx: fxSvc}
}

type Position struct {
	AccountID      uuid.UUID
	AccountName    string
	InstrumentID   uuid.UUID
	Ticker         string
	AssetClass     string
	Currency       string
	Quantity       decimal.Decimal
	Price          *decimal.Decimal
	PriceFetchedAt *time.Time
	ValueNative    *decimal.Decimal
	ValueDisplay   *decimal.Decimal
	PriceStale     bool
}

type Summary struct {
	DisplayCurrency string
	GrandTotal      decimal.Decimal
	ByAssetClass    map[string]decimal.Decimal
	ByCurrency      map[string]decimal.Decimal
	ByAccount       map[string]decimal.Decimal
}

type Portfolio struct {
	Summary   Summary
	Positions []Position
}

// Stale price threshold: if fetched_at older than this, mark as stale.
const priceFreshness = 24 * time.Hour

// Compute aggregates the portfolio of a user, converting values into displayCurrency
// where FX rates are available. Positions without prices or rates are surfaced
// with priceStale=true and nil ValueDisplay.
func (s *Service) Compute(ctx context.Context, userID uuid.UUID, displayCurrency string) (Portfolio, error) {
	displayCurrency = strings.ToUpper(strings.TrimSpace(displayCurrency))
	if displayCurrency == "" {
		displayCurrency = "RUB"
	}

	rows, err := s.q.GetUserPortfolioRows(ctx, userID)
	if err != nil {
		return Portfolio{}, fmt.Errorf("load rows: %w", err)
	}

	now := time.Now()
	out := Portfolio{
		Summary: Summary{
			DisplayCurrency: displayCurrency,
			GrandTotal:      decimal.Zero,
			ByAssetClass:    map[string]decimal.Decimal{},
			ByCurrency:      map[string]decimal.Decimal{},
			ByAccount:       map[string]decimal.Decimal{},
		},
		Positions: make([]Position, 0, len(rows)),
	}

	for _, row := range rows {
		pos := Position{
			AccountID:    row.AccountID,
			AccountName:  row.AccountName,
			InstrumentID: row.InstrumentID,
			Ticker:       row.InstrumentTicker,
			AssetClass:   row.InstrumentAssetClass,
			Currency:     row.InstrumentCurrency,
			Quantity:     row.Quantity,
		}

		if row.Price.Valid {
			price := row.Price.Decimal
			pos.Price = &price
			// Cash positions are pegged 1:1 to their currency by definition, so
			// fetched_at has no meaning — surface it as null and never mark stale.
			if row.FetchedAt.Valid && pos.AssetClass != "cash" {
				t := row.FetchedAt.Time
				pos.PriceFetchedAt = &t
				if now.Sub(t) > priceFreshness {
					pos.PriceStale = true
				}
			}
			val := pos.Quantity.Mul(price)
			pos.ValueNative = &val

			rate, err := s.fx.GetRate(ctx, pos.Currency, displayCurrency)
			if err == nil {
				display := val.Mul(rate)
				pos.ValueDisplay = &display
				out.Summary.GrandTotal = out.Summary.GrandTotal.Add(display)
				out.Summary.ByAssetClass[pos.AssetClass] = sumOr(out.Summary.ByAssetClass[pos.AssetClass], display)
				out.Summary.ByAccount[pos.AccountID.String()] = sumOr(out.Summary.ByAccount[pos.AccountID.String()], display)
			} else if !errors.Is(err, fx.ErrRateUnavailable) {
				return Portfolio{}, fmt.Errorf("fx lookup: %w", err)
			} else {
				pos.PriceStale = true
			}

			out.Summary.ByCurrency[pos.Currency] = sumOr(out.Summary.ByCurrency[pos.Currency], val)
		} else {
			pos.PriceStale = true
		}

		out.Positions = append(out.Positions, pos)
	}

	return out, nil
}

func sumOr(prev, add decimal.Decimal) decimal.Decimal {
	return prev.Add(add)
}
