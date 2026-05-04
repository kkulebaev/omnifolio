// Package main is the entrypoint for the price-refresh cron service. It runs
// once per invocation: build a seed catalog (static us_* list + dynamic ru_stock
// snapshot from T-Invest) → POST /admin/instruments → GET /admin/instruments →
// fetch quotes from external providers → POST /admin/prices → exit.
// Designed for Railway's cron mode.
package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

//go:embed instruments.json
var seedJSON []byte

const httpTimeout = 15 * time.Second

type config struct {
	APIURL        string
	AdminAPIKey   string
	FinnhubAPIKey string
	TInvestToken  string
}

func loadConfig() (config, error) {
	cfg := config{
		APIURL:        strings.TrimRight(os.Getenv("API_URL"), "/"),
		AdminAPIKey:   os.Getenv("ADMIN_API_KEY"),
		FinnhubAPIKey: os.Getenv("FINNHUB_API_KEY"),
		TInvestToken:  os.Getenv("TINVEST_TOKEN"),
	}
	if cfg.APIURL == "" {
		return cfg, fmt.Errorf("API_URL is required")
	}
	if cfg.AdminAPIKey == "" {
		return cfg, fmt.Errorf("ADMIN_API_KEY is required")
	}
	return cfg, nil
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("run failed", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	log.Info("cron: start", "api_url", cfg.APIURL)

	seeds, err := loadStaticSeed()
	if err != nil {
		return fmt.Errorf("load static seed: %w", err)
	}

	// RU shares are discovered at runtime: ask T-Invest for the MOEX universe,
	// then seed each share. We keep ticker→figi locally for the price step.
	figiByTicker := map[string]string{}
	if cfg.TInvestToken != "" {
		tin := newTinvestClient(cfg.TInvestToken)
		shares, err := tin.fetchMoexShares(ctx)
		if err != nil {
			log.Warn("cron: tinvest shares fetch failed; ru channel skipped", "err", err)
		} else {
			log.Info("cron: tinvest moex shares fetched", "count", len(shares))
			for _, s := range shares {
				ticker := strings.ToUpper(strings.TrimSpace(s.Ticker))
				if ticker == "" {
					continue
				}
				figiByTicker[ticker] = s.FIGI
				seeds = append(seeds, seedItem{
					Ticker:     ticker,
					Name:       s.Name,
					Currency:   strings.ToUpper(s.Currency),
					AssetClass: "ru_stock",
				})
			}
		}
	} else {
		log.Warn("cron: TINVEST_TOKEN not set; skipping ru channel")
	}

	if seedResp, err := seedInstruments(ctx, cfg, seeds); err != nil {
		return fmt.Errorf("seed instruments: %w", err)
	} else {
		log.Info("cron: catalog seeded",
			"processed", seedResp.Processed, "failed", seedResp.Failed)
		for _, e := range seedResp.Errors {
			log.Warn("cron: seed error", "msg", e)
		}
	}

	insts, err := listInstruments(ctx, cfg)
	if err != nil {
		return fmt.Errorf("list instruments: %w", err)
	}
	log.Info("cron: instruments fetched", "count", len(insts))

	var prices []priceItem

	if cfg.FinnhubAPIKey != "" {
		fh := newFinnhubClient(cfg.FinnhubAPIKey)
		eligible := filterByAssetClass(insts, "us_stock", "us_etf")
		log.Info("cron: querying finnhub", "count", len(eligible))
		prices = append(prices, fh.fetchAll(ctx, eligible, log)...)
	} else {
		log.Warn("cron: FINNHUB_API_KEY not set; skipping us_stock/us_etf")
	}

	if cfg.TInvestToken != "" && len(figiByTicker) > 0 {
		tin := newTinvestClient(cfg.TInvestToken)
		eligible := filterByAssetClass(insts, "ru_stock")
		targets := make([]ruPriceTarget, 0, len(eligible))
		for _, i := range eligible {
			figi, ok := figiByTicker[strings.ToUpper(i.Ticker)]
			if !ok {
				continue
			}
			targets = append(targets, ruPriceTarget{InstrumentID: i.ID, FIGI: figi})
		}
		log.Info("cron: querying tinvest", "count", len(targets))
		prices = append(prices, tin.fetchPrices(ctx, targets, log)...)
	}

	if len(prices) == 0 {
		log.Info("cron: nothing to upsert")
		return nil
	}

	resp, err := upsertPrices(ctx, cfg, prices)
	if err != nil {
		return fmt.Errorf("upsert prices: %w", err)
	}
	log.Info("cron: done", "updated", resp.Updated, "failed", resp.Failed, "duration", resp.Duration)
	if resp.Failed > 0 {
		for _, e := range resp.Errors {
			log.Warn("cron: upsert error", "msg", e)
		}
	}
	return nil
}

// ---------- Seed ----------

type seedItem struct {
	Ticker     string `json:"ticker"`
	Name       string `json:"name"`
	Currency   string `json:"currency"`
	AssetClass string `json:"assetClass"`
}

func loadStaticSeed() ([]seedItem, error) {
	var items []seedItem
	if err := json.Unmarshal(seedJSON, &items); err != nil {
		return nil, fmt.Errorf("parse instruments.json: %w", err)
	}
	return items, nil
}

// ---------- Admin API client ----------

type instrumentItem struct {
	ID         uuid.UUID `json:"id"`
	Ticker     string    `json:"ticker"`
	AssetClass string    `json:"assetClass"`
	Currency   string    `json:"currency"`
	Name       string    `json:"name"`
}

type instrumentsResponse struct {
	Items []instrumentItem `json:"items"`
}

type priceItem struct {
	InstrumentID uuid.UUID `json:"instrumentId"`
	Price        string    `json:"price"`
}

type seedResponse struct {
	Processed int      `json:"processed"`
	Failed    int      `json:"failed"`
	Errors    []string `json:"errors,omitempty"`
}

type upsertResponse struct {
	Updated  int      `json:"updated"`
	Failed   int      `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
	Duration string   `json:"duration"`
}

func adminClient() *http.Client {
	return &http.Client{Timeout: httpTimeout}
}

// seedInstruments idempotently registers every entry from the seed list in
// the canonical catalog. CreateOrGet on the api side ensures duplicates are
// no-ops, so it's safe to run on every invocation.
func seedInstruments(ctx context.Context, cfg config, items []seedItem) (seedResponse, error) {
	if len(items) == 0 {
		return seedResponse{}, nil
	}
	body, err := json.Marshal(map[string]any{"items": items})
	if err != nil {
		return seedResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.APIURL+"/admin/instruments", bytes.NewReader(body))
	if err != nil {
		return seedResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AdminAPIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := adminClient().Do(req)
	if err != nil {
		return seedResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return seedResponse{}, fmt.Errorf("admin/instruments POST: HTTP %d", res.StatusCode)
	}
	var out seedResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return seedResponse{}, fmt.Errorf("decode: %w", err)
	}
	return out, nil
}

func listInstruments(ctx context.Context, cfg config) ([]instrumentItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.APIURL+"/admin/instruments", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AdminAPIKey)
	req.Header.Set("Accept", "application/json")
	res, err := adminClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("admin/instruments: HTTP %d", res.StatusCode)
	}
	var out instrumentsResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return out.Items, nil
}

func upsertPrices(ctx context.Context, cfg config, prices []priceItem) (upsertResponse, error) {
	body, err := json.Marshal(map[string]any{"prices": prices})
	if err != nil {
		return upsertResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.APIURL+"/admin/prices", bytes.NewReader(body))
	if err != nil {
		return upsertResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AdminAPIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := adminClient().Do(req)
	if err != nil {
		return upsertResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return upsertResponse{}, fmt.Errorf("admin/prices: HTTP %d", res.StatusCode)
	}
	var out upsertResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return upsertResponse{}, fmt.Errorf("decode: %w", err)
	}
	return out, nil
}

func filterByAssetClass(insts []instrumentItem, classes ...string) []instrumentItem {
	allowed := make(map[string]struct{}, len(classes))
	for _, c := range classes {
		allowed[c] = struct{}{}
	}
	out := make([]instrumentItem, 0, len(insts))
	for _, i := range insts {
		if _, ok := allowed[i.AssetClass]; ok {
			out = append(out, i)
		}
	}
	return out
}
