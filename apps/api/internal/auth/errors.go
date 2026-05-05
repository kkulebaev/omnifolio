package auth

import "errors"

var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrSessionInvalid     = errors.New("auth: session invalid")
	ErrSessionExpired     = errors.New("auth: session expired")
	ErrUnauthenticated    = errors.New("auth: unauthenticated")
)
