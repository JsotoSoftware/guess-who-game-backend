package ws

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/coder/websocket"
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
}

func NewWSConn(parent context.Context, c *websocket.Conn, userID, role, name string) *WSConn {
	ctx, cancel := context.WithCancel(parent)
	w := &WSConn{
		c:      c,
		ctx:    ctx,
		cancel: cancel,
		userID: userID,
		role:   role,
		name:   name,
		sendCh: make(chan []byte, 64),
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
		return err
	}
	select {
	case w.sendCh <- b:
		return nil
	case <-w.ctx.Done():
		return errors.New("connection closed")
	}
}

func (w *WSConn) writeLoop() {
	for {
		select {
		case b, ok := <-w.sendCh:
			if !ok {
				return
			}
			_ = w.c.Write(w.ctx, websocket.MessageText, b)
		case <-w.ctx.Done():
			return
		}
	}
}

func (w *WSConn) Read(ctx context.Context) ([]byte, error) {
	_, b, err := w.c.Read(ctx)
	return b, err
}
