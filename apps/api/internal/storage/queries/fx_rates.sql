-- name: UpsertFxRate :exec
INSERT INTO fx_rates (date, from_ccy, to_ccy, rate)
VALUES ($1, $2, $3, $4)
ON CONFLICT (date, from_ccy, to_ccy) DO UPDATE SET rate = EXCLUDED.rate;

-- name: GetLatestFxRate :one
SELECT date, from_ccy, to_ccy, rate
FROM fx_rates
WHERE from_ccy = $1 AND to_ccy = $2
ORDER BY date DESC
LIMIT 1;

-- Prefers the latest rate on or before @at; falls back to the nearest later
-- one so flows older than the stored FX history still convert.
-- name: GetFxRateAt :one
SELECT date, from_ccy, to_ccy, rate
FROM fx_rates
WHERE from_ccy = @from_ccy AND to_ccy = @to_ccy
ORDER BY (date > @at::date), ABS(date - @at::date)
LIMIT 1;
