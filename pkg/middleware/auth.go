package middleware

import (
	"context"
	"net/http"
	"strings"

	"edgeaware/internal/auth"
)

type ctxKey string

const claimsKey ctxKey = "jwt_claims"

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "missing Authorization header", http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "invalid Authorization format", http.StatusUnauthorized)
			return
		}

		tokenStr := parts[1]

		claims, err := auth.VerifyToken(tokenStr)
		if err != nil {
			http.Error(w, "invalid or expired token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Optional helper if you want to read claims in handlers later.
func GetClaims(r *http.Request) map[string]any {
	v := r.Context().Value(claimsKey)
	if v == nil {
		return nil
	}
	if c, ok := v.(map[string]any); ok {
		return c
	}
	return nil
}
