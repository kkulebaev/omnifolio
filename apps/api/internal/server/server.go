package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	oapimw "github.com/oapi-codegen/nethttp-middleware"

	"github.com/kkulebaev/omnifolio/api/internal/account"
	"github.com/kkulebaev/omnifolio/api/internal/auth"
	"github.com/kkulebaev/omnifolio/api/internal/instrument"
	"github.com/kkulebaev/omnifolio/api/internal/portfolio"
	"github.com/kkulebaev/omnifolio/api/internal/position"
	"github.com/kkulebaev/omnifolio/api/internal/server/oapi"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
	"github.com/kkulebaev/omnifolio/api/internal/syncer"
)

type Deps struct {
	Auth        *auth.Service
	Account     *account.Service
	Instrument  *instrument.Service
	Position    *position.Service
	Portfolio   *portfolio.Service
	Syncer      *syncer.Service
	Queries     *storage.Queries
	AdminAPIKey string
	Logger      *slog.Logger
	Secure      bool
	MaxAge      int
}

func New(d Deps) (http.Handler, error) {
	spec, err := oapi.GetSwagger()
	if err != nil {
		return nil, err
	}
	spec.Servers = openapi3.Servers{}

	srv := &serverImpl{deps: d}
	strictHandler := oapi.NewStrictHandlerWithOptions(srv, nil, oapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  requestErrorHandler,
		ResponseErrorHandlerFunc: notImplementedHandler,
	})

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(d.Logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// /admin/* is service-to-service: bearer-token auth, no OpenAPI validation.
	admin := &adminHandlers{deps: d}
	r.Route("/admin", func(rt chi.Router) {
		rt.Use(auth.RequireAdmin(d.AdminAPIKey))
		rt.Get("/instruments", admin.listInstruments)
		rt.Post("/instruments", admin.seedInstruments)
		rt.Post("/prices", admin.upsertPrices)
	})

	// User-facing API: OpenAPI validation + session middleware.
	r.Group(func(api chi.Router) {
		api.Use(oapimw.OapiRequestValidatorWithOptions(spec, &oapimw.Options{
			Options: openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc},
		}))
		api.Use(auth.Middleware(d.Auth))
		oapi.HandlerFromMux(strictHandler, api)
	})

	return r, nil
}

func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}
