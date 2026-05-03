// Package pricecache lazily refreshes per-instrument prices that exceed a
// per-asset-class TTL. Designed to be called from the /portfolio handler so
// dashboard reads keep prices warm without a separate cron loop.
package pricecache

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kkulebaev/omnifolio/api/internal/source"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
)

// TTLs by asset_class. Anything outside this map is treated as ttl=5min.
var defaultTTLs = map[string]time.Duration{
	"crypto":   60 * time.Second,
	"ru_stock": 5 * time.Minute,
	"ru_bond":  10 * time.Minute,
	"ru_etf":   5 * time.Minute,
	"us_stock": 5 * time.Minute,
	"us_etf":   5 * time.Minute,
	"cash":     365 * 24 * time.Hour, // never refresh cash
}

func ttlFor(assetClass string) time.Duration {
	if v, ok := defaultTTLs[assetClass]; ok {
		return v
	}
	return 5 * time.Minute
}

type AccountCreds struct {
	AccountID  uuid.UUID
	SourceType string
	Plain      []byte
}

// CredsLoader resolves all of a user's brokerage credentials so RefreshStale
// can pick the right token for each instrument's source.
type CredsLoader interface {
	LoadActive(ctx context.Context, userID uuid.UUID) ([]AccountCreds, error)
}

type Cache struct {
	q        *storage.Queries
	registry *source.Registry
	loader   CredsLoader
	log      *slog.Logger

	mu       sync.Mutex
	inFlight map[uuid.UUID]bool // instrument_id → refresh-in-flight
}

func New(q *storage.Queries, registry *source.Registry, loader CredsLoader, log *slog.Logger) *Cache {
	return &Cache{
		q:        q,
		registry: registry,
		loader:   loader,
		log:      log,
		inFlight: make(map[uuid.UUID]bool),
	}
}

// RefreshStaleAsync looks at the supplied instruments and kicks an async refresh
// for any whose latest price is older than its asset-class TTL. Returns
// immediately; callers serve cached data and let the next read see fresh data.
func (c *Cache) RefreshStaleAsync(rootCtx context.Context, userID uuid.UUID, instruments []Instrument) {
	stale := c.pickStale(rootCtx, instruments)
	if len(stale) == 0 {
		return
	}
	go c.refresh(context.WithoutCancel(rootCtx), userID, stale)
}

type Instrument struct {
	ID         uuid.UUID
	AssetClass string
	// Ticker is required for price-only providers keyed off asset_class
	// (e.g. Finnhub for us_stock). Brokerage providers ignore it.
	Ticker string
}

// pickStale returns instruments whose price is missing or older than ttl.
func (c *Cache) pickStale(ctx context.Context, instruments []Instrument) []Instrument {
	now := time.Now()
	out := make([]Instrument, 0, len(instruments))
	for _, inst := range instruments {
		ttl := ttlFor(inst.AssetClass)
		row, err := c.q.GetPrice(ctx, inst.ID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				out = append(out, inst)
			}
			continue
		}
		if now.Sub(row.FetchedAt.Time) > ttl {
			out = append(out, inst)
		}
	}
	return out
}

func (c *Cache) refresh(ctx context.Context, userID uuid.UUID, instruments []Instrument) {
	claimed := c.claim(instruments)
	defer c.release(idsOf(claimed))
	if len(claimed) == 0 {
		return
	}

	// Split into price-only (keyed by asset_class, e.g. Finnhub for us_stock)
	// and brokerage-backed (keyed by external_ids.source + creds).
	var (
		byAssetClass = make(map[string][]source.ResolvedInstrument)
		brokerage    []Instrument
	)
	for _, inst := range claimed {
		if _, ok := c.registry.PricesByAssetClass[inst.AssetClass]; ok {
			if inst.Ticker == "" {
				continue
			}
			byAssetClass[inst.AssetClass] = append(byAssetClass[inst.AssetClass], source.ResolvedInstrument{
				InstrumentID:       inst.ID,
				NativeInstrumentID: inst.Ticker,
				AssetClass:         inst.AssetClass,
			})
			continue
		}
		brokerage = append(brokerage, inst)
	}

	for assetClass, insts := range byAssetClass {
		provider := c.registry.PricesByAssetClass[assetClass]
		prices, err := provider.GetPrices(ctx, nil, insts)
		if err != nil {
			c.log.Warn("pricecache: price-only provider failed", "asset_class", assetClass, "err", err)
			continue
		}
		c.upsertPrices(ctx, prices)
	}

	if len(brokerage) > 0 {
		c.refreshBrokerage(ctx, userID, brokerage)
	}

	c.log.Info("pricecache: refreshed", "user_id", userID, "instruments", len(claimed))
}

// refreshBrokerage handles instruments whose price comes from the broker that
// reported them: needs the user's encrypted creds plus the source-native id
// from instrument_external_ids.
func (c *Cache) refreshBrokerage(ctx context.Context, userID uuid.UUID, instruments []Instrument) {
	credsList, err := c.loader.LoadActive(ctx, userID)
	if err != nil {
		c.log.Warn("pricecache: load creds", "err", err)
		return
	}
	credsBySource := make(map[string]AccountCreds, len(credsList))
	for _, cr := range credsList {
		if _, ok := credsBySource[cr.SourceType]; !ok {
			credsBySource[cr.SourceType] = cr
		}
	}

	bySource := make(map[string][]source.ResolvedInstrument)
	for _, inst := range instruments {
		exts, err := c.q.ListExternalIDsForInstrument(ctx, inst.ID)
		if err != nil {
			continue
		}
		for _, ext := range exts {
			if _, ok := credsBySource[ext.Source]; !ok {
				continue
			}
			bySource[ext.Source] = append(bySource[ext.Source], source.ResolvedInstrument{
				InstrumentID:       inst.ID,
				NativeInstrumentID: ext.NativeID,
				AssetClass:         inst.AssetClass,
			})
			break
		}
	}

	for sourceType, insts := range bySource {
		provider, ok := c.registry.Prices[sourceType]
		if !ok {
			continue
		}
		prices, err := provider.GetPrices(ctx, credsBySource[sourceType].Plain, insts)
		if err != nil {
			c.log.Warn("pricecache: provider failed", "source", sourceType, "err", err)
			continue
		}
		c.upsertPrices(ctx, prices)
	}
}

func (c *Cache) upsertPrices(ctx context.Context, prices map[uuid.UUID]source.Price) {
	for instID, p := range prices {
		if err := c.q.UpsertPrice(ctx, storage.UpsertPriceParams{
			InstrumentID: instID,
			Price:        p.Amount,
		}); err != nil {
			c.log.Warn("pricecache: upsert price", "instrument_id", instID, "err", err)
		}
	}
}

func idsOf(instruments []Instrument) []uuid.UUID {
	out := make([]uuid.UUID, len(instruments))
	for i, inst := range instruments {
		out[i] = inst.ID
	}
	return out
}

// claim filters out instruments that are already being refreshed.
func (c *Cache) claim(instruments []Instrument) []Instrument {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Instrument, 0, len(instruments))
	for _, inst := range instruments {
		if c.inFlight[inst.ID] {
			continue
		}
		c.inFlight[inst.ID] = true
		out = append(out, inst)
	}
	return out
}

func (c *Cache) release(ids []uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, id := range ids {
		delete(c.inFlight, id)
	}
}
