-- name: CreateUser :one
INSERT INTO users (id, email, password_hash, display_currency)
VALUES ($1, $2, $3, $4)
RETURNING id, email, password_hash, display_currency, created_at, updated_at;

-- name: GetUserByID :one
SELECT id, email, password_hash, display_currency, created_at, updated_at
FROM users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, display_currency, created_at, updated_at
FROM users
WHERE email = $1;

-- name: CountUsers :one
SELECT COUNT(*) FROM users;

-- name: ListUserIDs :many
SELECT id FROM users ORDER BY created_at;
