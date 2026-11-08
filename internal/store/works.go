package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackmengtag/SonicHub_api/internal/model"
)

const workColumns = `
	w.id, w.title, w.cover, w.description, w.visibility, w.status, w.reject_reason,
	w.genre, w.tags, w.audio_url, w.duration_seconds, w.play_count, w.like_count,
	w.comment_count, w.share_count, w.is_ai,
	u.id, u.display_name, u.avatar, COALESCE(m.status='verified', false),
	EXISTS(SELECT 1 FROM work_likes wl WHERE wl.work_id=w.id AND wl.user_id=$1),
	EXISTS(SELECT 1 FROM work_collects wc WHERE wc.work_id=w.id AND wc.user_id=$1),
	w.published_at, w.created_at`

const workFrom = `
	FROM works w
	JOIN users u ON u.id=w.user_id
	LEFT JOIN musicians m ON m.user_id=u.id`

const visibleClause = `
	AND (w.status='published' OR w.user_id=$1)
	AND (
		w.visibility='public' OR w.user_id=$1 OR
		(w.visibility='friends' AND EXISTS(
			SELECT 1 FROM relations r1
			JOIN relations r2 ON r2.follower_id=r1.followee_id AND r2.followee_id=r1.follower_id
			WHERE r1.follower_id=$1 AND r1.followee_id=w.user_id))
	)
	AND NOT EXISTS(
		SELECT 1 FROM blocks b
		WHERE (b.user_id=$1 AND b.blocked_user_id=w.user_id)
		   OR (b.user_id=w.user_id AND b.blocked_user_id=$1)
	)`

func scanWork(row interface{ Scan(...any) error }) (*model.Work, error) {
	var w model.Work
	var tags []byte
	var published sql.NullTime
	err := row.Scan(
		&w.ID, &w.Title, &w.Cover, &w.Description, &w.Visibility, &w.Status, &w.RejectReason,
		&w.Genre, &tags, &w.AudioURL, &w.Duration, &w.PlayCount, &w.LikeCount,
		&w.CommentCount, &w.ShareCount, &w.IsAI,
		&w.OwnerID, &w.OwnerName, &w.OwnerAvatar, &w.OwnerVerified,
		&w.Liked, &w.Collected, &published, &w.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	w.Tags = decodeStrings(tags)
	w.PublishedAt = timePtr(published)
	return &w, nil
}

func (s *Store) GetWork(ctx context.Context, workID, viewerID string) (*model.Work, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+workColumns+workFrom+` WHERE w.id=$2 `+visibleClause, viewerID, workID)
	return scanWork(row)
}

func (s *Store) GetWorkAny(ctx context.Context, workID string) (*model.Work, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+workColumns+workFrom+` WHERE w.id=$2`, "", workID)
	return scanWork(row)
}

type WorkFilter struct {
	ViewerID string
	OwnerID  string
	Status   string
	Genre    string
	Sort     string
	Limit    int
	Offset   int
	Query    string
}

func (s *Store) ListWorks(ctx context.Context, f WorkFilter) ([]model.Work, error) {
	if f.Limit <= 0 || f.Limit > 50 {
		f.Limit = 20
	}
	args := []any{f.ViewerID}
	where := []string{"1=1"}
	add := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	isOwner := f.OwnerID != "" && f.OwnerID == f.ViewerID
	if f.OwnerID != "" {
		where = append(where, "w.user_id="+add(f.OwnerID))
	}
	if f.Status != "" {
		if !isOwner && f.Status != "published" {
			where = append(where, "w.status='published'")
		} else {
			where = append(where, "w.status="+add(f.Status))
		}
	} else if !isOwner {
		where = append(where, "w.status='published'")
	}
	if f.Genre != "" {
		where = append(where, "w.genre="+add(f.Genre))
	}
	if f.Query != "" {
		q := add("%" + f.Query + "%")
		where = append(where, "(w.title ILIKE "+q+" OR u.display_name ILIKE "+q+" OR w.genre ILIKE "+q+")")
	}
	// Visibility / block rules apply whenever the viewer is not the owner.
	if !isOwner {
		where = append(where, `(w.visibility='public' OR (w.visibility='friends' AND EXISTS(
			SELECT 1 FROM relations r1
			JOIN relations r2 ON r2.follower_id=r1.followee_id AND r2.followee_id=r1.follower_id
			WHERE r1.follower_id=`+add(f.ViewerID)+` AND r1.followee_id=w.user_id)))`)
		where = append(where, `NOT EXISTS(
			SELECT 1 FROM blocks b
			WHERE (b.user_id=`+add(f.ViewerID)+` AND b.blocked_user_id=w.user_id)
			   OR (b.user_id=w.user_id AND b.blocked_user_id=`+add(f.ViewerID)+`))`)
		where = append(where, `NOT EXISTS(
			SELECT 1 FROM feed_feedback f
			WHERE f.user_id=`+add(f.ViewerID)+` AND f.target_type='work' AND f.target_id=w.id)`)
		where = append(where, `NOT EXISTS(
			SELECT 1 FROM feed_feedback f
			WHERE f.user_id=`+add(f.ViewerID)+` AND f.target_type='author' AND f.target_id=w.user_id)`)
		// 推荐黑名单：支持作品 ID、作者 ID、风格、标题/简介与标签关键词。
		where = append(where, `NOT EXISTS(
			SELECT 1 FROM feed_lists fl
			WHERE fl.list_type='black' AND (
				fl.value = w.id OR fl.value = w.genre OR fl.value = w.user_id
				OR w.title ILIKE '%'||fl.value||'%'
				OR w.description ILIKE '%'||fl.value||'%'
				OR EXISTS (SELECT 1 FROM jsonb_array_elements_text(w.tags) tg WHERE tg ILIKE '%'||fl.value||'%')
			))`)
	}
	order := "ORDER BY COALESCE(w.published_at, w.created_at) DESC"
	if f.Sort == "hot" {
		order = `ORDER BY (EXISTS(
			SELECT 1 FROM feed_lists fl
			WHERE fl.list_type='white' AND (fl.value = w.id OR fl.value = w.genre OR fl.value = w.user_id)
		)) DESC, (
			w.like_count*3 + w.comment_count*4 + w.share_count*5 + w.play_count*0.02
		) / (1 + EXTRACT(EPOCH FROM (now()-COALESCE(w.published_at,w.created_at)))/86400.0) DESC`
	}
	q := `SELECT ` + workColumns + workFrom + ` WHERE ` + strings.Join(where, " AND ") + ` ` + order +
		` LIMIT ` + add(f.Limit) + ` OFFSET ` + add(f.Offset)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Work{}
	for rows.Next() {
		w, err := scanWork(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

// FallbackWorks 返回推荐流不足时使用的保底作品，按运营配置顺序。
func (s *Store) FallbackWorks(ctx context.Context, viewerID string, excludeIDs []string, limit int) ([]model.Work, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	args := []any{viewerID}
	exclude := ""
	if len(excludeIDs) > 0 {
		ph := make([]string, 0, len(excludeIDs))
		for _, id := range excludeIDs {
			args = append(args, id)
			ph = append(ph, fmt.Sprintf("$%d", len(args)))
		}
		exclude = " AND w.id NOT IN (" + strings.Join(ph, ",") + ")"
	}
	args = append(args, limit)
	q := `SELECT ` + workColumns + workFrom + `
		JOIN feed_fallbacks fb ON fb.work_id=w.id
		WHERE w.status='published' AND w.visibility='public'
		  AND NOT EXISTS(
			SELECT 1 FROM blocks b
			WHERE (b.user_id=$1 AND b.blocked_user_id=w.user_id)
			   OR (b.user_id=w.user_id AND b.blocked_user_id=$1))
		  AND NOT EXISTS(
			SELECT 1 FROM feed_lists fl
			WHERE fl.list_type='black' AND (
				fl.value = w.id OR fl.value = w.genre OR fl.value = w.user_id
				OR w.title ILIKE '%'||fl.value||'%'
				OR w.description ILIKE '%'||fl.value||'%'
				OR EXISTS (SELECT 1 FROM jsonb_array_elements_text(w.tags) tg WHERE tg ILIKE '%'||fl.value||'%')
			))
		  AND NOT EXISTS(
			SELECT 1 FROM feed_feedback f
			WHERE f.user_id=$1 AND f.target_type='work' AND f.target_id=w.id)
		  AND NOT EXISTS(
			SELECT 1 FROM feed_feedback f
			WHERE f.user_id=$1 AND f.target_type='author' AND f.target_id=w.user_id)` + exclude + `
		ORDER BY fb.sort_order, fb.created_at
		LIMIT ` + fmt.Sprintf("$%d", len(args))
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Work{}
	for rows.Next() {
		w, err := scanWork(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

type CreateWorkInput struct {
	TenantID     string
	UserID       string
	Title        string
	Cover        string
	Description  string
	Visibility   string
	Status       string
	RejectReason string
	Genre        string
	Tags         []string
	AudioURL     string
	Duration     int
	IsAI         bool
	GenerationID string
	PublishedAt  *time.Time
}

func (s *Store) CreateWork(ctx context.Context, in CreateWorkInput) (string, error) {
	if in.Cover == "" {
		in.Cover = "c1"
	}
	if in.Visibility == "" {
		in.Visibility = "public"
	}
	if in.Status == "" {
		in.Status = "draft"
	}
	tags, _ := marshalStrings(in.Tags)
	id := NewID("w_")
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO works(id, tenant_id, user_id, title, cover, description, visibility, status,
			reject_reason, duration_seconds, genre, tags, audio_url, is_ai, generation_id, published_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13,$14,NULLIF($15,''),$16)`,
		id, in.TenantID, in.UserID, in.Title, in.Cover, in.Description, in.Visibility, in.Status,
		in.RejectReason, in.Duration, in.Genre, tags, in.AudioURL, in.IsAI, in.GenerationID, in.PublishedAt)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) PublishWork(ctx context.Context, workID, userID, title, description, visibility string, tags []string, status, rejectReason string) error {
	tagJSON, _ := marshalStrings(tags)
	res, err := s.db.ExecContext(ctx, `
		UPDATE works SET title=$3, description=$4, visibility=$5, tags=$6::jsonb,
			status=$7, reject_reason=$8,
			published_at=CASE WHEN $7='published' THEN now() ELSE published_at END
		WHERE id=$1 AND user_id=$2`,
		workID, userID, title, description, visibility, tagJSON, status, rejectReason)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetVisibility(ctx context.Context, workID, userID, visibility string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE works SET visibility=$3 WHERE id=$1 AND user_id=$2`, workID, userID, visibility)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) IncrementPlay(ctx context.Context, workID string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE works SET play_count=play_count+1
		WHERE id=$1 AND status='published' AND visibility='public'`, workID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Like(ctx context.Context, workID, userID string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO work_likes(work_id, user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, workID, userID)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE works SET like_count=like_count+1 WHERE id=$1`, workID); err != nil {
			return 0, err
		}
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT like_count FROM works WHERE id=$1`, workID).Scan(&count); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) Unlike(ctx context.Context, workID, userID string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `DELETE FROM work_likes WHERE work_id=$1 AND user_id=$2`, workID, userID)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE works SET like_count=GREATEST(like_count-1,0) WHERE id=$1`, workID); err != nil {
			return 0, err
		}
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT like_count FROM works WHERE id=$1`, workID).Scan(&count); err != nil {
		return 0, err
	}
	return count, tx.Commit()
}

func (s *Store) Collect(ctx context.Context, workID, userID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO work_collects(work_id, user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, workID, userID)
	return err
}

func (s *Store) Uncollect(ctx context.Context, workID, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM work_collects WHERE work_id=$1 AND user_id=$2`, workID, userID)
	return err
}

func (s *Store) AddComment(ctx context.Context, workID, userID, content string) (*model.Comment, error) {
	id := NewID("c_")
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO comments(id, work_id, user_id, content) VALUES($1,$2,$3,$4)`,
		id, workID, userID, content); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE works SET comment_count=comment_count+1 WHERE id=$1`, workID); err != nil {
		return nil, err
	}
	var c model.Comment
	if err := tx.QueryRowContext(ctx, `
		SELECT c.id, c.work_id, c.user_id, u.display_name, u.avatar, c.content, c.created_at
		FROM comments c JOIN users u ON u.id=c.user_id WHERE c.id=$1`, id).
		Scan(&c.ID, &c.WorkID, &c.UserID, &c.Name, &c.Avatar, &c.Content, &c.CreatedAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) ListComments(ctx context.Context, workID string, limit int) ([]model.Comment, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.work_id, c.user_id, u.display_name, u.avatar, c.content, c.created_at
		FROM comments c JOIN users u ON u.id=c.user_id
		WHERE c.work_id=$1 AND c.status='published'
		ORDER BY c.created_at DESC LIMIT $2`, workID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Comment{}
	for rows.Next() {
		var c model.Comment
		if err := rows.Scan(&c.ID, &c.WorkID, &c.UserID, &c.Name, &c.Avatar, &c.Content, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) DeleteComment(ctx context.Context, commentID, userID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var workID string
	if err := tx.QueryRowContext(ctx, `
		DELETE FROM comments WHERE id=$1 AND user_id=$2 RETURNING work_id`, commentID, userID).Scan(&workID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE works SET comment_count=GREATEST(comment_count-1,0) WHERE id=$1`, workID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) WorkOwner(ctx context.Context, workID string) (string, error) {
	var owner string
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM works WHERE id=$1`, workID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return owner, err
}

func marshalStrings(v []string) (string, error) {
	if v == nil {
		v = []string{}
	}
	b, err := json.Marshal(v)
	return string(b), err
}
