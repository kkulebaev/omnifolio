package tinvest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/kkulebaev/omnifolio/api/internal/source"
)

// PositionSource implements source.PositionSource for T-Invest.
type PositionSource struct {
	client *Client
}

func NewPositionSource(client *Client) *PositionSource {
	return &PositionSource{client: client}
}

// ListSubAccounts returns active accounts (брокерский / ИИС / премиум) for the token.
func (p *PositionSource) ListSubAccounts(ctx context.Context, creds []byte) ([]source.SubAccount, error) {
	c, err := UnmarshalCredentials(creds)
	if err != nil {
		// preview flow may pass credentials with empty TInvestAccountID; re-parse leniently
		var tmp Credentials
		if err2 := unmarshalLenient(creds, &tmp); err2 != nil {
			return nil, fmt.Errorf("creds: %w", err)
		}
		c = tmp
	}

	var resp getAccountsResponse
	if err := p.client.call(ctx, c.Token, "UsersService", "GetAccounts",
		getAccountsRequest{Status: statusOpen}, &resp); err != nil {
		return nil, err
	}

	out := make([]source.SubAccount, 0, len(resp.Accounts))
	for _, a := range resp.Accounts {
		if a.AccessLevel == "ACCOUNT_ACCESS_LEVEL_NO_ACCESS" {
			continue
		}
		name := a.Name
		if name == "" {
			name = subAccountType(a.Type)
		}
		out = append(out, source.SubAccount{
			ID:   a.ID,
			Name: name,
			Type: subAccountType(a.Type),
		})
	}
	return out, nil
}

// Sync fetches the current positions for the configured sub-account.
func (p *PositionSource) Sync(ctx context.Context, creds []byte, subAccountID string) ([]source.Position, error) {
	c, err := UnmarshalCredentials(creds)
	if err != nil {
		return nil, err
	}
	if subAccountID == "" {
		subAccountID = c.TInvestAccountID
	}
	if subAccountID == "" {
		return nil, source.ErrSubAccountNotFound
	}

	var resp getPortfolioResponse
	if err := p.client.call(ctx, c.Token, "OperationsService", "GetPortfolio",
		getPortfolioRequest{AccountID: subAccountID}, &resp); err != nil {
		return nil, err
	}

	out := make([]source.Position, 0, len(resp.Positions))
	for _, ap := range resp.Positions {
		qty := ap.Quantity.ToDecimal()
		if qty.IsZero() {
			continue
		}
		if AssetClassFor(ap.InstrumentType, "") == "" && ap.InstrumentType != "share" && ap.InstrumentType != "etf" {
			// futures / options / sp — skip. Shares/ETFs need class_code from Resolve to classify
			// but we don't drop them here; resolution will set class properly.
			if ap.InstrumentType == "futures" || ap.InstrumentType == "option" || ap.InstrumentType == "sp" {
				continue
			}
		}
		out = append(out, source.Position{
			NativeInstrumentID: ap.FIGI,
			Quantity:           qty,
		})
	}
	return out, nil
}

// ResolveInstrument fetches metadata for an unfamiliar FIGI and returns a seed
// our account service can use to upsert into the canonical instruments table.
func (p *PositionSource) ResolveInstrument(ctx context.Context, creds []byte, figi string) (source.InstrumentSeed, error) {
	c, err := UnmarshalCredentials(creds)
	if err != nil {
		return source.InstrumentSeed{}, err
	}

	var resp getInstrumentByResponse
	if err := p.client.call(ctx, c.Token, "InstrumentsService", "GetInstrumentBy",
		getInstrumentByRequest{IDType: idTypeFIGI, ID: figi}, &resp); err != nil {
		if errors.Is(err, ErrNotFound) {
			return source.InstrumentSeed{}, source.ErrInstrumentUnknown
		}
		return source.InstrumentSeed{}, err
	}

	inst := resp.Instrument
	assetClass := AssetClassFor(inst.InstrumentType, inst.ClassCode)
	if assetClass == "" {
		return source.InstrumentSeed{}, fmt.Errorf("%w: instrument type %q not supported", source.ErrInstrumentUnknown, inst.InstrumentType)
	}

	currency := inst.Currency
	if currency != "" {
		currency = strNormalizeCurrency(currency)
	}

	seed := source.InstrumentSeed{
		Ticker:     inst.Ticker,
		AssetClass: assetClass,
		Currency:   currency,
		Name:       inst.Name,
	}
	if assetClass == "cash" {
		// Currency instruments use ticker like "RUB000UTSTOM" — we want "RUB" / "USD" etc.
		seed.Ticker = strNormalizeCurrency(currency)
		if seed.Ticker == "" {
			seed.Ticker = inst.Ticker
		}
		seed.Currency = seed.Ticker
	}
	return seed, nil
}

// strNormalizeCurrency uppercases a currency code (Tinkoff returns "rub", "usd").
func strNormalizeCurrency(c string) string {
	out := make([]byte, len(c))
	for i := 0; i < len(c); i++ {
		ch := c[i]
		if ch >= 'a' && ch <= 'z' {
			ch -= 32
		}
		out[i] = ch
	}
	return string(out)
}

// unmarshalLenient parses credentials without enforcing TInvestAccountID — used
// during preview flow when sub-account hasn't been picked yet.
func unmarshalLenient(b []byte, c *Credentials) error {
	type alias struct {
		Token            string `json:"token"`
		TInvestAccountID string `json:"tinvestAccountId"`
	}
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	if a.Token == "" {
		return errors.New("tinvest creds: empty token")
	}
	c.Token = a.Token
	c.TInvestAccountID = a.TInvestAccountID
	return nil
}
