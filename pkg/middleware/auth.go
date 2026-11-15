package middleware

import (
	"context"
	"net/http"
	"strings"

	"edgeaware/internal/auth"
	"edgeaware/internal/httpx"
)

type ctxKey struct{}

// Auth rejects requests without a valid Bearer token and stores the token's
// claims in the request context.
func Auth(tokens *auth.Tokens) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scheme, tokenStr, ok := strings.Cut(r.Header.Get("Authorization"), " ")
			if !ok || scheme != "Bearer" || tokenStr == "" {
				httpx.Error(w, http.StatusUnauthorized, "missing or invalid Authorization header")
				return
			}
			claims, err := tokens.Verify(tokenStr)
			if err != nil {
				httpx.Error(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, claims)))
		})
	}
}

// Claims returns the verified token claims stored by Auth, or nil.
func Claims(ctx context.Context) *auth.Claims {
	c, _ := ctx.Value(ctxKey{}).(*auth.Claims)
	return c
}

// UserID returns the logged-in user's ID, or "" if the request is unauthenticated.
func UserID(ctx context.Context) string {
	if c := Claims(ctx); c != nil {
		return c.UserID
	}
	return ""
}
