package http

import (
	"context"
	"net/http"
	"time"

	"github.com/JsotoSoftware/guess-who-game-backend/internal/storage"
	"github.com/go-chi/chi/v5"
)

type CollectionsHandlers struct {
	Store *storage.Storage
}

func NewCollectionsHandlers(store *storage.Storage) *CollectionsHandlers {
	return &CollectionsHandlers{
		Store: store,
	}
}

func (h *CollectionsHandlers) List(w http.ResponseWriter, r *http.Request) {
	lang := getLang(r)

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	items, err := h.Store.ListCollections(ctx, lang)
	if err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, items)
}

func (h *CollectionsHandlers) Packs(w http.ResponseWriter, r *http.Request) {
	lang := getLang(r)
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		http.Error(w, "missing slug", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	packs, err := h.Store.ListPacksForCollection(ctx, slug, lang)
	if err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, packs)
}
