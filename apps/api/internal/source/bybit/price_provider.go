package bybit

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/source"
)

// PriceProvider implements source.PriceProvider for instruments with
// instrument_external_ids.source = 'bybit'. Uses public spot tickers
// (no auth needed). Stablecoins get a fixed price=1.
type PriceProvider struct {
	client *Client
}

func NewPriceProvider(client *Client) *PriceProvider {
	return &PriceProvider{client: client}
}

func (p *PriceProvider) GetPrices(ctx context.Context, _ []byte, instruments []source.ResolvedInstrument) (map[uuid.UUID]source.Price, error) {
	out := make(map[uuid.UUID]source.Price)
	if len(instruments) == 0 {
		return out, nil
	}

	now := time.Now()
	for _, inst := range instruments {
		coin := strings.ToUpper(inst.NativeInstrumentID)
		if coin == "" || isStablecoin(coin) {
			continue
		}
		symbol := coin + "USDT"
		var resp tickersResponse
		params := url.Values{}
		params.Set("category", "spot")
		params.Set("symbol", symbol)
		if err := p.client.publicGet(ctx, "/v5/market/tickers", params, &resp); err != nil {
			continue
		}
		if len(resp.Result.List) == 0 {
			continue
		}
		t := resp.Result.List[0]
		price, err := decimal.NewFromString(t.LastPrice)
		if err != nil || price.IsZero() {
			continue
		}
		out[inst.InstrumentID] = source.Price{
			Amount:    price,
			Currency:  "USDT",
			FetchedAt: now,
		}
	}
	return out, nil
}
