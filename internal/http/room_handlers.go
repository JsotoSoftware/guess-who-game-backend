package http

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"

	"github.com/JsotoSoftware/guess-who-game-backend/internal/domain"
	"github.com/JsotoSoftware/guess-who-game-backend/internal/storage"
)

type RoomsHandlers struct {
	Store *storage.Storage
}

type createRoomResp struct {
	Code string `json:"code"`
}

type joinRoomReq struct {
	Code        string `json:"code"`
	DisplayName string `json:"displayName"`
}

func NewRoomHandlers(store *storage.Storage) *RoomsHandlers {
	return &RoomsHandlers{
		Store: store,
	}
}

func (h *RoomsHandlers) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	const maxRetries = 5
	for i := 0; i < maxRetries; i++ {
		code, err := domain.NewRoomCode(6)
		if err != nil {
			http.Error(w, "failed", http.StatusInternalServerError)
			return
		}

		_, err = h.Store.CreateRoom(ctx, userID, code)
		if err == nil {
			writeJSON(w, createRoomResp{Code: code})
			return
		}

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			http.Error(w, "failed to create room", http.StatusInternalServerError)
			return
		}

		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * 10 * time.Millisecond)
		}
	}

	http.Error(w, "could not create room after retries", http.StatusInternalServerError)
}

func (h *RoomsHandlers) Join(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req joinRoomReq

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		log.Error().Err(err).Msg("failed to read request body")
		http.Error(w, "bad request: failed to read body", http.StatusBadRequest)
		return
	}

	if len(bodyBytes) == 0 {
		http.Error(w, "bad request: empty body", http.StatusBadRequest)
		return
	}

	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		log.Error().Err(err).Bytes("body", bodyBytes).Msg("JSON decode failed")
		http.Error(w, "bad request: invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Code == "" || req.DisplayName == "" {
		http.Error(w, "code and displayName required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
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

	if err := h.Store.JoinRoom(ctx, room.ID, userID, req.DisplayName); err != nil {
		http.Error(w, "failed to join", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *RoomsHandlers) Members(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	room, err := h.Store.GetRoomByCode(ctx, code)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}

	members, err := h.Store.ListRoomMembers(ctx, room.ID)
	if err != nil {
		http.Error(w, "failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, members)
}

func (h *RoomsHandlers) GetRoomStats(w http.ResponseWriter, r *http.Request) {
	_, ok := UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	room, err := h.Store.GetRoomByCode(ctx, code)
	if err != nil {
		if err == pgx.ErrNoRows {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}

		http.Error(w, "get room failed", http.StatusInternalServerError)
		return
	}

	members, err := h.Store.ListRoomMembers(ctx, room.ID)
	if err != nil {
		http.Error(w, "get members failed", http.StatusInternalServerError)
		return
	}

	packs, err := h.Store.GetRoomSelectedPackSlugs(ctx, room.ID)
	if err != nil {
		http.Error(w, "get packs failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"code":    room.Code,
		"room":    room,
		"members": members,
		"packs":   packs,
	})
}
