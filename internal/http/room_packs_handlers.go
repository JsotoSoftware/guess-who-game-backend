package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/JsotoSoftware/guess-who-game-backend/internal/storage"
	"github.com/jackc/pgx/v5"
)

type RoomPacksHandlers struct {
	Store *storage.Storage
}

func NewRoomPacksHandlers(store *storage.Storage) *RoomPacksHandlers {
	return &RoomPacksHandlers{
		Store: store,
	}
}

type setRoomPacksReq struct {
	Code           string   `json:"code"`
	PackSlugs      []string `json:"packSlugs,omitempty"`
	CollectionSlug string   `json:"collectionSlug,omitempty"`
}

func (h *RoomPacksHandlers) Set(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req setRoomPacksReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.Code == "" {
		http.Error(w, "code required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	room, err := h.Store.GetRoomByCode(ctx, req.Code)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}

	isMember, err := h.Store.IsRoomMember(ctx, room.ID, userID)
	if err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}
	if !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	hostMember, err := h.Store.GetRoomMember(ctx, room.ID, userID)
	if err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}
	if hostMember.Role != "host" {
		http.Error(w, "host only", http.StatusForbidden)
		return
	}

	packSlugs := req.PackSlugs
	if req.CollectionSlug != "" {
		packs, err := h.Store.ListPacksForCollection(ctx, req.CollectionSlug, "es")
		if err != nil {
			http.Error(w, "failed", http.StatusInternalServerError)
			return
		}
		packSlugs = packSlugs[:0]
		for _, p := range packs {
			packSlugs = append(packSlugs, p.Slug)
		}
	}

	if len(packSlugs) == 0 {
		http.Error(w, "packSlugs or collectionSlug required", http.StatusBadRequest)
		return
	}

	if err := h.Store.SetRoomPackSelectionBySlugs(ctx, room.ID, packSlugs); err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *RoomPacksHandlers) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "code required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	room, err := h.Store.GetRoomByCode(ctx, code)
	if err != nil {
		http.Error(w, "room not found", http.StatusNotFound)
		return
	}

	isMember, err := h.Store.IsRoomMember(ctx, room.ID, userID)
	if err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}
	if !isMember {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	slugs, err := h.Store.GetRoomSelectedPackSlugs(ctx, room.ID)
	if err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"code": code, "packSlugs": slugs})
}
