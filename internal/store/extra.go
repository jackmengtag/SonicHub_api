package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jackmengtag/SonicHub_api/internal/model"
)

func (s *Store) RemoveWork(ctx context.Context, workID, userID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE works SET status='removed' WHERE id=$1 AND user_id=$2`, workID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SearchUsers(ctx context.Context, viewerID, q string, limit int) ([]model.UserCard, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	like := "%" + strings.TrimSpace(q) + "%"
	rows, err := s.db.QueryContext(ctx, userCardSelect+`
		WHERE (u.display_name ILIKE $2 OR COALESCE(m.stage_name,'') ILIKE $2)
		ORDER BY u.display_name LIMIT $3`, viewerID, like, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.UserCard{}
	for rows.Next() {
		c, err := scanUserCard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (s *Store) FollowingIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT followee_id FROM relations WHERE follower_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) CreateReport(ctx context.Context, reporterID, targetType, targetID, reason, detail string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO reports(id, reporter_id, target_type, target_id, reason, detail)
		VALUES($1,$2,$3,$4,$5,$6)`,
		NewID("r_"), reporterID, targetType, targetID, reason, detail)
	return err
}

func (s *Store) ConversationPeer(ctx context.Context, convID, userID string) (string, error) {
	var peer string
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id FROM conversation_members
		WHERE conversation_id=$1 AND user_id<>$2 LIMIT 1`, convID, userID).Scan(&peer)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return peer, err
}

func (s *Store) AddFeedback(ctx context.Context, userID, targetType, targetID, typ string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO feed_feedback(user_id, target_type, target_id, type)
		VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, userID, targetType, targetID, typ)
	return err
}
