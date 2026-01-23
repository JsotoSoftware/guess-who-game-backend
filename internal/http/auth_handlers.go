package http

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/JsotoSoftware/guess-who-game-backend/internal/auth"
	"github.com/JsotoSoftware/guess-who-game-backend/internal/storage"
)

type AuthHandlers struct {
	Store        *storage.Storage
	Tokens       *auth.TokenMaker
	CookieSecure bool
	CookieDomain string
}

type tokenResp struct {
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

func NewAuthHandlers(store *storage.Storage, tokens *auth.TokenMaker, cookieSecure bool, cookieDomain string) *AuthHandlers {
	return &AuthHandlers{
		Store:        store,
		Tokens:       tokens,
		CookieSecure: cookieSecure,
		CookieDomain: cookieDomain,
	}
}

func (h *AuthHandlers) Guest(w http.ResponseWriter, r *http.Request) {
	if _, err := r.Cookie(auth.RefreshCookieName); err == nil {
		h.Refresh(w, r)
		return
	}

	refreshToken, err := auth.NewRefreshToken()
	if err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}
	hash := auth.HashToken(refreshToken)

	expiresAt := time.Now().UTC().Add(30 * 24 * time.Hour)
	ua := r.UserAgent()
	ip := clientIP(r)

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	userID, _, err := h.Store.CreateGuestUserAndSession(ctx, hash, expiresAt, ua, ip)
	if err != nil {
		log.Error().Err(err).Msg("create guest failed")
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}

	setRefreshCookie(w, refreshToken, expiresAt, h.CookieSecure, h.CookieDomain)

	access, accessExp, err := h.Tokens.NewAccessToken(userID, 15*time.Minute)
	if err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, tokenResp{AccessToken: access, ExpiresAt: accessExp})
}

func (h *AuthHandlers) Refresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(auth.RefreshCookieName)
	if err != nil || c.Value == "" {
		http.Error(w, "missing refresh", http.StatusUnauthorized)
		return
	}

	oldToken := c.Value
	oldHash := auth.HashToken(oldToken)

	newToken, err := auth.NewRefreshToken()
	if err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}

	newHash := auth.HashToken(newToken)
	newExpires := time.Now().UTC().Add(30 * 24 * time.Hour)

	ua := r.UserAgent()
	ip := clientIP(r)

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	_, userID, err := h.Store.RotateRefreshSession(ctx, oldHash, newHash, newExpires, ua, ip)
	if err != nil {
		http.Error(w, "invalid refresh", http.StatusUnauthorized)
		return
	}

	setRefreshCookie(w, newToken, newExpires, h.CookieSecure, h.CookieDomain)

	access, accessExp, err := h.Tokens.NewAccessToken(userID, 15*time.Minute)
	if err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, tokenResp{AccessToken: access, ExpiresAt: accessExp})
}

func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(auth.RefreshCookieName)
	if err == nil && c.Value != "" {
		hash := auth.HashToken(c.Value)
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		_ = h.Store.RevokeRefreshSession(ctx, hash)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     auth.RefreshCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.CookieSecure,
		Domain:   h.CookieDomain,
	})

	w.WriteHeader(http.StatusNoContent)
}

func setRefreshCookie(w http.ResponseWriter, token string, exp time.Time, secure bool, domain string) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.RefreshCookieName,
		Value:    token,
		Path:     "/",
		Expires:  exp,
		MaxAge:   int(time.Until(exp).Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Domain:   domain,
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return strings.Split(host, "%")[0]
}
