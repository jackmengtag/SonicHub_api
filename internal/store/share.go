package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type ShareLink struct {
	Code      string
	WorkID    string
	SharerID  string
	Status    string
	ExpiresAt *time.Time
}

func (s *Store) CreateShareLink(ctx context.Context, workID, sharerID, code string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO share_links(code, work_id, sharer_id, status)
		VALUES($1,$2,$3,'active')`, code, workID, sharerID)
	return err
}

func (s *Store) GetShareLink(ctx context.Context, code string) (*ShareLink, error) {
	var l ShareLink
	var expires sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT code, work_id, sharer_id, status, expires_at
		FROM share_links WHERE code=$1`, code).
		Scan(&l.Code, &l.WorkID, &l.SharerID, &l.Status, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	l.ExpiresAt = timePtr(expires)
	return &l, err
}

func (s *Store) AddShareEvent(ctx context.Context, code, channel, eventType, actorID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var workID string
	if err := tx.QueryRowContext(ctx, `SELECT work_id FROM share_links WHERE code=$1`, code).Scan(&workID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO share_events(id, code, channel, event_type, actor_id)
		VALUES($1,$2,$3,$4,$5)`, NewID("se_"), code, channel, eventType, actorID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE share_links SET open_count=open_count+1 WHERE code=$1`, code); err != nil {
		return err
	}
	if eventType == "share" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE works SET share_count=share_count+1 WHERE id=$1`, workID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
