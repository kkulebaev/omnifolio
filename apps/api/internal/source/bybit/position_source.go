package bybit

import (
	"context"
	"net/url"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/source"
)

// SyntheticSubAccountID is what we return from ListSubAccounts to keep the
// onboarding flow uniform. Bybit users typically have one master account; we
// surface a single "main" sub-account so the UI doesn't need a special case.
const SyntheticSubAccountID = "main"

// PositionSource implements source.PositionSource for Bybit.
type PositionSource struct {
	client *Client
}

func NewPositionSource(client *Client) *PositionSource {
	return &PositionSource{client: client}
}

// ListSubAccounts validates credentials and returns a single synthetic sub-account.
// (Bybit master/sub model is not surfaced; multi-sub users add separate Omnifolio
// accounts each with their own key.)
func (p *PositionSource) ListSubAccounts(ctx context.Context, creds []byte) ([]source.SubAccount, error) {
	c, err := UnmarshalCredentials(creds)
	if err != nil {
		// Allow lenient parsing for preview (just verify keys non-empty).
		if c.APIKey == "" || c.APISecret == "" {
			return nil, err
		}
	}
	// Probe: a small signed call to validate keys before persisting.
	var resp walletBalanceResponse
	params := url.Values{}
	params.Set("accountType", "UNIFIED")
	if err := p.client.signedGet(ctx, c, "/v5/account/wallet-balance", params, &resp); err != nil {
		return nil, err
	}
	return []source.SubAccount{{ID: SyntheticSubAccountID, Name: "Bybit", Type: "UNIFIED"}}, nil
}

// Sync fetches UNIFIED wallet balances. Each non-zero coin becomes a position.
func (p *PositionSource) Sync(ctx context.Context, creds []byte, _ string) ([]source.Position, error) {
	c, err := UnmarshalCredentials(creds)
	if err != nil {
		return nil, err
	}
	var resp walletBalanceResponse
	params := url.Values{}
	params.Set("accountType", "UNIFIED")
	if err := p.client.signedGet(ctx, c, "/v5/account/wallet-balance", params, &resp); err != nil {
		return nil, err
	}

	out := make([]source.Position, 0, 16)
	for _, acc := range resp.Result.List {
		for _, coin := range acc.Coin {
			qtyStr := coin.WalletBalance
			if qtyStr == "" {
				qtyStr = coin.Equity
			}
			qty, err := decimal.NewFromString(qtyStr)
			if err != nil || qty.IsZero() {
				continue
			}
			out = append(out, source.Position{
				NativeInstrumentID: strings.ToUpper(coin.Coin),
				Quantity:           qty,
			})
		}
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
	case "USDT", "USDC", "BUSD", "TUSD", "DAI", "USD":
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
