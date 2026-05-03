-- name: CreateInstrument :one
INSERT INTO instruments (id, ticker, asset_class, currency, name)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, ticker, asset_class, currency, name, created_at, updated_at;

-- name: GetInstrumentByTickerAssetClass :one
SELECT id, ticker, asset_class, currency, name, created_at, updated_at
FROM instruments
WHERE LOWER(ticker) = LOWER($1) AND asset_class = $2;

-- name: GetInstrumentByID :one
SELECT id, ticker, asset_class, currency, name, created_at, updated_at
FROM instruments
WHERE id = $1;

-- name: SearchInstruments :many
SELECT id, ticker, asset_class, currency, name, created_at, updated_at
FROM instruments
WHERE ticker ILIKE '%' || $1 || '%' OR name ILIKE '%' || $1 || '%'
ORDER BY (LOWER(ticker) = LOWER($1)) DESC, ticker
LIMIT 20;

-- name: ListInstruments :many
SELECT id, ticker, asset_class, currency, name, created_at, updated_at
FROM instruments
WHERE
    (@q::text = '' OR ticker ILIKE '%' || @q || '%' OR name ILIKE '%' || @q || '%')
    AND (@asset_class::text = '' OR asset_class = @asset_class)
ORDER BY ticker
LIMIT @lim OFFSET @off;

-- name: CountInstruments :one
SELECT COUNT(*)::bigint
FROM instruments
WHERE
    (@q::text = '' OR ticker ILIKE '%' || @q || '%' OR name ILIKE '%' || @q || '%')
    AND (@asset_class::text = '' OR asset_class = @asset_class);
