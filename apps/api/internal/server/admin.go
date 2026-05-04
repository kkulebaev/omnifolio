package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/instrument"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
)

type adminHandlers struct {
	deps Deps
}

type adminInstrument struct {
	ID         uuid.UUID `json:"id"`
	Ticker     string    `json:"ticker"`
	AssetClass string    `json:"assetClass"`
	Currency   string    `json:"currency"`
	Name       string    `json:"name"`
}

type adminInstrumentsResponse struct {
	Items []adminInstrument `json:"items"`
}

// listInstruments returns every instrument in a single page so the cron service
// can decide which prices it can refresh. No pagination — the canonical catalog
// is small (≪10k rows expected).
func (a *adminHandlers) listInstruments(w http.ResponseWriter, r *http.Request) {
	res, err := a.deps.Instrument.List(r.Context(), instrument.ListInput{Limit: 10000})
	if err != nil {
		a.deps.Logger.Error("admin: list instruments", "err", err)
		writeAdminError(w, http.StatusInternalServerError, "list failed")
		return
	}
	out := adminInstrumentsResponse{Items: make([]adminInstrument, len(res.Items))}
	for i, x := range res.Items {
		out.Items[i] = adminInstrument{
			ID:         x.ID,
			Ticker:     x.Ticker,
			AssetClass: x.AssetClass,
			Currency:   x.Currency,
			Name:       x.Name,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type adminSeedInstrument struct {
	Ticker     string `json:"ticker"`
	Name       string `json:"name"`
	Currency   string `json:"currency"`
	AssetClass string `json:"assetClass"`
}

type adminSeedInstrumentsRequest struct {
	Items []adminSeedInstrument `json:"items"`
}

type adminSeedInstrumentsResponse struct {
	Processed int      `json:"processed"`
	Failed    int      `json:"failed"`
	Errors    []string `json:"errors,omitempty"`
}

// seedInstruments idempotently registers a batch of canonical instruments.
// Used by the cron service to keep the catalog in sync with its embedded
// instruments.json. Each item goes through CreateOrGet — pre-existing rows
// (matched by LOWER(ticker) + asset_class) are no-ops.
func (a *adminHandlers) seedInstruments(w http.ResponseWriter, r *http.Request) {
	var req adminSeedInstrumentsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAdminError(w, http.StatusBadRequest, fmt.Sprintf("decode: %v", err))
		return
	}

	resp := adminSeedInstrumentsResponse{}
	ctx := r.Context()
	for _, item := range req.Items {
		if _, err := a.deps.Instrument.CreateOrGet(ctx, instrument.CreateInput{
			Ticker:     item.Ticker,
			AssetClass: item.AssetClass,
			Currency:   item.Currency,
			Name:       item.Name,
		}); err != nil {
			resp.Failed++
			resp.Errors = append(resp.Errors, fmt.Sprintf("%s: %v", item.Ticker, err))
			continue
		}
		resp.Processed++
	}
	a.deps.Logger.Info("admin: instruments seeded",
		"processed", resp.Processed, "failed", resp.Failed)
	writeJSON(w, http.StatusOK, resp)
}

type adminPriceItem struct {
	InstrumentID uuid.UUID `json:"instrumentId"`
	Price        string    `json:"price"`
}

type adminUpsertPricesRequest struct {
	Prices []adminPriceItem `json:"prices"`
}

type adminUpsertPricesResponse struct {
	Updated  int      `json:"updated"`
	Failed   int      `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
	Duration string   `json:"duration"`
}

// upsertPrices writes a batch of (instrument_id, price) tuples to the prices
// table. Each row is independently upserted; per-row failures are counted but
// don't abort the batch.
func (a *adminHandlers) upsertPrices(w http.ResponseWriter, r *http.Request) {
	var req adminUpsertPricesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAdminError(w, http.StatusBadRequest, fmt.Sprintf("decode: %v", err))
		return
	}
	if len(req.Prices) == 0 {
		writeJSON(w, http.StatusOK, adminUpsertPricesResponse{Duration: "0s"})
		return
	}

	start := time.Now()
	resp := adminUpsertPricesResponse{}
	ctx := r.Context()
	for _, item := range req.Prices {
		price, err := decimal.NewFromString(item.Price)
		if err != nil || price.Sign() < 0 {
			resp.Failed++
			resp.Errors = append(resp.Errors, fmt.Sprintf("%s: invalid price %q", item.InstrumentID, item.Price))
			continue
		}
		if err := a.deps.Queries.UpsertPrice(ctx, storage.UpsertPriceParams{
			InstrumentID: item.InstrumentID,
			Price:        price,
		}); err != nil {
			resp.Failed++
			resp.Errors = append(resp.Errors, fmt.Sprintf("%s: %v", item.InstrumentID, err))
			continue
		}
		resp.Updated++
	}
	resp.Duration = time.Since(start).String()
	a.deps.Logger.Info("admin: prices upserted",
		"updated", resp.Updated, "failed", resp.Failed, "duration_ms", time.Since(start).Milliseconds())
	writeJSON(w, http.StatusOK, resp)
}

type adminFXRateItem struct {
	Date    string `json:"date"` // YYYY-MM-DD
	FromCcy string `json:"fromCcy"`
	ToCcy   string `json:"toCcy"`
	Rate    string `json:"rate"`
}

type adminUpsertFXRequest struct {
	Rates []adminFXRateItem `json:"rates"`
}

type adminUpsertFXResponse struct {
	Updated  int      `json:"updated"`
	Failed   int      `json:"failed"`
	Errors   []string `json:"errors,omitempty"`
	Duration string   `json:"duration"`
}

// upsertFXRates writes a batch of (date, from_ccy, to_ccy, rate) tuples to the
// fx_rates table. Each row is independently upserted; per-row failures are
// counted but don't abort the batch.
func (a *adminHandlers) upsertFXRates(w http.ResponseWriter, r *http.Request) {
	var req adminUpsertFXRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAdminError(w, http.StatusBadRequest, fmt.Sprintf("decode: %v", err))
		return
	}
	if len(req.Rates) == 0 {
		writeJSON(w, http.StatusOK, adminUpsertFXResponse{Duration: "0s"})
		return
	}

	start := time.Now()
	resp := adminUpsertFXResponse{}
	ctx := r.Context()
	for _, item := range req.Rates {
		date, err := time.Parse("2006-01-02", item.Date)
		if err != nil {
			resp.Failed++
			resp.Errors = append(resp.Errors, fmt.Sprintf("%s/%s: invalid date %q", item.FromCcy, item.ToCcy, item.Date))
			continue
		}
		rate, err := decimal.NewFromString(item.Rate)
		if err != nil || rate.Sign() < 0 {
			resp.Failed++
			resp.Errors = append(resp.Errors, fmt.Sprintf("%s/%s: invalid rate %q", item.FromCcy, item.ToCcy, item.Rate))
			continue
		}
		if err := a.deps.Queries.UpsertFxRate(ctx, storage.UpsertFxRateParams{
			Date:    pgtype.Date{Time: date, Valid: true},
			FromCcy: strings.ToUpper(item.FromCcy),
			ToCcy:   strings.ToUpper(item.ToCcy),
			Rate:    rate,
		}); err != nil {
			resp.Failed++
			resp.Errors = append(resp.Errors, fmt.Sprintf("%s/%s: %v", item.FromCcy, item.ToCcy, err))
			continue
		}
		resp.Updated++
	}
	resp.Duration = time.Since(start).String()
	a.deps.Logger.Info("admin: fx rates upserted",
		"updated", resp.Updated, "failed", resp.Failed, "duration_ms", time.Since(start).Milliseconds())
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeAdminError(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"title":  http.StatusText(status),
		"status": status,
		"detail": detail,
	})
}
