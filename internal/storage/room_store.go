package storage

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type Room struct {
	ID              string
	Code            string
	OwnerUserID     string
	Status          string
	CreatedAt       time.Time
	LastActivityAt  time.Time
	CurrentRoundID  *string
}

type RoomMember struct {
	UserID      string
	DisplayName string
	Role        string
	Score       int
	JoinedAt    time.Time
}

func (s *Storage) CreateRoom(ctx context.Context, ownerUserID string, code string) (*Room, error) {
	var r Room
	err := s.PG.QueryRow(ctx, `
		INSERT INTO rooms (code, owner_user_id, last_activity_at)
		VALUES ($1, $2, now())
		RETURNING id, code, owner_user_id, status, created_at, last_activity_at
	`, code, ownerUserID).Scan(
		&r.ID, &r.Code, &r.OwnerUserID, &r.Status, &r.CreatedAt, &r.LastActivityAt,
	)
	if err != nil {
		return nil, err
	}

	_, err = s.PG.Exec(ctx, `
		INSERT INTO room_members (room_id, user_id, display_name, role, score)
		VALUES ($1, $2, $3, 'host', 0)
		ON CONFLICT (room_id, user_id) DO NOTHING
	`, r.ID, ownerUserID, "Anfitrión")
	if err != nil {
		return nil, err
	}

	return &r, nil
}

func (s *Storage) GetRoomByCode(ctx context.Context, code string) (*Room, error) {
	var r Room
	err := s.PG.QueryRow(ctx, `
		SELECT id, code, owner_user_id, status, created_at, last_activity_at, current_round_id
		FROM rooms
		WHERE code = $1
	`, code).Scan(&r.ID, &r.Code, &r.OwnerUserID, &r.Status, &r.CreatedAt, &r.LastActivityAt, &r.CurrentRoundID)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Storage) TouchRoomActivity(ctx context.Context, roomID string) error {
	_, err := s.PG.Exec(ctx, `UPDATE rooms SET last_activity_at = now() WHERE id = $1`, roomID)
	return err
}

func (s *Storage) JoinRoom(ctx context.Context, roomID string, userID string, displayName string) error {
	tx, err := s.PG.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// ensure room exists and is active
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM rooms WHERE id=$1 FOR UPDATE`, roomID).Scan(&status); err != nil {
		return err
	}
	if status != "active" {
		return pgx.ErrNoRows
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO room_members (room_id, user_id, display_name, role, score)
		VALUES ($1, $2, $3, 'player', 0)
		ON CONFLICT (room_id, user_id) DO UPDATE SET display_name = EXCLUDED.display_name
	`, roomID, userID, displayName)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `UPDATE rooms SET last_activity_at = now() WHERE id=$1`, roomID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Storage) ListRoomMembers(ctx context.Context, roomID string) ([]RoomMember, error) {
	rows, err := s.PG.Query(ctx, `
		SELECT user_id, display_name, role, score, joined_at
		FROM room_members
		WHERE room_id = $1
		ORDER BY joined_at ASC
	`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RoomMember
	for rows.Next() {
		var m RoomMember
		if err := rows.Scan(&m.UserID, &m.DisplayName, &m.Role, &m.Score, &m.JoinedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Storage) UpsertRoomMember(ctx context.Context, roomID, userID, displayName, role string) error {
	_, err := s.PG.Exec(ctx, `
		INSERT INTO room_members (room_id, user_id, display_name, role, score)
		VALUES ($1, $2, $3, $4, 0)
		ON CONFLICT (room_id, user_id) DO UPDATE
		SET display_name = EXCLUDED.display_name,
		    role = EXCLUDED.role
	`, roomID, userID, displayName, role)
	return err
}
