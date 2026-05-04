package binance

import (
	"context"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/source"
)

// SyntheticSubAccountID keeps the onboarding flow uniform. Binance master/sub
// model is not surfaced; multi-sub users add separate Omnifolio accounts each
// with their own key.
const SyntheticSubAccountID = "main"

// PositionSource implements source.PositionSource for Binance.
type PositionSource struct {
	client *Client
}

func NewPositionSource(client *Client) *PositionSource {
	return &PositionSource{client: client}
}

// ListSubAccounts validates credentials and returns a single synthetic sub-account.
func (p *PositionSource) ListSubAccounts(ctx context.Context, creds []byte) ([]source.SubAccount, error) {
	c, err := UnmarshalCredentials(creds)
	if err != nil {
		if c.APIKey == "" || c.APISecret == "" {
			return nil, err
		}
	}
	var resp accountResponse
	if err := p.client.signedGet(ctx, c, "/api/v3/account", nil, &resp); err != nil {
		return nil, err
	}
	return []source.SubAccount{{ID: SyntheticSubAccountID, Name: "Binance", Type: "SPOT"}}, nil
}

// Sync fetches Spot account balances. Each non-zero asset becomes a position
// (free + locked).
func (p *PositionSource) Sync(ctx context.Context, creds []byte, _ string) ([]source.Position, error) {
	c, err := UnmarshalCredentials(creds)
	if err != nil {
		return nil, err
	}
	var resp accountResponse
	if err := p.client.signedGet(ctx, c, "/api/v3/account", nil, &resp); err != nil {
		return nil, err
	}

	out := make([]source.Position, 0, 16)
	for _, b := range resp.Balances {
		free, _ := decimal.NewFromString(b.Free)
		locked, _ := decimal.NewFromString(b.Locked)
		qty := free.Add(locked)
		if qty.IsZero() {
			continue
		}
		out = append(out, source.Position{
			NativeInstrumentID: strings.ToUpper(b.Asset),
			Quantity:           qty,
		})
	}
	return out, nil
}

// ResolveInstrument turns a coin code (e.g. "BTC") into an InstrumentSeed.
// Stablecoins are mapped to asset_class=cash so they aggregate as cash holdings.
func (p *PositionSource) ResolveInstrument(_ context.Context, _ []byte, coin string) (source.InstrumentSeed, error) {
	coin = strings.ToUpper(coin)
	if isStablecoin(coin) {
		return source.InstrumentSeed{
			Ticker:     coin,
			AssetClass: "cash",
			Currency:   coin,
			Name:       stablecoinName(coin),
		}, nil
	}
	return source.InstrumentSeed{
		Ticker:     coin,
		AssetClass: "crypto",
		Currency:   "USDT",
		Name:       coinDisplayName(coin),
	}, nil
}

func isStablecoin(coin string) bool {
	switch coin {
	case "USDT", "USDC", "BUSD", "TUSD", "DAI", "USD", "FDUSD":
		return true
	}
	return false
}

func stablecoinName(coin string) string {
	switch coin {
	case "USDT":
		return "Tether USD"
	case "USDC":
		return "USD Coin"
	case "BUSD":
		return "Binance USD"
	case "TUSD":
		return "TrueUSD"
	case "DAI":
		return "Dai Stablecoin"
	case "FDUSD":
		return "First Digital USD"
	}
	return coin
}

func coinDisplayName(coin string) string {
	switch coin {
	case "BTC":
		return "Bitcoin"
	case "ETH":
		return "Ethereum"
	case "SOL":
		return "Solana"
	case "TON":
		return "Toncoin"
	case "BNB":
		return "BNB"
	case "XRP":
		return "XRP"
	case "ADA":
		return "Cardano"
	case "DOGE":
		return "Dogecoin"
	}
	return coin
}
