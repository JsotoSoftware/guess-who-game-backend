package storage

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func (s *Storage) SetRoomPackSelectionBySlugs(ctx context.Context, roomID string, packSlugs []string) error {
	tx, err := s.PG.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if _, err = tx.Exec(ctx, `DELETE FROM room_pack_selection WHERE room_id=$1`, roomID); err != nil {
		return err
	}

	for _, slug := range packSlugs {
		var packID string
		if err = tx.QueryRow(ctx, `SELECT id FROM packs WHERE slug=$1 AND is_public=true`, slug).Scan(&packID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO room_pack_selection (room_id, pack_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, roomID, packID); err != nil {
			return err
		}
	}

	if _, err = tx.Exec(ctx, `UPDATE rooms SET last_activity_at=now() WHERE id=$1`, roomID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Storage) GetRoomSelectedPackSlugs(ctx context.Context, roomID string) ([]string, error) {
	rows, err := s.PG.Query(ctx, `
		SELECT p.slug
		FROM room_pack_selection rps
		JOIN packs p ON p.id = rps.pack_id
		WHERE rps.room_id = $1
		ORDER BY p.slug ASC
	`, roomID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, err
		}
		out = append(out, slug)
	}
	return out, rows.Err()
}
