package ws

import (
	"encoding/json"
	"sync"
	"time"
)

type Conn interface {
	Send(v any) error
	Close() error
	UserID() string
	Role() string
	DisplayName() string
}

type Hub struct {
	mu    sync.Mutex
	rooms map[string]*RoomHub
}

func NewHub() *Hub {
	return &Hub{rooms: make(map[string]*RoomHub)}
}

type RoomHub struct {
	code         string
	mu           sync.Mutex
	conns        map[string]Conn
	members      map[string]MemberState
	lastActivity time.Time
}

type MemberState struct {
	UserID      string `json:"userId"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Score       int    `json:"score"`
	Connected   bool   `json:"connected"`
}

func NewRoomHub(code string) *RoomHub {
	return &RoomHub{
		code:         code,
		conns:        map[string]Conn{},
		members:      map[string]MemberState{},
		lastActivity: time.Now(),
	}
}

func (h *Hub) GetRoom(code string) *RoomHub {
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := h.rooms[code]
	if !ok {
		r = NewRoomHub(code)
		h.rooms[code] = r
	}
	return r
}

func (r *RoomHub) UpsertMemberState(m MemberState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.members[m.UserID] = m
}

func (r *RoomHub) SetConnected(userID string, connected bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, ok := r.members[userID]
	if ok {
		m.Connected = connected
		r.members[userID] = m
	}
}

func (r *RoomHub) Register(c Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if old, ok := r.conns[c.UserID()]; ok {
		_ = old.Close()
	}

	r.conns[c.UserID()] = c
	r.lastActivity = time.Now()
}

func (r *RoomHub) Unregister(userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.conns, userID)
	r.lastActivity = time.Now()
}

func (r *RoomHub) BroadcastPresence() {
	r.mu.Lock()
	members := make([]MemberState, 0, len(r.members))
	for _, m := range r.members {
		members = append(members, m)
	}
	conns := make([]Conn, 0, len(r.conns))
	for _, c := range r.conns {
		conns = append(conns, c)
	}
	r.mu.Unlock()

	msg := map[string]any{
		"type": "room:presence",
		"payload": map[string]any{
			"code":    r.code,
			"members": members,
		},
	}

	for _, c := range conns {
		_ = c.Send(msg)
	}

	r.mu.Lock()
	r.lastActivity = time.Now()
	r.mu.Unlock()
}

func (r *RoomHub) SendTo(userID string, msg any) {
	r.mu.Lock()
	c := r.conns[userID]
	r.mu.Unlock()
	if c != nil {
		_ = c.Send(msg)
	}
}

func (h *Hub) RoomSnapshot() map[string]*RoomHub {
	h.mu.Lock()
	defer h.mu.Unlock()
	snap := make(map[string]*RoomHub, len(h.rooms))
	for k, v := range h.rooms {
		snap[k] = v
	}
	return snap
}

func (h *Hub) DeleteRoom(code string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms, code)
}

func (h *Hub) TryDeleteEmptyRoom(code string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	r, ok := h.rooms[code]
	if !ok {
		return false
	}

	r.mu.Lock()
	empty := len(r.conns) == 0
	r.mu.Unlock()

	if empty {
		delete(h.rooms, code)
		return true
	}
	return false
}

func (r *RoomHub) Touch() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastActivity = time.Now()
}

func (r *RoomHub) ConnCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.conns)
}

func (r *RoomHub) LastActivity() time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastActivity
}

func Marshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
