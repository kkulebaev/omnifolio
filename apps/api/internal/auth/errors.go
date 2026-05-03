package auth

import "errors"

var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrSessionInvalid     = errors.New("auth: session invalid")
	ErrSessionExpired     = errors.New("auth: session expired")
	ErrSessionIdle        = errors.New("auth: session idle")
	ErrUnauthenticated    = errors.New("auth: unauthenticated")
)
