package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kkulebaev/omnifolio/api/internal/account"
	"github.com/kkulebaev/omnifolio/api/internal/auth"
	"github.com/kkulebaev/omnifolio/api/internal/config"
	"github.com/kkulebaev/omnifolio/api/internal/crypto"
	"github.com/kkulebaev/omnifolio/api/internal/fx"
	"github.com/kkulebaev/omnifolio/api/internal/instrument"
	"github.com/kkulebaev/omnifolio/api/internal/portfolio"
	"github.com/kkulebaev/omnifolio/api/internal/position"
	"github.com/kkulebaev/omnifolio/api/internal/scheduler"
	"github.com/kkulebaev/omnifolio/api/internal/server"
	"github.com/kkulebaev/omnifolio/api/internal/source"
	"github.com/kkulebaev/omnifolio/api/internal/source/bybit"
	"github.com/kkulebaev/omnifolio/api/internal/source/tinvest"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
	"github.com/kkulebaev/omnifolio/api/internal/syncer"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)
	log.Info("starting", "env", cfg.Env, "port", cfg.Port)

	masterKey, err := crypto.ParseMasterKey(cfg.MasterKey)
	if err != nil {
		return fmt.Errorf("master key: %w", err)
	}
	encryptor, err := crypto.NewEncryptor(masterKey)
	if err != nil {
		return fmt.Errorf("encryptor: %w", err)
	}

	idleTimeout, err := time.ParseDuration(cfg.SessionIdleTimeout)
	if err != nil {
		return fmt.Errorf("parse session idle timeout: %w", err)
	}
	absoluteTimeout, err := time.ParseDuration(cfg.SessionAbsoluteTimeout)
	if err != nil {
		return fmt.Errorf("parse session absolute timeout: %w", err)
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := storage.NewPool(rootCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db pool: %w", err)
	}
	defer pool.Close()
	log.Info("db pool ready")

	if err := storage.Migrate(rootCtx, pool); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	log.Info("migrations applied")

	queries := storage.New(pool)

	// Source registry (positions only — prices are refreshed by apps/cron).
	registry := source.NewRegistry()
	registry.Positions[account.TypeTInvest] = tinvest.NewPositionSource(tinvest.NewClient())
	registry.Positions[account.TypeBybit] = bybit.NewPositionSource(bybit.NewClient())

	authSvc := auth.NewService(queries, idleTimeout, absoluteTimeout)
	accountSvc := account.NewService(pool, encryptor, registry)
	instrumentSvc := instrument.NewService(queries)
	positionSvc := position.NewService(queries, accountSvc, instrumentSvc)
	fxSvc := fx.NewService(queries, log)
	portfolioSvc := portfolio.NewService(queries, fxSvc)
	syncerSvc := syncer.NewService(pool, encryptor, registry, log)

	created, err := authSvc.Bootstrap(rootCtx, auth.BootstrapInput{
		Email:    cfg.BootstrapUserEmail,
		Password: cfg.BootstrapUserPassword,
	})
	if err != nil {
		return fmt.Errorf("bootstrap user: %w", err)
	}
	switch {
	case created:
		log.Info("bootstrap: user created", "email", cfg.BootstrapUserEmail)
	case cfg.BootstrapUserEmail == "":
		log.Warn("bootstrap skipped: BOOTSTRAP_USER_EMAIL not set; first user must be created via /auth/register (not yet implemented)")
	default:
		log.Info("bootstrap skipped: user exists", "email", cfg.BootstrapUserEmail)
	}

	sched := scheduler.New(log)
	if err := sched.Register(rootCtx,
		scheduler.Job{
			Name: "sessions-cleanup",
			Spec: "0 * * * *",
			Run: func(ctx context.Context) error {
				n, err := authSvc.CleanupSessions(ctx)
				if err == nil && n > 0 {
					log.Info("sessions cleanup", "deleted", n)
				}
				return err
			},
		},
		scheduler.Job{
			Name: "fx-refresh",
			Spec: "0 6 * * *",
			Run:  fxSvc.Refresh,
		},
		scheduler.Job{
			Name: "sync-brokerage-accounts",
			Spec: "0 * * * *",
			Run:  syncerSvc.SyncAll,
		},
	); err != nil {
		return fmt.Errorf("scheduler: %w", err)
	}
	sched.Start()
	defer sched.Stop()

	// Initial FX seed (best-effort) so /portfolio works on first run.
	go func() {
		if err := fxSvc.Refresh(rootCtx); err != nil {
			log.Warn("fx initial refresh failed", "err", err)
		}
	}()

	handler, err := server.New(server.Deps{
		Auth:        authSvc,
		Account:     accountSvc,
		Instrument:  instrumentSvc,
		Position:    positionSvc,
		Portfolio:   portfolioSvc,
		Syncer:      syncerSvc,
		Queries:     queries,
		AdminAPIKey: cfg.AdminAPIKey,
		Logger:      log,
		Secure:      cfg.IsProduction(),
		MaxAge:      int(absoluteTimeout / time.Second),
	})
	if err != nil {
		return fmt.Errorf("server: %w", err)
	}

	httpSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("listening", "addr", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http listen", "err", err)
			stop()
		}
	}()

	<-rootCtx.Done()
	log.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown", "err", err)
	}
	log.Info("bye")
	return nil
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
