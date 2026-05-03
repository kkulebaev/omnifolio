// Package source defines the abstractions for external position/price providers
// (T-Invest, Bybit, etc.). Concrete implementations live in subpackages
// internal/source/tinvest, internal/source/bybit.
package source

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Common errors that providers should return so the account service can map
// them to user-visible messages and HTTP statuses.
var (
	ErrTokenInvalid     = errors.New("source: token invalid")
	ErrRateLimited      = errors.New("source: rate limited")
	ErrSubAccountNotFound = errors.New("source: sub-account not found")
	ErrInstrumentUnknown  = errors.New("source: instrument unknown")
)

// Position is a raw position pulled from a broker — referenced by source-native
// identifier (e.g. FIGI for T-Invest). It must be resolved to a canonical
// internal instrument before being stored.
type Position struct {
	NativeInstrumentID string
	Quantity           decimal.Decimal
}

// InstrumentSeed is what the source knows about a freshly-encountered instrument.
// The account service uses it to upsert into the canonical `instruments` table.
type InstrumentSeed struct {
	Ticker     string
	AssetClass string // ru_stock, ru_bond, ru_etf, us_stock, us_etf, crypto, cash
	Currency   string
	Name       string
}

// ResolvedInstrument couples a canonical internal instrument with its
// source-native id, so a price provider can call the right RPC.
type ResolvedInstrument struct {
	InstrumentID       uuid.UUID
	NativeInstrumentID string
	AssetClass         string
	Currency           string
}

// Price is a per-instrument quote. Currency is the native instrument currency
// (e.g. USD for AAPL); the caller converts to display currency separately.
type Price struct {
	Amount    decimal.Decimal
	Currency  string
	FetchedAt time.Time
}

// SubAccount is a child of a broker user (T-Invest's БРОКЕР / ИИС / ПРЕМИУМ).
// One Omnifolio account maps to exactly one SubAccount.
type SubAccount struct {
	ID   string
	Name string
	Type string // BROKER, IIS, PREMIUM, … (source-defined)
}

// PositionSource is implemented by sources that can list a sub-account's
// current positions and resolve unknown native instruments to a seed.
type PositionSource interface {
	// ListSubAccounts is called during onboarding to let the user pick which
	// sub-account this Omnifolio account should mirror.
	ListSubAccounts(ctx context.Context, creds []byte) ([]SubAccount, error)

	// Sync returns the snapshot of positions for the given sub-account.
	Sync(ctx context.Context, creds []byte, subAccountID string) ([]Position, error)

	// ResolveInstrument fills metadata for a not-yet-cached native instrument id.
	ResolveInstrument(ctx context.Context, creds []byte, nativeID string) (InstrumentSeed, error)
}

// PriceProvider returns latest prices for a batch of resolved instruments.
type PriceProvider interface {
	GetPrices(ctx context.Context, creds []byte, instruments []ResolvedInstrument) (map[uuid.UUID]Price, error)
}

// Registry wires sources into the rest of the app. There are two flavors of
// price provider:
//   - Prices: paired with a broker source_type, called with that broker's creds
//     and the instrument's source-native id (e.g. FIGI for T-Invest).
//   - PricesByAssetClass: standalone price feeds (e.g. Finnhub for US stocks)
//     that don't require user credentials and key off asset_class instead.
type Registry struct {
	Positions          map[string]PositionSource // key: account.source_type
	Prices             map[string]PriceProvider  // key: account.source_type
	PricesByAssetClass map[string]PriceProvider  // key: instrument.asset_class
}

// NewRegistry returns a registry with all maps initialized.
func NewRegistry() *Registry {
	return &Registry{
		Positions:          make(map[string]PositionSource),
		Prices:             make(map[string]PriceProvider),
		PricesByAssetClass: make(map[string]PriceProvider),
	}
}
