package auth

import (
	"context"
	"net/http"
)

const SessionCookieName = "sid"

type userCtxKey struct{}
type cookieCtxKey struct{}

// Middleware reads the session cookie, resolves it through the service, and
// stores the resolved User and raw cookie value in the request context. It
// does NOT block requests without a valid session — handlers / route groups
// decide whether auth is required.
func Middleware(svc *Service) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(SessionCookieName)
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}
			ctx := context.WithValue(r.Context(), cookieCtxKey{}, cookie.Value)
			user, err := svc.Resolve(ctx, cookie.Value)
			if err != nil {
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			ctx = context.WithValue(ctx, userCtxKey{}, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireUser is middleware for protected route groups: returns 401 if no user
// in context. Use after Middleware.
func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserFromContext(r.Context()); !ok {
			http.Error(w, `{"title":"Unauthenticated","status":401}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userCtxKey{}).(User)
	return u, ok
}

func MustUserFromContext(ctx context.Context) User {
	u, ok := UserFromContext(ctx)
	if !ok {
		panic("auth: no user in context (route not protected by RequireUser)")
	}
	return u
}

// CookieFromContext returns the raw session cookie value if a request carried one.
func CookieFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(cookieCtxKey{}).(string)
	return v, ok
}

// RequireAdmin gates routes behind a fixed bearer token (ADMIN_API_KEY).
// Used for service-to-service calls (e.g. the daily price-refresh cron).
func RequireAdmin(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if key == "" {
				http.Error(w, `{"title":"Admin disabled","status":503}`, http.StatusServiceUnavailable)
				return
			}
			const prefix = "Bearer "
			h := r.Header.Get("Authorization")
			if len(h) <= len(prefix) || h[:len(prefix)] != prefix || h[len(prefix):] != key {
				w.Header().Set("Content-Type", "application/problem+json")
				http.Error(w, `{"title":"Unauthorized","status":401,"type":"/errors/unauthorized"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// SetCookie writes the session cookie to the response.
func SetCookie(w http.ResponseWriter, token string, maxAge int, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearCookie unsets the session cookie.
func ClearCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
