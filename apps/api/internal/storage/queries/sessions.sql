-- name: CreateSession :exec
INSERT INTO sessions (token_hash, user_id, expires_at, last_seen_at)
VALUES ($1, $2, $3, now());

-- name: GetSession :one
SELECT token_hash, user_id, created_at, expires_at, last_seen_at
FROM sessions
WHERE token_hash = $1;

-- name: TouchSession :exec
UPDATE sessions
SET last_seen_at = now(),
    expires_at = GREATEST(expires_at, sqlc.arg(min_expires_at)::timestamptz)
WHERE token_hash = $1;

-- name: DeleteSession :exec
DELETE FROM sessions
WHERE token_hash = $1;

-- name: DeleteSessionsByUser :exec
DELETE FROM sessions
WHERE user_id = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions
WHERE expires_at < now() - interval '1 day';
