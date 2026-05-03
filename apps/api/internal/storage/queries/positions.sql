-- name: CreatePosition :one
INSERT INTO positions (account_id, instrument_id, quantity)
VALUES ($1, $2, $3)
RETURNING account_id, instrument_id, quantity, updated_at;

-- name: UpdatePositionQuantity :one
UPDATE positions
SET quantity = $3, updated_at = now()
WHERE account_id = $1 AND instrument_id = $2
RETURNING account_id, instrument_id, quantity, updated_at;

-- name: DeletePosition :execrows
DELETE FROM positions
WHERE account_id = $1 AND instrument_id = $2;

-- name: ListAccountPositionsWithInstrument :many
SELECT
    p.account_id,
    p.instrument_id,
    p.quantity,
    p.updated_at,
    i.id          AS i_id,
    i.ticker      AS i_ticker,
    i.asset_class AS i_asset_class,
    i.currency    AS i_currency,
    i.name        AS i_name,
    i.created_at  AS i_created_at,
    i.updated_at  AS i_updated_at
FROM positions p
JOIN instruments i ON p.instrument_id = i.id
WHERE p.account_id = $1
ORDER BY i.ticker;
