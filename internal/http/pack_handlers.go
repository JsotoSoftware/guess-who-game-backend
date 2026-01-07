package http

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/JsotoSoftware/guess-who-game-backend/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type PacksHandlers struct {
	Store *storage.Storage
}

func NewPacksHandlers(store *storage.Storage) *PacksHandlers {
	return &PacksHandlers{
		Store: store,
	}
}

func getLang(r *http.Request) string {
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		return "es"
	}
	return lang
}

func (h *PacksHandlers) List(w http.ResponseWriter, r *http.Request) {
	lang := getLang(r)

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	packs, err := h.Store.ListPacks(ctx, lang)
	if err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, packs)
}

func (h *PacksHandlers) Get(w http.ResponseWriter, r *http.Request) {
	lang := getLang(r)
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		http.Error(w, "missing slug", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	p, err := h.Store.GetPackBySlug(ctx, slug, lang)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, p)
}

func (h *PacksHandlers) Characters(w http.ResponseWriter, r *http.Request) {
	lang := getLang(r)
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		http.Error(w, "missing slug", http.StatusBadRequest)
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	items, err := h.Store.ListCharactersByPackSlug(ctx, slug, lang, limit, offset)
	if err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, items)
}
