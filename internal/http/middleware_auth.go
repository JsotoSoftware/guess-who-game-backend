package http

import (
	"context"
	"net/http"

	"github.com/JsotoSoftware/guess-who-game-backend/internal/auth"
)

type ctxKey string

const ctxUserID ctxKey = "user_id"

func UserIDFromContext(ctx context.Context) (string, bool) {
	v := ctx.Value(ctxUserID)
	s, ok := v.(string)
	return s, ok
}

func RequireAuth(tokens *auth.TokenMaker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := auth.ExtractBearerToken(r.Header.Get("Authorization"))
			if !ok {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}

			claims, err := tokens.ParseAccessToken(raw)
			if err != nil || claims.UserID == "" {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ctxUserID, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
