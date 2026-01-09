package ws

import (
	"encoding/json"
	"sync"
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

type RoomHub struct {
	code    string
	mu      sync.Mutex
	conns   map[string]Conn
	members map[string]MemberState
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
		code:    code,
		conns:   map[string]Conn{},
		members: map[string]MemberState{},
	}
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
}

func (r *RoomHub) Unregister(userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.conns, userID)
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
}

func (r *RoomHub) SendTo(userID string, msg any) {
	r.mu.Lock()
	c := r.conns[userID]
	r.mu.Unlock()
	if c != nil {
		_ = c.Send(msg)
	}
}

func Marshal(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
