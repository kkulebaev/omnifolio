package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/kkulebaev/omnifolio/api/internal/auth"
)

// problem matches the OpenAPI Problem schema (RFC 7807).
type problem struct {
	Type     string            `json:"type,omitempty"`
	Title    string            `json:"title"`
	Status   int               `json:"status"`
	Detail   string            `json:"detail,omitempty"`
	Instance string            `json:"instance,omitempty"`
	Fields   map[string]string `json:"fields,omitempty"`
}

func writeProblem(w http.ResponseWriter, p problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// errorToProblem maps a domain error to an HTTP status + problem payload.
func errorToProblem(log *slog.Logger, err error) problem {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		return problem{Type: "/errors/unauthorized", Title: "Invalid credentials", Status: 401}
	case errors.Is(err, auth.ErrUnauthenticated),
		errors.Is(err, auth.ErrSessionInvalid),
		errors.Is(err, auth.ErrSessionExpired):
		return problem{Type: "/errors/unauthorized", Title: "Unauthenticated", Status: 401}
	default:
		log.Error("internal error", "err", err)
		return problem{Type: "/errors/internal", Title: "Internal server error", Status: 500}
	}
}
