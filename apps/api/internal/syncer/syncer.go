// Package syncer orchestrates a position sync for a brokerage account: fetch
// positions from source.PositionSource, resolve unknown instruments, upsert
// positions, update account.last_sync_status. Prices are not touched here —
// they are refreshed by the apps/cron service through the admin API.
package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/crypto"
	"github.com/kkulebaev/omnifolio/api/internal/source"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
)

func extractSubAccountID(sourceType string, creds []byte) (string, error) {
	switch sourceType {
	case "tinvest":
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

type Service struct {
	pool     *pgxpool.Pool
	q        *storage.Queries
	enc      *crypto.Encryptor
	registry *source.Registry
	log      *slog.Logger
}

func NewService(pool *pgxpool.Pool, enc *crypto.Encryptor, registry *source.Registry, log *slog.Logger) *Service {
	return &Service{pool: pool, q: storage.New(pool), enc: enc, registry: registry, log: log}
}

// Sync performs a full sync for a single account. Manual accounts are no-op.
// Errors are persisted into accounts.last_sync_status / last_sync_error.
func (s *Service) Sync(ctx context.Context, accountID uuid.UUID) error {
	acc, err := s.q.GetAccountByID(ctx, accountID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("syncer: account %s not found", accountID)
		}
		return fmt.Errorf("syncer: get account: %w", err)
	}
	if acc.SourceType == "manual" {
		return nil
	}

	posSource, ok := s.registry.Positions[acc.SourceType]
	if !ok {
		return s.markFailed(ctx, accountID, fmt.Errorf("no PositionSource for source_type %q", acc.SourceType))
	}

	creds, err := s.loadCreds(ctx, accountID)
	if err != nil {
		return s.markFailed(ctx, accountID, fmt.Errorf("load creds: %w", err))
	}

	subAccountID, err := extractSubAccountID(acc.SourceType, creds)
	if err != nil {
		return s.markFailed(ctx, accountID, err)
	}

	rawPositions, err := posSource.Sync(ctx, creds, subAccountID)
	if err != nil {
		return s.markFailed(ctx, accountID, mapSourceErr(err))
	}

	resolved, err := s.resolveAll(ctx, posSource, creds, acc.SourceType, rawPositions)
	if err != nil {
		return s.markFailed(ctx, accountID, fmt.Errorf("resolve: %w", err))
	}

	if err := s.applyPositions(ctx, accountID, resolved); err != nil {
		return s.markFailed(ctx, accountID, fmt.Errorf("apply positions: %w", err))
	}

	now := time.Now()
	if err := s.q.SetAccountSyncStatus(ctx, storage.SetAccountSyncStatusParams{
		ID:             accountID,
		LastSyncStatus: ptr("success"),
		LastSyncedAt:   pgtype.Timestamptz{Time: now, Valid: true},
		LastSyncError:  nil,
	}); err != nil {
		s.log.Error("syncer: set status success failed", "account_id", accountID, "err", err)
	}
	s.log.Info("syncer: synced", "account_id", accountID, "positions", len(resolved))
	return nil
}

// SyncAll iterates over all brokerage accounts; one bad apple doesn't stop the run.
func (s *Service) SyncAll(ctx context.Context) error {
	accs, err := s.q.ListSyncableAccounts(ctx)
	if err != nil {
		return fmt.Errorf("list syncable: %w", err)
	}
	for _, a := range accs {
		if err := s.Sync(ctx, a.ID); err != nil {
			s.log.Error("syncer: account failed", "account_id", a.ID, "err", err)
		}
	}
	return nil
}

// loadCreds reads + decrypts the account_credentials blob.
func (s *Service) loadCreds(ctx context.Context, accountID uuid.UUID) ([]byte, error) {
	row, err := s.q.GetAccountCredentials(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("get credentials: %w", err)
	}
	plain, err := s.enc.Decrypt(row.Ciphertext, row.Nonce, accountID[:])
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plain, nil
}

type resolvedPosition struct {
	Position   source.Position
	Instrument source.ResolvedInstrument
}

// resolveAll maps native ids to canonical instruments, creating new ones when needed.
func (s *Service) resolveAll(
	ctx context.Context, posSource source.PositionSource, creds []byte,
	sourceType string, raw []source.Position,
) ([]resolvedPosition, error) {
	out := make([]resolvedPosition, 0, len(raw))
	for _, p := range raw {
		row, err := s.q.GetInstrumentByExternalID(ctx, storage.GetInstrumentByExternalIDParams{
			Source:   sourceType,
			NativeID: p.NativeInstrumentID,
		})
		if err == nil {
			out = append(out, resolvedPosition{
				Position: p,
				Instrument: source.ResolvedInstrument{
					InstrumentID:       row.ID,
					NativeInstrumentID: p.NativeInstrumentID,
					AssetClass:         row.AssetClass,
					Currency:           row.Currency,
				},
			})
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("lookup instrument: %w", err)
		}

		seed, err := posSource.ResolveInstrument(ctx, creds, p.NativeInstrumentID)
		if err != nil {
			if errors.Is(err, source.ErrInstrumentUnknown) {
				s.log.Warn("syncer: skip unsupported instrument",
					"native_id", p.NativeInstrumentID, "err", err)
				continue
			}
			return nil, fmt.Errorf("resolve instrument: %w", err)
		}

		newRow, err := s.q.UpsertInstrumentBySeed(ctx, storage.UpsertInstrumentBySeedParams{
			ID:         uuid.Must(uuid.NewV7()),
			Ticker:     seed.Ticker,
			AssetClass: seed.AssetClass,
			Currency:   seed.Currency,
			Name:       seed.Name,
		})
		if err != nil {
			return nil, fmt.Errorf("upsert instrument: %w", err)
		}
		if err := s.q.UpsertInstrumentExternalID(ctx, storage.UpsertInstrumentExternalIDParams{
			Source:       sourceType,
			NativeID:     p.NativeInstrumentID,
			InstrumentID: newRow.ID,
		}); err != nil {
			return nil, fmt.Errorf("upsert external id: %w", err)
		}

		// Cash positions get a fixed price=1.00 right away.
		if seed.AssetClass == "cash" {
			_ = s.q.UpsertPrice(ctx, storage.UpsertPriceParams{
				InstrumentID: newRow.ID,
				Price:        decimal.NewFromInt(1),
			})
		}

		out = append(out, resolvedPosition{
			Position: p,
			Instrument: source.ResolvedInstrument{
				InstrumentID:       newRow.ID,
				NativeInstrumentID: p.NativeInstrumentID,
				AssetClass:         newRow.AssetClass,
				Currency:           newRow.Currency,
			},
		})
	}
	return out, nil
}

// applyPositions UPSERTs the snapshot in a single transaction. Orphan positions
// (in DB but not in the new snapshot) are deleted.
func (s *Service) applyPositions(ctx context.Context, accountID uuid.UUID, resolved []resolvedPosition) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := s.q.WithTx(tx)

	locked, err := qtx.TryLockAccountSync(ctx, accountID.String())
	if err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	if !locked {
		return errors.New("syncer: account already syncing")
	}

	keep := make([]uuid.UUID, 0, len(resolved))
	for _, r := range resolved {
		if err := qtx.UpsertPosition(ctx, storage.UpsertPositionParams{
			AccountID:    accountID,
			InstrumentID: r.Instrument.InstrumentID,
			Quantity:     r.Position.Quantity,
		}); err != nil {
			return fmt.Errorf("upsert position: %w", err)
		}
		keep = append(keep, r.Instrument.InstrumentID)
	}
	if err := qtx.DeleteOrphanPositions(ctx, storage.DeleteOrphanPositionsParams{
		AccountID: accountID,
		Column2:   keep,
	}); err != nil {
		return fmt.Errorf("delete orphans: %w", err)
	}
	return tx.Commit(ctx)
}

// markFailed records the failure reason on the account row.
func (s *Service) markFailed(ctx context.Context, accountID uuid.UUID, cause error) error {
	msg := cause.Error()
	if err := s.q.SetAccountSyncStatus(ctx, storage.SetAccountSyncStatusParams{
		ID:             accountID,
		LastSyncStatus: ptr("failed"),
		LastSyncedAt:   pgtype.Timestamptz{Valid: false},
		LastSyncError:  &msg,
	}); err != nil {
		s.log.Error("syncer: set status failed itself failed",
			"account_id", accountID, "err", err)
	}
	s.log.Warn("syncer: failed", "account_id", accountID, "err", cause)
	return cause
}

func mapSourceErr(err error) error {
	switch {
	case errors.Is(err, source.ErrTokenInvalid):
		return errors.New("Токен отклонён T-Invest. Удалите и создайте аккаунт заново.")
	case errors.Is(err, source.ErrRateLimited):
		return errors.New("Превышен лимит запросов T-Invest. Попробуйте позже.")
	case errors.Is(err, source.ErrSubAccountNotFound):
		return errors.New("Sub-аккаунт T-Invest не найден.")
	}
	return err
}

func ptr[T any](v T) *T { return &v }
