package http

import (
	"github.com/JsotoSoftware/guess-who-game-backend/internal/storage"
	"github.com/go-chi/chi/v5"
)

func NewRouter(storage storage.Repository) *chi.Mux {
	r := chi.NewRouter()

	h := NewHandler(storage)

	r.Get("/healthz", h.healthCheck)
	r.Get("/readyz", h.readyCheck)

	return r
}
