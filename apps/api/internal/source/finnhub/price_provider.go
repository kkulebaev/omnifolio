package finnhub

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/source"
)

// concurrency caps in-flight quote requests so we don't burst past Finnhub's
// 60/min free-tier limit on portfolios with many tickers.
const concurrency = 8

// PriceProvider implements source.PriceProvider for US stocks/ETFs via Finnhub.
// It's a price-only provider: creds are unused, NativeInstrumentID must hold
// the canonical ticker (e.g. "AAPL").
type PriceProvider struct {
	client *Client
}

func NewPriceProvider(client *Client) *PriceProvider {
	return &PriceProvider{client: client}
}

func (p *PriceProvider) GetPrices(ctx context.Context, _ []byte, instruments []source.ResolvedInstrument) (map[uuid.UUID]source.Price, error) {
	out := make(map[uuid.UUID]source.Price, len(instruments))
	if len(instruments) == 0 {
		return out, nil
	}

	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, concurrency)
	)
	for _, inst := range instruments {
		symbol := strings.ToUpper(strings.TrimSpace(inst.NativeInstrumentID))
		if symbol == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(inst source.ResolvedInstrument, symbol string) {
			defer wg.Done()
			defer func() { <-sem }()

			q, err := p.client.quote(ctx, symbol)
			if err != nil || q.C == 0 {
				return
			}
			fetchedAt := time.Now()
			if q.T > 0 {
				fetchedAt = time.Unix(q.T, 0)
			}
			mu.Lock()
			out[inst.InstrumentID] = source.Price{
				Amount:    decimal.NewFromFloat(q.C),
				Currency:  "USD",
				FetchedAt: fetchedAt,
			}
			mu.Unlock()
		}(inst, symbol)
	}
	wg.Wait()
	return out, nil
}
