package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/rs/zerolog/log"

	"github.com/JsotoSoftware/guess-who-game-backend/internal/auth"
	"github.com/JsotoSoftware/guess-who-game-backend/internal/storage"
)

type Handler struct {
	Hub    *Hub
	Store  *storage.Storage
	Tokens *auth.TokenMaker
}

func NewHandler(hub *Hub, store *storage.Storage, tokens *auth.TokenMaker) *Handler {
	return &Handler{
		Hub:    hub,
		Store:  store,
		Tokens: tokens,
	}
}

type Envelope struct {
	Type      string          `json:"type"`
	RequestID string          `json:"requestId,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
}

type JoinPayload struct {
	Code        string `json:"code"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
}

type StartRoundPayload struct {
	Code string `json:"code"`
	Lang string `json:"lang,omitempty"`
}

type ScoreAddPayload struct {
	Code   string `json:"code"`
	UserID string `json:"userId"`
	Delta  int    `json:"delta"`
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("access_token")
	if raw == "" {
		raw = r.URL.Query().Get("token")
	}
	if raw == "" {
		http.Error(w, "missing access_token", http.StatusUnauthorized)
		return
	}

	claims, err := h.Tokens.ParseAccessToken(raw)
	if err != nil || claims.UserID == "" {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	userID := claims.UserID

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return
	}
	defer c.Close(websocket.StatusInternalError, "server error")

	ctx := r.Context()

	type tmpConn struct {
		c *websocket.Conn
	}

	read := func() ([]byte, error) {
		_, b, err := c.Read(ctx)
		return b, err
	}

	var room *RoomHub
	var wsconn *WSConn
	var roomCode string
	var roomID string
	var role string
	var displayName string

	for {
		b, err := read()
		if err != nil {
			break
		}

		var env Envelope
		if err := json.Unmarshal(b, &env); err != nil {
			_ = c.Write(ctx, websocket.MessageText, Marshal(map[string]any{
				"type": "error", "payload": map[string]any{"message": "bad json"},
			}))
			continue
		}

		switch env.Type {

		case "room:join":
			var p JoinPayload
			if err := json.Unmarshal(env.Payload, &p); err != nil || p.Code == "" || p.DisplayName == "" || (p.Role != "host" && p.Role != "player") {
				_ = c.Write(ctx, websocket.MessageText, Marshal(map[string]any{
					"type": "error", "payload": map[string]any{"message": "invalid join payload"},
				}))
				continue
			}

			// Load room from DB
			dbCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			roomObj, err := h.Store.GetRoomByCode(dbCtx, p.Code)
			if err != nil {
				_ = c.Write(ctx, websocket.MessageText, Marshal(map[string]any{
					"type": "error", "payload": map[string]any{"message": "room not found"},
				}))
				continue
			}

			// Enforce: only the owner can join as host
			if p.Role == "host" && roomObj.OwnerUserID != userID {
				_ = c.Write(ctx, websocket.MessageText, Marshal(map[string]any{
					"type": "error", "payload": map[string]any{"message": "not host"},
				}))
				continue
			}

			// Upsert membership from WS (no HTTP join required)
			dbCtx, cancel = context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			err = h.Store.UpsertRoomMember(dbCtx, roomObj.ID, userID, p.DisplayName, p.Role)
			_ = h.Store.TouchRoomActivity(dbCtx, roomObj.ID) // best-effort
			if err != nil {
				_ = c.Write(ctx, websocket.MessageText, Marshal(map[string]any{
					"type": "error", "payload": map[string]any{"message": "failed to join room"},
				}))
				continue
			}

			// Save session vars for the connection
			roomCode = p.Code
			roomID = roomObj.ID
			role = p.Role
			displayName = p.DisplayName

			// Wrap connection with single-writer (only after successful join)
			wsconn = NewWSConn(ctx, c, userID, role, displayName)

			// Register in hub
			room = h.Hub.GetRoom(roomCode)
			room.Register(wsconn)

			// Refresh member states from DB (authoritative: displayName/role/score)
			dbCtx, cancel = context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			members, _ := h.Store.ListRoomMembers(dbCtx, roomID)

			// Update hub presence cache
			for _, m := range members {
				room.UpsertMemberState(MemberState{
					UserID:      m.UserID,
					DisplayName: m.DisplayName,
					Role:        m.Role,
					Score:       m.Score,
					Connected:   false, // set below
				})
			}

			// Mark connected for current hub conns
			room.mu.Lock()
			for uid, ms := range room.members {
				_, ok := room.conns[uid]
				ms.Connected = ok
				room.members[uid] = ms
			}
			room.mu.Unlock()

			// Reply joined
			_ = wsconn.Send(map[string]any{
				"type": "room:joined",
				"payload": map[string]any{
					"code":   roomCode,
					"roomId": roomID,
					"userId": userID,
					"role":   role,
				},
			})

			// Broadcast presence
			room.BroadcastPresence()

		case "host:start_round":
			if wsconn == nil || room == nil {
				_ = c.Write(ctx, websocket.MessageText, Marshal(map[string]any{
					"type": "error", "payload": map[string]any{"message": "must join first"},
				}))
				continue
			}
			if role != "host" {
				_ = wsconn.Send(map[string]any{"type": "error", "payload": map[string]any{"message": "host only"}})
				continue
			}

			var p StartRoundPayload
			if err := json.Unmarshal(env.Payload, &p); err != nil || p.Code == "" {
				_ = wsconn.Send(map[string]any{"type": "error", "payload": map[string]any{"message": "invalid payload"}})
				continue
			}
			lang := p.Lang
			if lang == "" {
				lang = "es"
			}

			// Determine currently connected players (exclude host)
			room.mu.Lock()
			var playerIDs []string
			for uid, c := range room.conns {
				if c.Role() == "player" {
					playerIDs = append(playerIDs, uid)
				}
			}
			room.mu.Unlock()

			dbCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()

			roundID, assigns, err := h.Store.StartRoundAssignCharacters(dbCtx, roomID, lang, playerIDs)
			if err != nil {
				msg := err.Error()
				_ = wsconn.Send(map[string]any{"type": "error", "payload": map[string]any{"message": msg}})
				continue
			}

			// Send each player all OTHER players' assignments (they need to guess their own)
			for _, currentPlayer := range assigns {
				// Build list of other players' assignments
				otherAssignments := make([]map[string]any, 0, len(assigns)-1)
				for _, a := range assigns {
					if a.UserID != currentPlayer.UserID {
						otherAssignments = append(otherAssignments, map[string]any{
							"userId": a.UserID,
							"character": map[string]any{
								"id":   a.Character.ID,
								"name": a.Character.Name,
							},
						})
					}
				}

				room.SendTo(currentPlayer.UserID, map[string]any{
					"type": "round:assigned",
					"payload": map[string]any{
						"roundId":     roundID,
						"assignments": otherAssignments,
					},
				})
			}

			// Broadcast presence again (not necessary, but useful to keep UI synced)
			room.BroadcastPresence()

		case "host:score_add":
			if wsconn == nil || room == nil {
				continue
			}
			if role != "host" {
				_ = wsconn.Send(map[string]any{"type": "error", "payload": map[string]any{"message": "host only"}})
				continue
			}

			var p ScoreAddPayload
			if err := json.Unmarshal(env.Payload, &p); err != nil || p.UserID == "" || p.Delta == 0 {
				_ = wsconn.Send(map[string]any{"type": "error", "payload": map[string]any{"message": "invalid payload"}})
				continue
			}

			dbCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()

			if err := h.Store.AddMemberScore(dbCtx, roomID, p.UserID, p.Delta); err != nil {
				_ = wsconn.Send(map[string]any{"type": "error", "payload": map[string]any{"message": "score update failed"}})
				continue
			}

			// Refresh member states from DB (simple + consistent)
			members, _ := h.Store.ListRoomMembersWithConnectionHint(dbCtx, roomID)
			for _, m := range members {
				room.UpsertMemberState(MemberState{
					UserID:      m.UserID,
					DisplayName: m.DisplayName,
					Role:        m.Role,
					Score:       m.Score,
					Connected:   true, // will be corrected below
				})
			}
			// Mark connected based on hub
			room.mu.Lock()
			for uid := range room.members {
				_, ok := room.conns[uid]
				room.members[uid] = MemberState{
					UserID:      room.members[uid].UserID,
					DisplayName: room.members[uid].DisplayName,
					Role:        room.members[uid].Role,
					Score:       room.members[uid].Score,
					Connected:   ok,
				}
			}
			room.mu.Unlock()

			room.BroadcastPresence()

		default:
			// ignore unknown
		}
	}

	// Disconnect cleanup
	if room != nil {
		room.Unregister(userID)
		room.SetConnected(userID, false)
		room.BroadcastPresence()
	}
	if wsconn != nil {
		_ = wsconn.Close()
	}

	c.Close(websocket.StatusNormalClosure, "bye")
	log.Info().Str("user", userID).Str("room", roomCode).Msg("ws disconnected")
}
