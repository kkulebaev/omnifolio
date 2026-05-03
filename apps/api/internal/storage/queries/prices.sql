-- name: UpsertPrice :exec
INSERT INTO prices (instrument_id, price, fetched_at)
VALUES ($1, $2, now())
ON CONFLICT (instrument_id) DO UPDATE
SET price = EXCLUDED.price, fetched_at = EXCLUDED.fetched_at;

-- name: GetPrice :one
SELECT instrument_id, price, fetched_at FROM prices WHERE instrument_id = $1;
