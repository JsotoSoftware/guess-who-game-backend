package http

import (
	"context"
	"net/http"
	"time"

	"github.com/JsotoSoftware/guess-who-game-backend/internal/storage"
)

type Handler struct {
	storage storage.Repository
}

func NewHandler(storage storage.Repository) *Handler {
	return &Handler{
		storage: storage,
	}
}

func (h *Handler) healthCheck(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (h *Handler) readyCheck(w http.ResponseWriter, _ *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := h.storage.Ping(ctx); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("storage not ready: " + err.Error()))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ready"))
}
