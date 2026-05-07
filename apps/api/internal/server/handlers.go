package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/shopspring/decimal"

	"github.com/kkulebaev/omnifolio/api/internal/account"
	"github.com/kkulebaev/omnifolio/api/internal/auth"
	"github.com/kkulebaev/omnifolio/api/internal/deposits"
	"github.com/kkulebaev/omnifolio/api/internal/instrument"
	"github.com/kkulebaev/omnifolio/api/internal/portfolio"
	"github.com/kkulebaev/omnifolio/api/internal/position"
	"github.com/kkulebaev/omnifolio/api/internal/server/oapi"
	"github.com/kkulebaev/omnifolio/api/internal/source"
	"github.com/kkulebaev/omnifolio/api/internal/storage"
)

type serverImpl struct {
	deps Deps
}

var errNotImplemented = errors.New("not implemented")

// ----- system -----

func (s *serverImpl) GetHealthz(_ context.Context, _ oapi.GetHealthzRequestObject) (oapi.GetHealthzResponseObject, error) {
	return oapi.GetHealthz200JSONResponse{Status: "ok"}, nil
}

// ----- auth -----

func (s *serverImpl) Login(ctx context.Context, req oapi.LoginRequestObject) (oapi.LoginResponseObject, error) {
	if req.Body == nil {
		return validationProblem("missing body", nil).asLoginResponse(), nil
	}

	rememberMe := req.Body.RememberMe != nil && *req.Body.RememberMe
	token, user, err := s.deps.Auth.Login(ctx, string(req.Body.Email), req.Body.Password, rememberMe)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			s.deps.Logger.Warn("auth: login failed", "email", string(req.Body.Email))
			return oapi.Login401ApplicationProblemPlusJSONResponse{
				UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(invalidCredentialsProblem()),
			}, nil
		}
		return nil, err
	}

	s.deps.Logger.Info("auth: login ok", "user_id", user.ID)
	maxAge := 0
	if rememberMe {
		maxAge = s.deps.MaxAge
	}
	cookie := buildSessionCookie(token, maxAge, s.deps.Secure).String()
	return oapi.Login200JSONResponse{
		Body:    toOapiUser(user),
		Headers: oapi.Login200ResponseHeaders{SetCookie: &cookie},
	}, nil
}

func (s *serverImpl) Logout(ctx context.Context, _ oapi.LogoutRequestObject) (oapi.LogoutResponseObject, error) {
	if cookie, ok := auth.CookieFromContext(ctx); ok {
		_ = s.deps.Auth.Logout(ctx, cookie)
	}
	if user, ok := auth.UserFromContext(ctx); ok {
		s.deps.Logger.Info("auth: logout", "user_id", user.ID)
	}
	return logoutWithClearCookie{secure: s.deps.Secure}, nil
}

func (s *serverImpl) GetMe(ctx context.Context, _ oapi.GetMeRequestObject) (oapi.GetMeResponseObject, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return oapi.GetMe401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}
	return oapi.GetMe200JSONResponse(toOapiUser(user)), nil
}

// ----- accounts -----

func (s *serverImpl) ListAccounts(ctx context.Context, _ oapi.ListAccountsRequestObject) (oapi.ListAccountsResponseObject, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return oapi.ListAccounts401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}

	rows, err := s.deps.Account.List(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	items := make([]oapi.Account, len(rows))
	for i, a := range rows {
		items[i] = toOapiAccount(a)
	}
	return oapi.ListAccounts200JSONResponse{Items: items}, nil
}

func (s *serverImpl) CreateAccount(ctx context.Context, req oapi.CreateAccountRequestObject) (oapi.CreateAccountResponseObject, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return oapi.CreateAccount401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}
	if req.Body == nil {
		return createAccountValidationResp("missing body", nil), nil
	}

	in := account.CreateInput{
		Name: req.Body.Name,
		Type: string(req.Body.Type),
	}
	switch string(req.Body.Type) {
	case account.TypeTInvest:
		if req.Body.Token == nil || *req.Body.Token == "" {
			return createAccountValidationResp("Validation failed",
				map[string]string{"token": "required for type=tinvest"}), nil
		}
		if req.Body.TinvestAccountId == nil || *req.Body.TinvestAccountId == "" {
			return createAccountValidationResp("Validation failed",
				map[string]string{"tinvestAccountId": "required for type=tinvest"}), nil
		}
		in.TInvestToken = *req.Body.Token
		in.TInvestAccountID = *req.Body.TinvestAccountId
	case account.TypeBybit:
		if req.Body.ApiKey == nil || *req.Body.ApiKey == "" {
			return createAccountValidationResp("Validation failed",
				map[string]string{"apiKey": "required for type=bybit"}), nil
		}
		if req.Body.ApiSecret == nil || *req.Body.ApiSecret == "" {
			return createAccountValidationResp("Validation failed",
				map[string]string{"apiSecret": "required for type=bybit"}), nil
		}
		in.BybitAPIKey = *req.Body.ApiKey
		in.BybitAPISecret = *req.Body.ApiSecret
	case account.TypeBinance:
		if req.Body.ApiKey == nil || *req.Body.ApiKey == "" {
			return createAccountValidationResp("Validation failed",
				map[string]string{"apiKey": "required for type=binance"}), nil
		}
		if req.Body.ApiSecret == nil || *req.Body.ApiSecret == "" {
			return createAccountValidationResp("Validation failed",
				map[string]string{"apiSecret": "required for type=binance"}), nil
		}
		in.BinanceAPIKey = *req.Body.ApiKey
		in.BinanceAPISecret = *req.Body.ApiSecret
	}

	a, err := s.deps.Account.Create(ctx, user.ID, in)
	if err != nil {
		switch {
		case errors.Is(err, account.ErrTypeNotSupported):
			return createAccountValidationResp("Type not supported",
				map[string]string{"type": "supported types: manual, tinvest"}), nil
		case errors.Is(err, account.ErrTokenInvalid):
			fields := map[string]string{}
			switch string(req.Body.Type) {
			case account.TypeTInvest:
				fields["token"] = "rejected by T-Invest"
			case account.TypeBybit:
				fields["apiKey"] = "rejected by Bybit"
			case account.TypeBinance:
				fields["apiKey"] = "rejected by Binance"
			default:
				fields["token"] = "credentials rejected"
			}
			return createAccountValidationResp("Invalid credentials", fields), nil
		case errors.Is(err, source.ErrSubAccountNotFound):
			return createAccountValidationResp("Sub-account not found",
				map[string]string{"tinvestAccountId": "not found in your T-Invest account list"}), nil
		}
		return nil, err
	}
	s.deps.Logger.Info("account: created", "user_id", user.ID, "account_id", a.ID, "type", a.SourceType)

	// For brokerage types kick off async first sync.
	if a.SourceType != account.TypeManual && s.deps.Syncer != nil {
		go func(id uuid.UUID) {
			ctx2, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			_ = s.deps.Syncer.Sync(ctx2, id)
		}(a.ID)
	}

	return oapi.CreateAccount201JSONResponse(toOapiAccount(a)), nil
}

func (s *serverImpl) PreviewTInvestAccounts(ctx context.Context, req oapi.PreviewTInvestAccountsRequestObject) (oapi.PreviewTInvestAccountsResponseObject, error) {
	if _, ok := auth.UserFromContext(ctx); !ok {
		return oapi.PreviewTInvestAccounts401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}
	if req.Body == nil || req.Body.Token == "" {
		return oapi.PreviewTInvestAccounts422ApplicationProblemPlusJSONResponse{
			ValidationErrorApplicationProblemPlusJSONResponse: validationProblem("Validation failed", map[string]string{"token": "required"}).build(),
		}, nil
	}
	subs, err := s.deps.Account.PreviewTInvest(ctx, req.Body.Token)
	if err != nil {
		if errors.Is(err, account.ErrTokenInvalid) {
			return oapi.PreviewTInvestAccounts422ApplicationProblemPlusJSONResponse{
				ValidationErrorApplicationProblemPlusJSONResponse: validationProblem("Invalid token", map[string]string{"token": "rejected by T-Invest"}).build(),
			}, nil
		}
		return nil, err
	}
	out := make([]oapi.TInvestSubAccount, len(subs))
	for i, sub := range subs {
		out[i] = oapi.TInvestSubAccount{
			Id:   sub.ID,
			Name: sub.Name,
			Type: oapi.TInvestSubAccountType(sub.Type),
		}
	}
	return oapi.PreviewTInvestAccounts200JSONResponse{SubAccounts: out}, nil
}

func (s *serverImpl) SyncAccount(ctx context.Context, req oapi.SyncAccountRequestObject) (oapi.SyncAccountResponseObject, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return oapi.SyncAccount401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}
	a, err := s.deps.Account.Get(ctx, user.ID, req.AccountId)
	if err != nil {
		if errors.Is(err, account.ErrNotFound) {
			return oapi.SyncAccount404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: oapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem("account")),
			}, nil
		}
		return nil, err
	}
	if a.SourceType == account.TypeManual {
		return oapi.SyncAccount422ApplicationProblemPlusJSONResponse{
			ValidationErrorApplicationProblemPlusJSONResponse: validationProblem("Manual accounts cannot be synced", nil).build(),
		}, nil
	}
	if s.deps.Syncer == nil {
		return nil, errors.New("syncer not configured")
	}
	_ = s.deps.Syncer.Sync(ctx, req.AccountId)

	// Re-read to surface fresh status.
	a, err = s.deps.Account.Get(ctx, user.ID, req.AccountId)
	if err != nil {
		return nil, err
	}
	return oapi.SyncAccount200JSONResponse(toOapiAccount(a)), nil
}

func (s *serverImpl) GetAccount(ctx context.Context, req oapi.GetAccountRequestObject) (oapi.GetAccountResponseObject, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return oapi.GetAccount401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}

	a, err := s.deps.Account.Get(ctx, user.ID, req.AccountId)
	if err != nil {
		if errors.Is(err, account.ErrNotFound) {
			return oapi.GetAccount404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: oapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem("account")),
			}, nil
		}
		return nil, err
	}

	positions, err := s.deps.Position.ListForAccount(ctx, user.ID, req.AccountId)
	if err != nil {
		return nil, err
	}

	return oapi.GetAccount200JSONResponse(toOapiAccountDetail(a, positions)), nil
}

func (s *serverImpl) UpdateAccount(ctx context.Context, req oapi.UpdateAccountRequestObject) (oapi.UpdateAccountResponseObject, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return oapi.UpdateAccount401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}
	if req.Body == nil {
		return updateAccountValidationResp("missing body", nil), nil
	}

	a, err := s.deps.Account.Rename(ctx, user.ID, req.AccountId, req.Body.Name)
	if err != nil {
		if errors.Is(err, account.ErrNotFound) {
			return oapi.UpdateAccount404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: oapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem("account")),
			}, nil
		}
		return nil, err
	}
	return oapi.UpdateAccount200JSONResponse(toOapiAccount(a)), nil
}

func (s *serverImpl) DeleteAccount(ctx context.Context, req oapi.DeleteAccountRequestObject) (oapi.DeleteAccountResponseObject, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return oapi.DeleteAccount401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}

	if err := s.deps.Account.Delete(ctx, user.ID, req.AccountId); err != nil {
		if errors.Is(err, account.ErrNotFound) {
			return oapi.DeleteAccount404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: oapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem("account")),
			}, nil
		}
		return nil, err
	}
	s.deps.Logger.Info("account: deleted", "user_id", user.ID, "account_id", req.AccountId)
	return oapi.DeleteAccount204Response{}, nil
}

// ----- positions -----

func (s *serverImpl) CreatePosition(ctx context.Context, req oapi.CreatePositionRequestObject) (oapi.CreatePositionResponseObject, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return oapi.CreatePosition401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}
	if req.Body == nil {
		return createPositionValidationResp("missing body", nil), nil
	}

	qty, err := decimal.NewFromString(req.Body.Quantity)
	if err != nil || qty.Sign() <= 0 {
		return createPositionValidationResp("Invalid quantity",
			map[string]string{"quantity": "must be a positive decimal"}), nil
	}

	pos, err := s.deps.Position.Create(ctx, user.ID, req.AccountId, req.Body.InstrumentId, qty)
	if err != nil {
		switch {
		case errors.Is(err, position.ErrAccountNotFound):
			return oapi.CreatePosition404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: oapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem("account")),
			}, nil
		case errors.Is(err, position.ErrInstrumentNotFound):
			return createPositionValidationResp("Validation failed",
				map[string]string{"instrumentId": "not found"}), nil
		case errors.Is(err, position.ErrAlreadyExists):
			return oapi.CreatePosition409ApplicationProblemPlusJSONResponse{
				ConflictApplicationProblemPlusJSONResponse: oapi.ConflictApplicationProblemPlusJSONResponse(conflictProblem("position already exists for this instrument; use PUT to update")),
			}, nil
		}
		return nil, err
	}

	inst, err := s.deps.Instrument.Get(ctx, pos.InstrumentID)
	if err != nil {
		return nil, err
	}
	return oapi.CreatePosition201JSONResponse(toOapiPosition(pos, inst)), nil
}

func (s *serverImpl) UpdatePosition(ctx context.Context, req oapi.UpdatePositionRequestObject) (oapi.UpdatePositionResponseObject, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return oapi.UpdatePosition401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}
	if req.Body == nil {
		return updatePositionValidationResp("missing body", nil), nil
	}

	qty, err := decimal.NewFromString(req.Body.Quantity)
	if err != nil || qty.Sign() <= 0 {
		return updatePositionValidationResp("Invalid quantity",
			map[string]string{"quantity": "must be a positive decimal"}), nil
	}

	pos, err := s.deps.Position.Update(ctx, user.ID, req.AccountId, req.InstrumentId, qty)
	if err != nil {
		switch {
		case errors.Is(err, position.ErrAccountNotFound), errors.Is(err, position.ErrNotFound):
			return oapi.UpdatePosition404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: oapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem("position")),
			}, nil
		}
		return nil, err
	}

	inst, err := s.deps.Instrument.Get(ctx, pos.InstrumentID)
	if err != nil {
		return nil, err
	}
	return oapi.UpdatePosition200JSONResponse(toOapiPosition(pos, inst)), nil
}

func (s *serverImpl) DeletePosition(ctx context.Context, req oapi.DeletePositionRequestObject) (oapi.DeletePositionResponseObject, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return oapi.DeletePosition401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}

	if err := s.deps.Position.Delete(ctx, user.ID, req.AccountId, req.InstrumentId); err != nil {
		if errors.Is(err, position.ErrAccountNotFound) || errors.Is(err, position.ErrNotFound) {
			return oapi.DeletePosition404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: oapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem("position")),
			}, nil
		}
		return nil, err
	}
	return oapi.DeletePosition204Response{}, nil
}

// ----- instruments -----

func (s *serverImpl) SearchInstruments(ctx context.Context, req oapi.SearchInstrumentsRequestObject) (oapi.SearchInstrumentsResponseObject, error) {
	if _, ok := auth.UserFromContext(ctx); !ok {
		return oapi.SearchInstruments401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}
	rows, err := s.deps.Instrument.Search(ctx, req.Params.Q)
	if err != nil {
		return nil, err
	}
	items := make([]oapi.Instrument, len(rows))
	for i, x := range rows {
		items[i] = toOapiInstrument(x)
	}
	return oapi.SearchInstruments200JSONResponse{Items: items, Total: len(items)}, nil
}

func (s *serverImpl) ListInstruments(ctx context.Context, req oapi.ListInstrumentsRequestObject) (oapi.ListInstrumentsResponseObject, error) {
	if _, ok := auth.UserFromContext(ctx); !ok {
		return oapi.ListInstruments401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}

	in := instrument.ListInput{Limit: 50, Offset: 0}
	if req.Params.Q != nil {
		in.Q = *req.Params.Q
	}
	if req.Params.AssetClass != nil {
		in.AssetClass = string(*req.Params.AssetClass)
	}
	if req.Params.Limit != nil {
		in.Limit = int32(*req.Params.Limit)
	}
	if req.Params.Offset != nil {
		in.Offset = int32(*req.Params.Offset)
	}

	res, err := s.deps.Instrument.List(ctx, in)
	if err != nil {
		return nil, err
	}
	items := make([]oapi.Instrument, len(res.Items))
	for i, x := range res.Items {
		items[i] = toOapiInstrument(x)
	}
	return oapi.ListInstruments200JSONResponse{Items: items, Total: int(res.Total)}, nil
}

// ----- portfolio -----

func (s *serverImpl) GetPortfolio(ctx context.Context, req oapi.GetPortfolioRequestObject) (oapi.GetPortfolioResponseObject, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return oapi.GetPortfolio401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}

	displayCcy := user.DisplayCurrency
	if req.Params.Currency != nil && *req.Params.Currency != "" {
		displayCcy = *req.Params.Currency
	}

	pf, err := s.deps.Portfolio.Compute(ctx, user.ID, displayCcy)
	if err != nil {
		return nil, err
	}

	return oapi.GetPortfolio200JSONResponse(toOapiPortfolio(pf)), nil
}

func toOapiPortfolio(pf portfolio.Portfolio) oapi.Portfolio {
	pos := make([]oapi.PortfolioPosition, len(pf.Positions))
	for i, p := range pf.Positions {
		pp := oapi.PortfolioPosition{
			AccountId:    p.AccountID,
			AccountName:  p.AccountName,
			InstrumentId: p.InstrumentID,
			Ticker:       p.Ticker,
			AssetClass:   oapi.AssetClass(p.AssetClass),
			Currency:     p.Currency,
			Quantity:     p.Quantity.String(),
			PriceStale:   p.PriceStale,
		}
		if p.Price != nil {
			s := p.Price.String()
			pp.Price = &s
		}
		if p.ValueNative != nil {
			s := p.ValueNative.String()
			pp.ValueNative = &s
		}
		if p.ValueDisplay != nil {
			s := p.ValueDisplay.String()
			pp.ValueDisplay = &s
		}
		if p.PriceFetchedAt != nil {
			t := *p.PriceFetchedAt
			pp.PriceFetchedAt = &t
		}
		pos[i] = pp
	}
	return oapi.Portfolio{
		Summary: oapi.PortfolioSummary{
			DisplayCurrency: pf.Summary.DisplayCurrency,
			GrandTotal:      pf.Summary.GrandTotal.String(),
			ByAssetClass:    decimalMapToString(pf.Summary.ByAssetClass),
			ByCurrency:      decimalMapToString(pf.Summary.ByCurrency),
			ByAccount:       decimalMapToString(pf.Summary.ByAccount),
		},
		Positions: pos,
	}
}

func decimalMapToString(m map[string]decimal.Decimal) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v.String()
	}
	return out
}

func (s *serverImpl) GetPortfolioHistory(ctx context.Context, req oapi.GetPortfolioHistoryRequestObject) (oapi.GetPortfolioHistoryResponseObject, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return oapi.GetPortfolioHistory401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	to := today
	if req.Params.To != nil {
		to = req.Params.To.Time
	}
	from := to.AddDate(0, 0, -90)
	if req.Params.From != nil {
		from = req.Params.From.Time
	}
	if from.After(to) {
		return oapi.GetPortfolioHistory422ApplicationProblemPlusJSONResponse{
			ValidationErrorApplicationProblemPlusJSONResponse: validationProblem("from must be on or before to", map[string]string{"from": "after to"}).build(),
		}, nil
	}

	rows, err := s.deps.Queries.ListPortfolioSnapshotsByDateRange(ctx, storage.ListPortfolioSnapshotsByDateRangeParams{
		UserID:         user.ID,
		SnapshotDate:   pgtype.Date{Time: from, Valid: true},
		SnapshotDate_2: pgtype.Date{Time: to, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}

	points := make([]oapi.PortfolioSnapshot, 0, len(rows))
	for _, r := range rows {
		byAssetClass, err := decodeAmounts(r.ByAssetClass)
		if err != nil {
			return nil, fmt.Errorf("decode by_asset_class: %w", err)
		}
		byCurrency, err := decodeAmounts(r.ByCurrency)
		if err != nil {
			return nil, fmt.Errorf("decode by_currency: %w", err)
		}
		byAccount, err := decodeAmounts(r.ByAccount)
		if err != nil {
			return nil, fmt.Errorf("decode by_account: %w", err)
		}
		points = append(points, oapi.PortfolioSnapshot{
			Date:            openapi_types.Date{Time: r.SnapshotDate.Time},
			DisplayCurrency: r.DisplayCurrency,
			GrandTotal:      r.GrandTotal.String(),
			ByAssetClass:    byAssetClass,
			ByCurrency:      byCurrency,
			ByAccount:       byAccount,
		})
	}

	return oapi.GetPortfolioHistory200JSONResponse{
		CurrentDisplayCurrency: user.DisplayCurrency,
		Points:                 points,
	}, nil
}

func decodeAmounts(b []byte) (map[string]string, error) {
	if len(b) == 0 {
		return map[string]string{}, nil
	}
	m := map[string]string{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// ----- deposits -----

func (s *serverImpl) ListDeposits(ctx context.Context, _ oapi.ListDepositsRequestObject) (oapi.ListDepositsResponseObject, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return oapi.ListDeposits401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}
	rows, err := s.deps.Deposits.List(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	items := make([]oapi.Deposit, len(rows))
	for i, d := range rows {
		items[i] = toOapiDeposit(d)
	}
	return oapi.ListDeposits200JSONResponse{Items: items}, nil
}

func (s *serverImpl) CreateDeposit(ctx context.Context, req oapi.CreateDepositRequestObject) (oapi.CreateDepositResponseObject, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return oapi.CreateDeposit401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}
	if req.Body == nil {
		return createDepositValidationResp("missing body", nil), nil
	}
	amt, err := decimal.NewFromString(req.Body.Amount)
	if err != nil || amt.Sign() <= 0 || !amt.IsInteger() {
		return createDepositValidationResp("Invalid amount",
			map[string]string{"amount": "must be a positive integer"}), nil
	}

	d, err := s.deps.Deposits.Create(ctx, user.ID, req.Body.Month.Time, amt)
	if err != nil {
		return nil, err
	}
	s.deps.Logger.Info("deposit: created", "user_id", user.ID, "deposit_id", d.ID)
	return oapi.CreateDeposit201JSONResponse(toOapiDeposit(d)), nil
}

func (s *serverImpl) DeleteDeposit(ctx context.Context, req oapi.DeleteDepositRequestObject) (oapi.DeleteDepositResponseObject, error) {
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return oapi.DeleteDeposit401ApplicationProblemPlusJSONResponse{
			UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
		}, nil
	}
	if err := s.deps.Deposits.Delete(ctx, user.ID, req.DepositId); err != nil {
		if errors.Is(err, deposits.ErrNotFound) {
			return oapi.DeleteDeposit404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: oapi.NotFoundApplicationProblemPlusJSONResponse(notFoundProblem("deposit")),
			}, nil
		}
		return nil, err
	}
	s.deps.Logger.Info("deposit: deleted", "user_id", user.ID, "deposit_id", req.DepositId)
	return oapi.DeleteDeposit204Response{}, nil
}

func toOapiDeposit(d deposits.Deposit) oapi.Deposit {
	return oapi.Deposit{
		Id:        d.ID,
		Month:     openapi_types.Date{Time: d.Month},
		Amount:    d.Amount.String(),
		CreatedAt: d.CreatedAt,
	}
}

// ----- mappers -----

func toOapiUser(u auth.User) oapi.User {
	return oapi.User{
		Id:              u.ID,
		Email:           openapi_types.Email(u.Email),
		DisplayCurrency: u.DisplayCurrency,
		CreatedAt:       u.CreatedAt,
	}
}

func toOapiAccount(a account.Account) oapi.Account {
	out := oapi.Account{
		Id:            a.ID,
		Name:          a.Name,
		Type:          oapi.AccountType(a.SourceType),
		LastSyncedAt:  a.LastSyncedAt,
		LastSyncError: a.LastSyncError,
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
	}
	if a.LastSyncStatus != nil {
		st := oapi.AccountSyncStatus(*a.LastSyncStatus)
		out.LastSyncStatus = &st
	}
	return out
}

func toOapiAccountDetail(a account.Account, positions []position.EnrichedPosition) oapi.AccountDetail {
	base := toOapiAccount(a)
	posList := make([]oapi.Position, len(positions))
	for i, p := range positions {
		posList[i] = toOapiPosition(p.Position, p.Instrument)
	}
	return oapi.AccountDetail{
		Id:             base.Id,
		Name:           base.Name,
		Type:           base.Type,
		LastSyncedAt:   base.LastSyncedAt,
		LastSyncStatus: base.LastSyncStatus,
		LastSyncError:  base.LastSyncError,
		CreatedAt:      base.CreatedAt,
		UpdatedAt:      base.UpdatedAt,
		Positions:      posList,
	}
}

func toOapiInstrument(i instrument.Instrument) oapi.Instrument {
	out := oapi.Instrument{
		Id:         i.ID,
		Ticker:     i.Ticker,
		AssetClass: oapi.AssetClass(i.AssetClass),
		Currency:   i.Currency,
		Name:       i.Name,
		CreatedAt:  i.CreatedAt,
		UpdatedAt:  i.UpdatedAt,
	}
	if i.CurrentPrice != nil {
		s := i.CurrentPrice.String()
		out.CurrentPrice = &s
	}
	if i.PriceFetchedAt != nil && i.AssetClass != "cash" {
		t := *i.PriceFetchedAt
		out.PriceUpdatedAt = &t
	}
	return out
}

func toOapiPosition(p position.Position, inst instrument.Instrument) oapi.Position {
	return oapi.Position{
		AccountId:  p.AccountID,
		Instrument: toOapiInstrument(inst),
		Quantity:   p.Quantity.String(),
		UpdatedAt:  p.UpdatedAt,
	}
}

// ----- helpers -----

func buildSessionCookie(token string, maxAge int, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

type logoutWithClearCookie struct {
	secure bool
}

func (r logoutWithClearCookie) VisitLogoutResponse(w http.ResponseWriter) error {
	clear := &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.secure,
		SameSite: http.SameSiteLaxMode,
	}
	w.Header().Set("Set-Cookie", clear.String())
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// ----- problem builders -----

func unauthorizedProblem() oapi.Problem {
	t := "/errors/unauthorized"
	d := "Authentication required"
	return oapi.Problem{Type: &t, Title: "Unauthorized", Status: 401, Detail: &d}
}

func invalidCredentialsProblem() oapi.Problem {
	t := "/errors/invalid-credentials"
	d := "Invalid email or password"
	return oapi.Problem{Type: &t, Title: "Invalid credentials", Status: 401, Detail: &d}
}

func notFoundProblem(resource string) oapi.Problem {
	t := "/errors/not-found"
	d := fmt.Sprintf("%s not found", resource)
	return oapi.Problem{Type: &t, Title: "Not found", Status: 404, Detail: &d}
}

func conflictProblem(detail string) oapi.Problem {
	t := "/errors/conflict"
	d := detail
	return oapi.Problem{Type: &t, Title: "Conflict", Status: 409, Detail: &d}
}

type validationFlex struct {
	title  string
	fields *map[string]string
}

func (v validationFlex) build() oapi.ValidationErrorApplicationProblemPlusJSONResponse {
	t := "/errors/validation"
	return oapi.ValidationErrorApplicationProblemPlusJSONResponse{
		Type:   &t,
		Title:  v.title,
		Status: 422,
		Fields: v.fields,
	}
}

func (v validationFlex) asLoginResponse() oapi.Login422ApplicationProblemPlusJSONResponse {
	return oapi.Login422ApplicationProblemPlusJSONResponse{ValidationErrorApplicationProblemPlusJSONResponse: v.build()}
}

func validationProblem(title string, fields map[string]string) validationFlex {
	if fields != nil {
		return validationFlex{title: title, fields: &fields}
	}
	return validationFlex{title: title}
}

func createAccountValidationResp(title string, fields map[string]string) oapi.CreateAccount422ApplicationProblemPlusJSONResponse {
	return oapi.CreateAccount422ApplicationProblemPlusJSONResponse{ValidationErrorApplicationProblemPlusJSONResponse: validationProblem(title, fields).build()}
}
func updateAccountValidationResp(title string, fields map[string]string) oapi.UpdateAccount422ApplicationProblemPlusJSONResponse {
	return oapi.UpdateAccount422ApplicationProblemPlusJSONResponse{ValidationErrorApplicationProblemPlusJSONResponse: validationProblem(title, fields).build()}
}
func createPositionValidationResp(title string, fields map[string]string) oapi.CreatePosition422ApplicationProblemPlusJSONResponse {
	return oapi.CreatePosition422ApplicationProblemPlusJSONResponse{ValidationErrorApplicationProblemPlusJSONResponse: validationProblem(title, fields).build()}
}
func updatePositionValidationResp(title string, fields map[string]string) oapi.UpdatePosition422ApplicationProblemPlusJSONResponse {
	return oapi.UpdatePosition422ApplicationProblemPlusJSONResponse{ValidationErrorApplicationProblemPlusJSONResponse: validationProblem(title, fields).build()}
}
func createDepositValidationResp(title string, fields map[string]string) oapi.CreateDeposit422ApplicationProblemPlusJSONResponse {
	return oapi.CreateDeposit422ApplicationProblemPlusJSONResponse{ValidationErrorApplicationProblemPlusJSONResponse: validationProblem(title, fields).build()}
}
// ----- strict server error handlers -----

func notImplementedHandler(w http.ResponseWriter, _ *http.Request, err error) {
	w.Header().Set("Content-Type", "application/problem+json")
	if errors.Is(err, errNotImplemented) {
		w.WriteHeader(http.StatusNotImplemented)
		fmt.Fprint(w, `{"title":"Not implemented","status":501,"type":"/errors/not-implemented"}`)
		return
	}
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprint(w, `{"title":"Internal server error","status":500,"type":"/errors/internal"}`)
}

func requestErrorHandler(w http.ResponseWriter, _ *http.Request, _ error) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprint(w, `{"title":"Bad request","status":400,"type":"/errors/bad-request"}`)
}
