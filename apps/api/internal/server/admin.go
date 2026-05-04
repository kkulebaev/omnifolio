package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
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

