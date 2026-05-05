package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kkulebaev/omnifolio/api/internal/storage"
)

// Dummy hash used to perform a constant-time argon2 comparison when a user is not found.
// Generated with the same params we use for new hashes; password "x".
const dummyHash = "$argon2id$v=19$m=65536,t=1,p=4$YWFhYWFhYWFhYWFhYWFhYQ$LOwsuvAAQXGNvFuvJsLQF1qybEQpGSiTWv4tWrpbxvc"

var argonParams = &argon2id.Params{
	Memory:      64 * 1024,
	Iterations:  1,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

type User struct {
	ID              uuid.UUID
	Email           string
	DisplayCurrency string
	CreatedAt       time.Time
}

type Service struct {
	q                *storage.Queries
	idleTimeout      time.Duration
	absoluteTimeout  time.Duration
	now              func() time.Time
}

func NewService(q *storage.Queries, idle, absolute time.Duration) *Service {
	return &Service{
		q:               q,
		idleTimeout:     idle,
		absoluteTimeout: absolute,
		now:             time.Now,
	}
}

// Login verifies credentials and creates a new session. Returns the cookie token
// (plaintext) and the user. On failure returns ErrInvalidCredentials.
//
// rememberMe controls session lifetime: when true, expires_at is set to the
// absolute timeout and is not extended on activity (long-lived persistent
// session). When false, expires_at uses the idle timeout and slides forward
// on each request.
func (s *Service) Login(ctx context.Context, email, password string, rememberMe bool) (token string, user User, err error) {
	email = strings.ToLower(strings.TrimSpace(email))

	row, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// constant-time: compare against dummy hash to equalize timing
			_, _ = argon2id.ComparePasswordAndHash(password, dummyHash)
			return "", User{}, ErrInvalidCredentials
		}
		return "", User{}, fmt.Errorf("get user: %w", err)
	}

	match, err := argon2id.ComparePasswordAndHash(password, row.PasswordHash)
	if err != nil || !match {
		return "", User{}, ErrInvalidCredentials
	}

	tok, hash, err := generateSessionToken()
	if err != nil {
		return "", User{}, fmt.Errorf("generate session: %w", err)
	}

	lifetime := s.idleTimeout
	if rememberMe {
		lifetime = s.absoluteTimeout
	}

	if err := s.q.CreateSession(ctx, storage.CreateSessionParams{
		TokenHash: hash,
		UserID:    row.ID,
		ExpiresAt: pgTimestamp(s.now().Add(lifetime)),
	}); err != nil {
		return "", User{}, fmt.Errorf("create session: %w", err)
	}

	return tok, toUser(row), nil
}

// Logout deletes the session identified by the cookie token. Missing/invalid tokens are silently ignored.
func (s *Service) Logout(ctx context.Context, cookieToken string) error {
	if cookieToken == "" {
		return nil
	}
	hash, err := hashCookieToken(cookieToken)
	if err != nil {
		return nil
	}
	return s.q.DeleteSession(ctx, hash)
}

// Resolve looks up the session identified by cookieToken, validates expires_at,
// slides expires_at forward via TouchSession (capped at the current value, so
// remember-me sessions are never shortened), and returns the associated user.
func (s *Service) Resolve(ctx context.Context, cookieToken string) (User, error) {
	if cookieToken == "" {
		return User{}, ErrUnauthenticated
	}
	hash, err := hashCookieToken(cookieToken)
	if err != nil {
		return User{}, ErrSessionInvalid
	}

	sess, err := s.q.GetSession(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrSessionInvalid
		}
		return User{}, fmt.Errorf("get session: %w", err)
	}

	now := s.now()
	if !sess.ExpiresAt.Time.After(now) {
		_ = s.q.DeleteSession(ctx, hash)
		return User{}, ErrSessionExpired
	}

	if now.Sub(sess.LastSeenAt.Time) > time.Minute {
		_ = s.q.TouchSession(ctx, storage.TouchSessionParams{
			TokenHash:    hash,
			MinExpiresAt: pgTimestamp(now.Add(s.idleTimeout)),
		})
	}

	row, err := s.q.GetUserByID(ctx, sess.UserID)
	if err != nil {
		return User{}, fmt.Errorf("get user: %w", err)
	}
	return toUser(row), nil
}

// Bootstrap ensures that if no user exists and BOOTSTRAP_USER_* env vars are set,
// a first user is created. Idempotent (no-op if user already exists).
type BootstrapInput struct {
	Email    string
	Password string
}

func (s *Service) Bootstrap(ctx context.Context, in BootstrapInput) (created bool, err error) {
	if in.Email == "" || in.Password == "" {
		return false, nil
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))

	_, err = s.q.GetUserByEmail(ctx, email)
	if err == nil {
		return false, nil // exists, no-op
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("lookup: %w", err)
	}

	hash, err := argon2id.CreateHash(in.Password, argonParams)
	if err != nil {
		return false, fmt.Errorf("hash password: %w", err)
	}

	_, err = s.q.CreateUser(ctx, storage.CreateUserParams{
		ID:              uuid.Must(uuid.NewV7()),
		Email:           email,
		PasswordHash:    hash,
		DisplayCurrency: "RUB",
	})
	if err != nil {
		return false, fmt.Errorf("create user: %w", err)
	}
	return true, nil
}

// CleanupSessions deletes hard-expired sessions; intended to run as a cron.
func (s *Service) CleanupSessions(ctx context.Context) (int64, error) {
	return s.q.DeleteExpiredSessions(ctx)
}

func toUser(row storage.User) User {
	return User{
		ID:              row.ID,
		Email:           row.Email,
		DisplayCurrency: row.DisplayCurrency,
		CreatedAt:       row.CreatedAt.Time,
	}
}

func pgTimestamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
