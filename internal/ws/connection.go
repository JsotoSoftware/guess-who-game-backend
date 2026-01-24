package ws

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/rs/zerolog/log"
)

type WSConn struct {
	c      *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc

	userID string
	role   string
	name   string

	sendCh chan []byte
	once   sync.Once

	lastMu   sync.Mutex
	lastSeen time.Time
}

func NewWSConn(c *websocket.Conn, userID, role, name string) *WSConn {
	ctx, cancel := context.WithCancel(context.Background())
	w := &WSConn{
		c:        c,
		ctx:      ctx,
		cancel:   cancel,
		userID:   userID,
		role:     role,
		name:     name,
		sendCh:   make(chan []byte, 64),
		lastSeen: time.Now(),
	}
	go w.writeLoop()
	return w
}

func (w *WSConn) Close() error {
	w.once.Do(func() {
		w.cancel()
		close(w.sendCh)
		_ = w.c.Close(websocket.StatusNormalClosure, "bye")
	})
	return nil
}

func (w *WSConn) UserID() string      { return w.userID }
func (w *WSConn) Role() string        { return w.role }
func (w *WSConn) DisplayName() string { return w.name }

func (w *WSConn) Send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		log.Error().Str("user", w.userID).Err(err).Msg("ws: failed to marshal message")
		return err
	}

	var msgType string
	if msg, ok := v.(map[string]any); ok {
		if t, ok := msg["type"].(string); ok {
			msgType = t
		}
	}

	select {
	case w.sendCh <- b:
		log.Debug().
			Str("user", w.userID).
			Str("type", msgType).
			Msg("ws: message queued for sending")
		return nil
	case <-w.ctx.Done():
		log.Warn().Str("user", w.userID).Str("type", msgType).Msg("ws: failed to queue message (connection closed)")
		return errors.New("connection closed")
	}
}

func (w *WSConn) writeLoop() {
	for {
		select {
		case b, ok := <-w.sendCh:
			if !ok {
				log.Debug().Str("user", w.userID).Msg("ws: writeLoop channel closed")
				return
			}

			var msgType string
			var msg map[string]any
			if err := json.Unmarshal(b, &msg); err == nil {
				if t, ok := msg["type"].(string); ok {
					msgType = t
				}
			}

			writeCtx, writeCancel := context.WithTimeout(context.Background(), 5*time.Second)
			err := w.c.Write(writeCtx, websocket.MessageText, b)
			writeCancel()

			if err != nil {
				log.Error().
					Str("user", w.userID).
					Str("type", msgType).
					Err(err).
					Msg("ws: failed to write message")
				_ = w.Close()
				return
			}

			log.Info().
				Str("user", w.userID).
				Str("type", msgType).
				Int("bytes", len(b)).
				Msg("ws: message sent")

		case <-w.ctx.Done():
			log.Debug().Str("user", w.userID).Msg("ws: writeLoop context cancelled")
			return
		}
	}
}

func (w *WSConn) Read(ctx context.Context) ([]byte, error) {
	_, b, err := w.c.Read(ctx)
	return b, err
}

func (w *WSConn) Touch() {
	w.lastMu.Lock()
	defer w.lastMu.Unlock()
	w.lastSeen = time.Now()
}

func (w *WSConn) LastSeen() time.Time {
	w.lastMu.Lock()
	defer w.lastMu.Unlock()
	t := w.lastSeen
	return t
}
