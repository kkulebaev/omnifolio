package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/kkulebaev/omnifolio/api/internal/auth"
	"github.com/kkulebaev/omnifolio/api/internal/server/oapi"
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

	token, user, err := s.deps.Auth.Login(ctx, string(req.Body.Email), req.Body.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			s.deps.Logger.Warn("auth: login failed", "email", string(req.Body.Email))
			return oapi.Login401ApplicationProblemPlusJSONResponse{
				UnauthorizedApplicationProblemPlusJSONResponse: oapi.UnauthorizedApplicationProblemPlusJSONResponse(unauthorizedProblem()),
			}, nil
		}
		return nil, err
	}

	s.deps.Logger.Info("auth: login ok", "user_id", user.ID)
	return oapi.Login200JSONResponse{
		Body:    toOapiUser(user),
		Headers: oapi.Login200ResponseHeaders{SetCookie: buildSessionCookie(token, s.deps.MaxAge, s.deps.Secure).String()},
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

// ----- accounts (M1.3 stubs) -----

func (s *serverImpl) ListAccounts(_ context.Context, _ oapi.ListAccountsRequestObject) (oapi.ListAccountsResponseObject, error) {
	return nil, errNotImplemented
}
func (s *serverImpl) CreateAccount(_ context.Context, _ oapi.CreateAccountRequestObject) (oapi.CreateAccountResponseObject, error) {
	return nil, errNotImplemented
}
func (s *serverImpl) GetAccount(_ context.Context, _ oapi.GetAccountRequestObject) (oapi.GetAccountResponseObject, error) {
	return nil, errNotImplemented
}
func (s *serverImpl) UpdateAccount(_ context.Context, _ oapi.UpdateAccountRequestObject) (oapi.UpdateAccountResponseObject, error) {
	return nil, errNotImplemented
}
func (s *serverImpl) DeleteAccount(_ context.Context, _ oapi.DeleteAccountRequestObject) (oapi.DeleteAccountResponseObject, error) {
	return nil, errNotImplemented
}

// ----- positions (M1.3 stubs) -----

func (s *serverImpl) CreatePosition(_ context.Context, _ oapi.CreatePositionRequestObject) (oapi.CreatePositionResponseObject, error) {
	return nil, errNotImplemented
}
func (s *serverImpl) UpdatePosition(_ context.Context, _ oapi.UpdatePositionRequestObject) (oapi.UpdatePositionResponseObject, error) {
	return nil, errNotImplemented
}
func (s *serverImpl) DeletePosition(_ context.Context, _ oapi.DeletePositionRequestObject) (oapi.DeletePositionResponseObject, error) {
	return nil, errNotImplemented
}

// ----- instruments (M1.3 stubs) -----

func (s *serverImpl) SearchInstruments(_ context.Context, _ oapi.SearchInstrumentsRequestObject) (oapi.SearchInstrumentsResponseObject, error) {
	return nil, errNotImplemented
}
func (s *serverImpl) CreateInstrument(_ context.Context, _ oapi.CreateInstrumentRequestObject) (oapi.CreateInstrumentResponseObject, error) {
	return nil, errNotImplemented
}

// ----- portfolio (M1.4 stub) -----

func (s *serverImpl) GetPortfolio(_ context.Context, _ oapi.GetPortfolioRequestObject) (oapi.GetPortfolioResponseObject, error) {
	return nil, errNotImplemented
}

// ----- helpers -----

func toOapiUser(u auth.User) oapi.User {
	return oapi.User{
		Id:              u.ID,
		Email:           openapi_types.Email(u.Email),
		DisplayCurrency: u.DisplayCurrency,
		CreatedAt:       u.CreatedAt,
	}
}

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

// logoutWithClearCookie is a custom LogoutResponseObject that sets Set-Cookie
// to clear the session cookie before writing 204.
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

type validationFlex struct {
	title  string
	fields *map[string]string
}

func (v validationFlex) asLoginResponse() oapi.Login422ApplicationProblemPlusJSONResponse {
	t := "/errors/validation"
	return oapi.Login422ApplicationProblemPlusJSONResponse{
		ValidationErrorApplicationProblemPlusJSONResponse: oapi.ValidationErrorApplicationProblemPlusJSONResponse{
			Type:   &t,
			Title:  v.title,
			Status: 422,
			Fields: v.fields,
		},
	}
}

func validationProblem(title string, fields map[string]string) validationFlex {
	if fields != nil {
		return validationFlex{title: title, fields: &fields}
	}
	return validationFlex{title: title}
}

// notImplementedHandler is the strict server's ResponseErrorHandlerFunc; it
// maps errNotImplemented to a 501 problem and other errors to 500.
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

// requestErrorHandler is invoked when oapi-codegen strict server fails to decode
// a request (malformed JSON, etc).
func requestErrorHandler(w http.ResponseWriter, _ *http.Request, _ error) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprint(w, `{"title":"Bad request","status":400,"type":"/errors/bad-request"}`)
}

// _ ensures uuid import is kept; remove once accounts handlers use it.
var _ = uuid.Nil
