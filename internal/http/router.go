package http

import (
	"github.com/JsotoSoftware/guess-who-game-backend/internal/auth"
	"github.com/JsotoSoftware/guess-who-game-backend/internal/storage"
	"github.com/JsotoSoftware/guess-who-game-backend/internal/ws"
	"github.com/go-chi/chi/v5"
)

func NewRouter(store *storage.Storage, tokens *auth.TokenMaker, cookieSecure bool, cookieDomain string, wsHandler *ws.Handler) *chi.Mux {
	r := chi.NewRouter()

	h := NewHandler(store)

	r.Get("/healthz", h.healthCheck)
	r.Get("/readyz", h.readyCheck)

	// WebSocket handler
	r.Get("/ws", wsHandler.ServeHTTP)

	// Auth handlers
	ah := NewAuthHandlers(store, tokens, cookieSecure, cookieDomain)
	r.Route("/v1/auth", func(r chi.Router) {
		r.Post("/guest", ah.Guest)
		r.Post("/refresh", ah.Refresh)
		r.Post("/logout", ah.Logout)
	})

	// Protected
	rh := NewRoomHandlers(store)
	ph := NewPacksHandlers(store)
	ch := NewCollectionsHandlers(store)
	rhp := NewRoomPacksHandlers(store)

	r.Route("/v1", func(r chi.Router) {
		r.Use(RequireAuth(tokens))

		r.Post("/rooms", rh.Create)
		r.Post("/rooms/join", rh.Join)
		r.Get("/rooms/members", rh.Members)

		r.Get("/packs", ph.List)
		r.Get("/packs/{slug}", ph.Get)
		r.Get("/packs/{slug}/characters", ph.Characters)

		r.Get("/collections", ch.List)
		r.Get("/collections/{slug}/packs", ch.Packs)

		r.Post("/rooms/packs", rhp.Set)
		r.Get("/rooms/packs", rhp.Get)
	})

	return r
}
