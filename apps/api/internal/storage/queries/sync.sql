-- name: ListSyncableAccounts :many
SELECT id, user_id, source_type, name, last_synced_at, last_sync_status, last_sync_error, created_at, updated_at
FROM accounts
WHERE source_type IN ('tinvest', 'bybit');

-- name: GetAccountCredentials :one
SELECT account_id, ciphertext, nonce, key_version, created_at, updated_at
FROM account_credentials
WHERE account_id = $1;

-- name: UpsertAccountCredentials :exec
INSERT INTO account_credentials (account_id, ciphertext, nonce, key_version)
VALUES ($1, $2, $3, $4)
ON CONFLICT (account_id) DO UPDATE
SET ciphertext = EXCLUDED.ciphertext,
    nonce = EXCLUDED.nonce,
    key_version = EXCLUDED.key_version,
    updated_at = now();

-- name: UpsertPosition :exec
INSERT INTO positions (account_id, instrument_id, quantity)
VALUES ($1, $2, $3)
ON CONFLICT (account_id, instrument_id) DO UPDATE
SET quantity = EXCLUDED.quantity,
    updated_at = now();

-- name: DeleteOrphanPositions :exec
DELETE FROM positions
WHERE account_id = $1
  AND NOT (instrument_id = ANY($2::uuid[]));

-- name: SetAccountSyncStatus :exec
UPDATE accounts
SET last_synced_at = COALESCE($3, last_synced_at),
    last_sync_status = $2,
    last_sync_error = $4,
    updated_at = now()
WHERE id = $1;

-- name: TryLockAccountSync :one
SELECT pg_try_advisory_xact_lock(hashtext('sync:' || $1::text)) AS acquired;

-- name: UpsertInstrumentBySeed :one
WITH ins AS (
    INSERT INTO instruments (id, ticker, asset_class, currency, name)
    VALUES ($1, $2, $3, $4, $5)
    ON CONFLICT (LOWER(ticker), asset_class) DO NOTHING
    RETURNING id, ticker, asset_class, currency, name, created_at, updated_at
)
SELECT id, ticker, asset_class, currency, name, created_at, updated_at FROM ins
UNION ALL
SELECT id, ticker, asset_class, currency, name, created_at, updated_at
FROM instruments
WHERE LOWER(ticker) = LOWER($2) AND asset_class = $3
  AND NOT EXISTS (SELECT 1 FROM ins);

-- name: UpsertInstrumentExternalID :exec
INSERT INTO instrument_external_ids (source, native_id, instrument_id)
VALUES ($1, $2, $3)
ON CONFLICT (source, native_id) DO NOTHING;

-- name: GetInstrumentByExternalID :one
SELECT i.id, i.ticker, i.asset_class, i.currency, i.name, i.created_at, i.updated_at
FROM instruments i
JOIN instrument_external_ids x ON x.instrument_id = i.id
WHERE x.source = $1 AND x.native_id = $2;
