-- name: CreateInstrument :one
INSERT INTO instruments (id, user_id, ticker, asset_class, currency, name)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, user_id, ticker, asset_class, currency, name, created_at, updated_at;

-- name: GetGlobalInstrumentByTickerAssetClass :one
SELECT id, user_id, ticker, asset_class, currency, name, created_at, updated_at
FROM instruments
WHERE LOWER(ticker) = LOWER($1) AND asset_class = $2 AND user_id IS NULL;

-- name: GetUserInstrumentByTickerAssetClass :one
SELECT id, user_id, ticker, asset_class, currency, name, created_at, updated_at
FROM instruments
WHERE LOWER(ticker) = LOWER($1) AND asset_class = $2 AND user_id = $3;

-- name: GetInstrumentByID :one
SELECT id, user_id, ticker, asset_class, currency, name, created_at, updated_at
FROM instruments
WHERE id = $1;

-- name: SearchInstruments :many
SELECT id, user_id, ticker, asset_class, currency, name, created_at, updated_at
FROM instruments
WHERE (ticker ILIKE '%' || $1 || '%' OR name ILIKE '%' || $1 || '%')
  AND (user_id IS NULL OR user_id = $2)
ORDER BY (LOWER(ticker) = LOWER($1)) DESC, ticker
LIMIT 20;

-- name: ListInstruments :many
SELECT
    i.id, i.user_id, i.ticker, i.asset_class, i.currency, i.name, i.created_at, i.updated_at,
    pr.price        AS current_price,
    pr.fetched_at   AS price_fetched_at
FROM instruments i
LEFT JOIN prices pr ON pr.instrument_id = i.id
WHERE
    (@q::text = '' OR i.ticker ILIKE '%' || @q || '%' OR i.name ILIKE '%' || @q || '%')
    AND (@asset_class::text = '' OR i.asset_class = @asset_class)
    AND (
        @scope::text = 'mine'   AND i.user_id = @caller_id
     OR @scope::text = 'global' AND i.user_id IS NULL
     OR @scope::text = ''       AND (i.user_id IS NULL OR i.user_id = @caller_id)
    )
ORDER BY i.ticker
LIMIT @lim OFFSET @off;

-- name: CountInstruments :one
SELECT COUNT(*)::bigint
FROM instruments
WHERE
    (@q::text = '' OR ticker ILIKE '%' || @q || '%' OR name ILIKE '%' || @q || '%')
    AND (@asset_class::text = '' OR asset_class = @asset_class)
    AND (
        @scope::text = 'mine'   AND user_id = @caller_id
     OR @scope::text = 'global' AND user_id IS NULL
     OR @scope::text = ''       AND (user_id IS NULL OR user_id = @caller_id)
    );

-- name: UpdateInstrumentMeta :one
-- Personal-only rename. Caller must own the row; non-matching rows return ErrNoRows.
UPDATE instruments
SET ticker = COALESCE(sqlc.narg('ticker')::text, ticker),
    name   = COALESCE(sqlc.narg('name')::text, name),
    updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING id, user_id, ticker, asset_class, currency, name, created_at, updated_at;

-- name: DeletePersonalInstrument :execrows
-- Personal-only delete. FK positions.instrument_id ON DELETE RESTRICT raises 23503
-- which the service maps to ErrHasPositions → 409.
DELETE FROM instruments
WHERE id = $1 AND user_id = $2;
