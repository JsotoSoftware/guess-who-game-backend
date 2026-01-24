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
		log.Warn().Str("user", userID).Err(err).Msg("ws: failed to accept connection")
		return
	}
	defer c.Close(websocket.StatusInternalError, "server error")

	log.Info().Str("user", userID).Msg("ws: connection established")

	readCtx := r.Context()
	ctx := r.Context()

	type tmpConn struct {
		c *websocket.Conn
	}

	read := func() ([]byte, error) {
		_, b, err := c.Read(readCtx)
		return b, err
	}

	var room *RoomHub
	var wsconn *WSConn
	var roomCode string
	var roomID string
	var role string
	var displayName string
	var dbCtx context.Context
	var cancel context.CancelFunc

	for {
		b, err := read()
		if err != nil {
			log.Info().
				Str("user", userID).
				Str("room", roomCode).
				Err(err).
				Msg("ws: read error, closing connection")
			break
		}

		log.Debug().Str("user", userID).Int("bytes", len(b)).Str("raw", string(b)).Msg("ws: raw message received")

		var env Envelope
		if err := json.Unmarshal(b, &env); err != nil {
			log.Warn().Str("user", userID).Str("room", roomCode).Err(err).Str("raw", string(b)).Msg("ws: received invalid JSON")
			_ = c.Write(ctx, websocket.MessageText, Marshal(map[string]any{
				"type": "error", "payload": map[string]any{"message": "bad json"},
			}))
			continue
		}

		log.Info().
			Str("user", userID).
			Str("room", roomCode).
			Str("type", env.Type).
			Int("payloadLen", len(env.Payload)).
			Msg("ws: message received")

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
			dbCtx, cancel = context.WithTimeout(ctx, 3*time.Second)
			roomObj, err := h.Store.GetRoomByCode(dbCtx, p.Code)
			cancel()
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

			dbCtx, cancel = context.WithTimeout(ctx, 3*time.Second)
			err = h.Store.UpsertRoomMember(dbCtx, roomObj.ID, userID, p.DisplayName, p.Role)
			_ = h.Store.TouchRoomActivity(dbCtx, roomObj.ID)
			cancel()
			if err != nil {
				_ = c.Write(ctx, websocket.MessageText, Marshal(map[string]any{
					"type": "error", "payload": map[string]any{"message": "failed to join room"},
				}))
				continue
			}

			roomCode = p.Code
			roomID = roomObj.ID
			role = p.Role
			displayName = p.DisplayName

			wsconn = NewWSConn(c, userID, role, displayName)

			room = h.Hub.GetRoom(roomCode)
			room.Register(wsconn)

			dbCtx, cancel = context.WithTimeout(ctx, 3*time.Second)
			members, _ := h.Store.ListRoomMembers(dbCtx, roomID)
			cancel()

			for _, m := range members {
				room.UpsertMemberState(MemberState{
					UserID:      m.UserID,
					DisplayName: m.DisplayName,
					Role:        m.Role,
					Score:       m.Score,
					Connected:   false,
				})
			}

			room.mu.Lock()
			for uid, ms := range room.members {
				_, ok := room.conns[uid]
				ms.Connected = ok
				room.members[uid] = ms
			}
			room.mu.Unlock()

			_ = wsconn.Send(map[string]any{
				"type": "room:joined",
				"payload": map[string]any{
					"code":   roomCode,
					"roomId": roomID,
					"userId": userID,
					"role":   role,
				},
			})

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
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				_ = wsconn.Send(map[string]any{"type": "error", "payload": map[string]any{"message": "invalid payload: " + err.Error()}})
				continue
			}
			if p.Code == "" {
				_ = wsconn.Send(map[string]any{"type": "error", "payload": map[string]any{"message": "code is required"}})
				continue
			}
			lang := p.Lang
			if lang == "" {
				lang = "es"
			}

			room.mu.Lock()
			playerIDs := make([]string, 0, len(room.conns))
			for uid := range room.conns {
				playerIDs = append(playerIDs, uid)
			}
			room.mu.Unlock()

			if len(playerIDs) == 0 {
				_ = wsconn.Send(map[string]any{"type": "error", "payload": map[string]any{"message": "no players connected"}})
				continue
			}

			dbCtx, cancel = context.WithTimeout(ctx, 8*time.Second)
			roundID, assigns, err := h.Store.StartRoundAssignCharacters(dbCtx, roomID, lang, playerIDs)
			cancel()
			if err != nil {
				_ = wsconn.Send(map[string]any{"type": "error", "payload": map[string]any{"message": "failed to start round: " + err.Error()}})
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

			_ = wsconn.Send(map[string]any{
				"type": "host:round_started",
				"payload": map[string]any{
					"roundId":     roundID,
					"playerCount": len(assigns),
					"lang":        lang,
				},
			})

			room.BroadcastPresence()

		case "host:score_add":
			if wsconn == nil || room == nil {
				_ = c.Write(ctx, websocket.MessageText, Marshal(map[string]any{
					"type": "error", "payload": map[string]any{"message": "must join first"},
				}))
				continue
			}
			if role != "host" {
				_ = c.Write(ctx, websocket.MessageText, Marshal(map[string]any{
					"type": "error", "payload": map[string]any{"message": "host only"},
				}))
				continue
			}

			var p ScoreAddPayload
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				_ = c.Write(ctx, websocket.MessageText, Marshal(map[string]any{
					"type": "error", "payload": map[string]any{"message": "invalid payload: " + err.Error()},
				}))
				continue
			}
			if p.UserID == "" || p.Delta == 0 {
				_ = c.Write(ctx, websocket.MessageText, Marshal(map[string]any{
					"type": "error", "payload": map[string]any{"message": "userId and delta required, delta must be non-zero"},
				}))
				continue
			}

			dbCtx, cancel = context.WithTimeout(ctx, 3*time.Second)
			if err := h.Store.AddMemberScore(dbCtx, roomID, p.UserID, p.Delta); err != nil {
				cancel()
				_ = c.Write(ctx, websocket.MessageText, Marshal(map[string]any{
					"type": "error", "payload": map[string]any{"message": "score update failed: " + err.Error()},
				}))
				continue
			}
			members, _ := h.Store.ListRoomMembersWithConnectionHint(dbCtx, roomID)
			cancel()
			for _, m := range members {
				room.UpsertMemberState(MemberState{
					UserID:      m.UserID,
					DisplayName: m.DisplayName,
					Role:        m.Role,
					Score:       m.Score,
					Connected:   true,
				})
			}
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

			_ = wsconn.Send(map[string]any{
				"type": "host:score_added",
				"payload": map[string]any{
					"userId": p.UserID,
					"delta":  p.Delta,
				},
			})

			room.BroadcastPresence()

		case "client:ping":
			if wsconn != nil {
				_ = wsconn.Send(map[string]any{"type": "server:pong", "payload": map[string]any{"ts": time.Now().UnixMilli()}})
			} else {
				_ = c.Write(ctx, websocket.MessageText, Marshal(map[string]any{"type": "server:pong"}))
			}

		default:
			log.Debug().Str("user", userID).Str("type", env.Type).Msg("ws: unknown message type, ignoring")
		}

		if wsconn != nil {
			wsconn.Touch()
		}
	}

	if room != nil {
		room.Unregister(userID)
		room.SetConnected(userID, false)
		room.BroadcastPresence()
	}
	if wsconn != nil {
		_ = wsconn.Close()
	}

	c.Close(websocket.StatusNormalClosure, "bye")
	log.Info().Str("user", userID).Str("room", roomCode).Msg("ws: connection closed")
}
