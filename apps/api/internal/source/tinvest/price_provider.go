package tinvest

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/kkulebaev/omnifolio/api/internal/source"
)

// PriceProvider implements source.PriceProvider for instruments with
// instrument_external_ids.source = 'tinvest'.
type PriceProvider struct {
	client *Client
}

func NewPriceProvider(client *Client) *PriceProvider {
	return &PriceProvider{client: client}
}

// GetPrices returns the latest known price for each instrument with a tinvest FIGI.
// Instruments without a tinvest FIGI (NativeInstrumentID empty) are silently skipped.
func (p *PriceProvider) GetPrices(ctx context.Context, creds []byte, instruments []source.ResolvedInstrument) (map[uuid.UUID]source.Price, error) {
	if len(instruments) == 0 {
		return map[uuid.UUID]source.Price{}, nil
	}
	c, err := UnmarshalCredentials(creds)
	if err != nil {
		return nil, err
	}

	figiToID := make(map[string]uuid.UUID, len(instruments))
	figiToCcy := make(map[string]string, len(instruments))
	figis := make([]string, 0, len(instruments))
	for _, inst := range instruments {
		if inst.NativeInstrumentID == "" {
			continue
		}
		figiToID[inst.NativeInstrumentID] = inst.InstrumentID
		figiToCcy[inst.NativeInstrumentID] = inst.Currency
		figis = append(figis, inst.NativeInstrumentID)
	}
	if len(figis) == 0 {
		return map[uuid.UUID]source.Price{}, nil
	}

	var resp getLastPricesResponse
	if err := p.client.call(ctx, c.Token, "MarketDataService", "GetLastPrices",
		getLastPricesRequest{FIGI: figis}, &resp); err != nil {
		return nil, err
	}

	out := make(map[uuid.UUID]source.Price, len(resp.LastPrices))
	for _, lp := range resp.LastPrices {
		id, ok := figiToID[lp.FIGI]
		if !ok {
			continue
		}
		amount := lp.Price.ToDecimal()
		if amount.IsZero() {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, lp.Time)
		if err != nil {
			ts = time.Now()
		}
		out[id] = source.Price{
			Amount:    amount,
			Currency:  figiToCcy[lp.FIGI],
			FetchedAt: ts,
		}
	}
	return out, nil
}

