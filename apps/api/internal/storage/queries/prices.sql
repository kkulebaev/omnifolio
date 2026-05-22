-- name: UpsertPrice :exec
-- Low-level upsert with no ownership check. Internal use only; service-level
-- callers should use UpsertGlobalPrice (admin/cron path) or UpsertPersonalPrice
-- (user-driven path) so ownership is enforced at the SQL level.
INSERT INTO prices (instrument_id, price, fetched_at)
VALUES ($1, $2, now())
ON CONFLICT (instrument_id) DO UPDATE
SET price = EXCLUDED.price, fetched_at = EXCLUDED.fetched_at;

-- name: UpsertPersonalPrice :execrows
-- Ownership predicate is inside the same statement: the row is only written when
-- the target instrument is owned by the caller. rows_affected = 0 means either
-- the instrument does not exist or it does not belong to the caller (both 404).
INSERT INTO prices (instrument_id, price, fetched_at)
SELECT i.id, $2, now()
FROM instruments i
WHERE i.id = $1 AND i.user_id = $3
ON CONFLICT (instrument_id) DO UPDATE
SET price = EXCLUDED.price, fetched_at = EXCLUDED.fetched_at;

-- name: UpsertGlobalPrice :execrows
-- Admin/cron path: only writes when the target instrument is global
-- (user_id IS NULL). rows_affected = 0 means the instrument is missing or
-- personal — admin must not touch personal price rows.
INSERT INTO prices (instrument_id, price, fetched_at)
SELECT i.id, $2, now()
FROM instruments i
WHERE i.id = $1 AND i.user_id IS NULL
ON CONFLICT (instrument_id) DO UPDATE
SET price = EXCLUDED.price, fetched_at = EXCLUDED.fetched_at;

-- name: GetPrice :one
SELECT instrument_id, price, fetched_at FROM prices WHERE instrument_id = $1;
