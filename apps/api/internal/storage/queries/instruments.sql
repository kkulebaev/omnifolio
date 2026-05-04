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
SELECT
    i.id, i.ticker, i.asset_class, i.currency, i.name, i.created_at, i.updated_at,
    pr.price        AS current_price,
    pr.fetched_at   AS price_fetched_at
FROM instruments i
LEFT JOIN prices pr ON pr.instrument_id = i.id
WHERE
    (@q::text = '' OR i.ticker ILIKE '%' || @q || '%' OR i.name ILIKE '%' || @q || '%')
    AND (@asset_class::text = '' OR i.asset_class = @asset_class)
ORDER BY i.ticker
LIMIT @lim OFFSET @off;

-- name: CountInstruments :one
SELECT COUNT(*)::bigint
FROM instruments
WHERE
    (@q::text = '' OR ticker ILIKE '%' || @q || '%' OR name ILIKE '%' || @q || '%')
    AND (@asset_class::text = '' OR asset_class = @asset_class);
