package ws

import (
	"log"
	"time"
)

type SweeperConfig struct {
	ConnIdleTimeout time.Duration // 30s
	RoomIdleTimeout time.Duration // 10m
	Tick            time.Duration // 10s
}

func StartSweeper(h *Hub, cfg SweeperConfig) func() {
	stop := make(chan struct{})

	go func() {
		ticker := time.NewTicker(cfg.Tick)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				now := time.Now()
				rooms := h.RoomSnapshot()

				for code, r := range rooms {
					var toClose []Conn
					r.mu.Lock()
					for uid, c := range r.conns {
						wc, ok := c.(*WSConn)
						if !ok {
							continue
						}
						if now.Sub(wc.LastSeen()) <= cfg.ConnIdleTimeout {
							continue
						}
						toClose = append(toClose, c)
						delete(r.conns, uid)
						if m, ok := r.members[uid]; ok {
							m.Connected = false
							r.members[uid] = m
						} else {
							log.Printf("sweeper: member not found: %s", uid)
						}
					}
					empty := len(r.conns) == 0
					last := r.lastActivity
					r.mu.Unlock()

					for _, c := range toClose {
						_ = c.Close()
					}
					if len(toClose) > 0 {
						r.BroadcastPresence()
					}

					if empty && now.Sub(last) > cfg.RoomIdleTimeout {
						h.TryDeleteEmptyRoom(code)
					}
				}

			case <-stop:
				return
			}
		}
	}()
	return func() { close(stop) }
}
