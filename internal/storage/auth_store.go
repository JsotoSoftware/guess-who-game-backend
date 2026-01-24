package storage

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type RefreshSession struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
	RevokedAt *time.Time
}

func (s *Storage) CreateGuestUserAndSession(
	ctx context.Context,
	refreshTokenHash []byte,
	expiresAt time.Time,
	userAgent string,
	ip string,
) (userID string, sessionID string, err error) {

	tx, err := s.PG.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", "", err
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if err = tx.QueryRow(ctx, `INSERT INTO users DEFAULT VALUES RETURNING id`).Scan(&userID); err != nil {
		return "", "", err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO user_identities (user_id, provider, provider_id)
		VALUES ($1, 'guest', $2)
	`, userID, "guest:"+userID); err != nil {
		return "", "", err
	}

	if err = tx.QueryRow(ctx, `
		INSERT INTO refresh_sessions (user_id, token_hash, expires_at, user_agent, ip)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, userID, refreshTokenHash, expiresAt, userAgent, ip).Scan(&sessionID); err != nil {
		return "", "", err
	}

	if err = tx.Commit(ctx); err != nil {
		return "", "", err
	}

	return userID, sessionID, nil
}

func (s *Storage) GetRefreshSessionByHash(ctx context.Context, tokenHash []byte) (*RefreshSession, error) {
	var rs RefreshSession
	var revokedAt *time.Time

	err := s.PG.QueryRow(ctx, `
		SELECT id, user_id, expires_at, revoked_at
		FROM refresh_sessions
		WHERE token_hash = $1
	`, tokenHash).Scan(&rs.ID, &rs.UserID, &rs.ExpiresAt, &revokedAt)

	if err != nil {
		return nil, err
	}
	rs.RevokedAt = revokedAt
	return &rs, nil
}

func (s *Storage) RotateRefreshSession(
	ctx context.Context,
	oldHash []byte,
	newHash []byte,
	newExpiresAt time.Time,
	userAgent string,
	ip string,
) (newSessionID string, userID string, err error) {

	tx, err := s.PG.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var expiresAt time.Time
	var revokedAt *time.Time
	if err = tx.QueryRow(ctx, `
		SELECT user_id, expires_at, revoked_at
		FROM refresh_sessions
		WHERE token_hash = $1
		FOR UPDATE
	`, oldHash).Scan(&userID, &expiresAt, &revokedAt); err != nil {
		return "", "", err
	}

	now := time.Now().UTC()
	if revokedAt != nil || now.After(expiresAt) {
		return "", "", pgx.ErrNoRows
	}

	if _, err = tx.Exec(ctx, `
		UPDATE refresh_sessions
		SET revoked_at = now(), last_used_at = now()
		WHERE token_hash = $1
	`, oldHash); err != nil {
		return "", "", err
	}

	if err = tx.QueryRow(ctx, `
		INSERT INTO refresh_sessions (user_id, token_hash, expires_at, user_agent, ip, last_used_at)
		VALUES ($1, $2, $3, $4, $5, now())
		RETURNING id
	`, userID, newHash, newExpiresAt, userAgent, ip).Scan(&newSessionID); err != nil {
		return "", "", err
	}

	if err = tx.Commit(ctx); err != nil {
		return "", "", err
	}

	return newSessionID, userID, nil
}

func (s *Storage) RevokeRefreshSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.PG.Exec(ctx, `
		UPDATE refresh_sessions
		SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`, tokenHash)
	return err
}
