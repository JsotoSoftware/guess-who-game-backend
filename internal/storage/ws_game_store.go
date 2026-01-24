package storage

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrNotHost             = errors.New("not host")
	ErrNoPackSelection     = errors.New("no packs selected for room")
	ErrNotEnoughCharacters = errors.New("not enough characters available")
	ErrRoundAlreadyActive  = errors.New("current round must be ended before starting a new one")
)

func (s *Storage) AddMemberScore(ctx context.Context, roomID, userID string, delta int) error {
	_, err := s.PG.Exec(ctx, `
		UPDATE room_members
		SET score = score + $3
		WHERE room_id=$1 AND user_id=$2
	`, roomID, userID, delta)
	if err != nil {
		return err
	}
	_, _ = s.PG.Exec(ctx, `UPDATE rooms SET last_activity_at=now() WHERE id=$1`, roomID)
	return nil
}

type AssignedCharacter struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RoundAssignment struct {
	UserID    string
	Character AssignedCharacter
}

func (s *Storage) StartRoundAssignCharacters(
	ctx context.Context,
	roomID string,
	lang string,
	playerUserIDs []string,
) (roundID string, assignments []RoundAssignment, err error) {

	if len(playerUserIDs) == 0 {
		return "", nil, errors.New("no players to assign")
	}

	packRows, err := s.PG.Query(ctx, `
		SELECT pack_id
		FROM room_pack_selection
		WHERE room_id=$1
	`, roomID)
	if err != nil {
		return "", nil, err
	}
	defer packRows.Close()

	var packIDs []string
	for packRows.Next() {
		var id string
		if err := packRows.Scan(&id); err != nil {
			return "", nil, err
		}
		packIDs = append(packIDs, id)
	}
	if err := packRows.Err(); err != nil {
		return "", nil, err
	}
	if len(packIDs) == 0 {
		return "", nil, ErrNoPackSelection
	}

	tx, err := s.PG.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var cur *string
	if err = tx.QueryRow(ctx, `SELECT current_round_id FROM rooms WHERE id=$1 FOR UPDATE`, roomID).Scan(&cur); err != nil {
		return "", nil, err
	}
	if cur != nil && *cur != "" {
		return "", nil, ErrRoundAlreadyActive
	}

	if err = tx.QueryRow(ctx, `
		INSERT INTO room_rounds (room_id, started_at, lang)
		VALUES ($1, now(), $2)
		RETURNING id
	`, roomID, lang).Scan(&roundID); err != nil {
		return "", nil, err
	}

	need := len(playerUserIDs)

	type pick struct {
		id   string
		name string
	}

	rows, err := tx.Query(ctx, `
		SELECT
			c.id,
			COALESCE(ct_req.name, ct_es.name, ct_en.name, c.canonical_key) AS name
		FROM characters c
		LEFT JOIN character_translations ct_req ON ct_req.character_id = c.id AND ct_req.lang = $3
		LEFT JOIN character_translations ct_es  ON ct_es.character_id  = c.id AND ct_es.lang  = 'es'
		LEFT JOIN character_translations ct_en  ON ct_en.character_id  = c.id AND ct_en.lang  = 'en'
		WHERE c.pack_id = ANY($1)
		  AND NOT EXISTS (
			SELECT 1 FROM room_used_characters u
			WHERE u.room_id = $2 AND u.character_id = c.id
		  )
		ORDER BY random()
		LIMIT $4
		FOR UPDATE OF c SKIP LOCKED
	`, packIDs, roomID, lang, need)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()

	var picked []pick
	for rows.Next() {
		var p pick
		if err := rows.Scan(&p.id, &p.name); err != nil {
			return "", nil, err
		}
		picked = append(picked, p)
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}

	if len(picked) < need {
		return "", nil, ErrNotEnoughCharacters
	}

	for _, p := range picked {
		if _, err = tx.Exec(ctx, `
			INSERT INTO room_used_characters (room_id, character_id, first_used_at)
			VALUES ($1, $2, now())
		`, roomID, p.id); err != nil {
			return "", nil, err
		}
	}

	assignments = make([]RoundAssignment, 0, need)
	for i := 0; i < need; i++ {
		userID := playerUserIDs[i]
		ch := picked[i]

		if _, err = tx.Exec(ctx, `
			INSERT INTO round_assignments (round_id, user_id, character_id, assigned_at)
			VALUES ($1, $2, $3, now())
		`, roundID, userID, ch.id); err != nil {
			return "", nil, err
		}

		assignments = append(assignments, RoundAssignment{
			UserID: userID,
			Character: AssignedCharacter{
				ID:   ch.id,
				Name: ch.name,
			},
		})
	}

	_, _ = tx.Exec(ctx, `UPDATE rooms SET current_round_id=$2, last_activity_at=now() WHERE id=$1`, roomID, roundID)

	if err = tx.Commit(ctx); err != nil {
		return "", nil, err
	}

	return roundID, assignments, nil
}

func (s *Storage) EndRound(ctx context.Context, roomID string) error {
	tx, err := s.PG.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var cur *string
	if err = tx.QueryRow(ctx, `SELECT current_round_id FROM rooms WHERE id=$1 FOR UPDATE`, roomID).Scan(&cur); err != nil {
		return err
	}
	if cur == nil || *cur == "" {
		return tx.Commit(ctx)
	}

	if _, err = tx.Exec(ctx, `UPDATE room_rounds SET ended_at=now() WHERE id=$1`, *cur); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE rooms SET current_round_id=NULL WHERE id=$1`, roomID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Storage) TouchUser(ctx context.Context, userID string) {
	_, _ = s.PG.Exec(ctx, `UPDATE users SET last_seen_at=now() WHERE id=$1`, userID)
}

func (s *Storage) TouchRoom(ctx context.Context, roomID string) {
	_, _ = s.PG.Exec(ctx, `UPDATE rooms SET last_activity_at=now() WHERE id=$1`, roomID)
}

func (s *Storage) ListRoomMembersWithConnectionHint(
	ctx context.Context,
	roomID string,
) ([]RoomMember, error) {
	return s.ListRoomMembers(ctx, roomID)
}

func NowUTC() time.Time { return time.Now().UTC() }
