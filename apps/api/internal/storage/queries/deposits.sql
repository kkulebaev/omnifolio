-- name: CreateDeposit :one
INSERT INTO deposits (id, user_id, date, kind, currency, amount)
VALUES (@id, @user_id, @date, @kind, @currency, @amount)
RETURNING id, user_id, date, amount, created_at, updated_at, kind, currency;

-- name: ListDepositsByUser :many
SELECT id, user_id, date, amount, created_at, updated_at, kind, currency
FROM deposits
WHERE user_id = $1
ORDER BY date DESC, created_at DESC;

-- name: UpdateDeposit :one
UPDATE deposits
SET date = @date, kind = @kind, currency = @currency, amount = @amount
WHERE id = @id AND user_id = @user_id
RETURNING id, user_id, date, amount, created_at, updated_at, kind, currency;

-- name: DeleteDeposit :execrows
DELETE FROM deposits
WHERE id = $1 AND user_id = $2;
