-- name: CreateDeposit :one
INSERT INTO deposits (id, user_id, month, amount)
VALUES (@id, @user_id, date_trunc('month', @month::date)::date, @amount)
RETURNING id, user_id, month, amount, created_at, updated_at;

-- name: ListDepositsByUser :many
SELECT id, user_id, month, amount, created_at, updated_at
FROM deposits
WHERE user_id = $1
ORDER BY month DESC, created_at DESC;

-- name: DeleteDeposit :execrows
DELETE FROM deposits
WHERE id = $1 AND user_id = $2;
