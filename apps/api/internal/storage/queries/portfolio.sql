-- name: GetUserPortfolioRows :many
SELECT
    a.id          AS account_id,
    a.name        AS account_name,
    i.id          AS instrument_id,
    i.ticker      AS instrument_ticker,
    i.asset_class AS instrument_asset_class,
    i.currency    AS instrument_currency,
    i.name        AS instrument_name,
    p.quantity,
    pr.price,
    pr.fetched_at
FROM positions p
JOIN accounts a    ON p.account_id = a.id
JOIN instruments i ON p.instrument_id = i.id
LEFT JOIN prices pr ON pr.instrument_id = i.id
WHERE a.user_id = $1
ORDER BY a.created_at, i.ticker;
