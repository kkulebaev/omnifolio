-- name: CreateAccount :one
INSERT INTO accounts (id, user_id, source_type, name)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, source_type, name, last_synced_at, last_sync_status, last_sync_error, created_at, updated_at;

-- name: GetAccountByUser :one
SELECT id, user_id, source_type, name, last_synced_at, last_sync_status, last_sync_error, created_at, updated_at
FROM accounts
WHERE id = $1 AND user_id = $2;

-- name: ListAccountsByUser :many
SELECT id, user_id, source_type, name, last_synced_at, last_sync_status, last_sync_error, created_at, updated_at
FROM accounts
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: UpdateAccountName :one
UPDATE accounts
SET name = $3
WHERE id = $1 AND user_id = $2
RETURNING id, user_id, source_type, name, last_synced_at, last_sync_status, last_sync_error, created_at, updated_at;

-- name: DeleteAccount :execrows
DELETE FROM accounts
WHERE id = $1 AND user_id = $2;
