package ws

import (
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
					r.mu.Lock()
					for uid, c := range r.conns {
						wc, ok := c.(*WSConn)
						if ok {
							if now.Sub(wc.LastSeen()) > cfg.ConnIdleTimeout {
								_ = wc.Close()
								delete(r.conns, uid)

								member, memberOk := r.members[uid]
								if memberOk {
									member.Connected = false
									r.members[uid] = member
								}
							}
						}
					}

					empty := len(r.conns) == 0
					last := r.lastActivity
					r.mu.Unlock()

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
