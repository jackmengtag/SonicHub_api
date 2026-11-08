package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrDuplicate = errors.New("记录已存在")

/* ---------------- 推荐黑白名单 ---------------- */

type FeedListEntry struct {
	ID        string    `json:"id"`
	ListType  string    `json:"list_type"`
	Value     string    `json:"value"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) ListFeedLists(ctx context.Context, listType string) ([]FeedListEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, list_type, value, created_by, created_at
		FROM feed_lists WHERE ($1='' OR list_type=$1)
		ORDER BY list_type, created_at DESC`, listType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FeedListEntry{}
	for rows.Next() {
		var e FeedListEntry
		if err := rows.Scan(&e.ID, &e.ListType, &e.Value, &e.CreatedBy, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) CreateFeedList(ctx context.Context, actorID, listType, value string) (*FeedListEntry, error) {
	listType = strings.TrimSpace(listType)
	value = strings.TrimSpace(value)
	if listType != "black" && listType != "white" {
		return nil, ErrInvalidState
	}
	if value == "" {
		return nil, fmt.Errorf("%w: 名单内容不能为空", ErrInvalidState)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	id := NewID("fl_")
	res, err := tx.ExecContext(ctx, `
		INSERT INTO feed_lists(id, list_type, value, created_by)
		VALUES($1,$2,$3,$4) ON CONFLICT (list_type, value) DO NOTHING`,
		id, listType, value, actorID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrDuplicate
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'feed_list.create','feed_list',$3,$4)`,
		NewID("a_"), actorID, id, listType+":"+value); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &FeedListEntry{ID: id, ListType: listType, Value: value, CreatedBy: actorID, CreatedAt: time.Now()}, nil
}

func (s *Store) DeleteFeedList(ctx context.Context, actorID, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var listType, value string
	if err := tx.QueryRowContext(ctx, `
		SELECT list_type, value FROM feed_lists WHERE id=$1`, id).Scan(&listType, &value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM feed_lists WHERE id=$1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'feed_list.delete','feed_list',$3,$4)`,
		NewID("a_"), actorID, id, listType+":"+value); err != nil {
		return err
	}
	return tx.Commit()
}

/* ---------------- 推荐保底内容 ---------------- */

type FeedFallback struct {
	ID        string    `json:"id"`
	WorkID    string    `json:"work_id"`
	WorkTitle string    `json:"work_title"`
	WorkCover string    `json:"work_cover"`
	Author    string    `json:"author"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) ListFeedFallbacks(ctx context.Context) ([]FeedFallback, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT f.id, f.work_id, COALESCE(w.title,''), COALESCE(w.cover,''), COALESCE(u.display_name,''),
		       f.sort_order, f.created_at
		FROM feed_fallbacks f
		LEFT JOIN works w ON w.id=f.work_id
		LEFT JOIN users u ON u.id=w.user_id
		ORDER BY f.sort_order, f.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FeedFallback{}
	for rows.Next() {
		var f FeedFallback
		if err := rows.Scan(&f.ID, &f.WorkID, &f.WorkTitle, &f.WorkCover, &f.Author, &f.SortOrder, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) CreateFeedFallback(ctx context.Context, actorID, workID string) (*FeedFallback, error) {
	workID = strings.TrimSpace(workID)
	if workID == "" {
		return nil, fmt.Errorf("%w: 请选择作品", ErrInvalidState)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var status, visibility, title, cover, author string
	if err := tx.QueryRowContext(ctx, `
		SELECT w.status, w.visibility, w.title, w.cover, u.display_name
		FROM works w JOIN users u ON u.id=w.user_id WHERE w.id=$1`, workID).
		Scan(&status, &visibility, &title, &cover, &author); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if status != "published" || visibility != "public" {
		return nil, fmt.Errorf("%w: 只有已发布且公开的作品可以设为保底内容", ErrInvalidState)
	}

	var next int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(max(sort_order),0)+1 FROM feed_fallbacks`).Scan(&next); err != nil {
		return nil, err
	}
	id := NewID("fb_")
	res, err := tx.ExecContext(ctx, `
		INSERT INTO feed_fallbacks(id, work_id, sort_order, created_by)
		VALUES($1,$2,$3,$4) ON CONFLICT (work_id) DO NOTHING`,
		id, workID, next, actorID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrDuplicate
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'feed_fallback.create','work',$3,$4)`,
		NewID("a_"), actorID, workID, title); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &FeedFallback{
		ID: id, WorkID: workID, WorkTitle: title, WorkCover: cover, Author: author,
		SortOrder: next, CreatedAt: time.Now(),
	}, nil
}

func (s *Store) DeleteFeedFallback(ctx context.Context, actorID, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var workID string
	if err := tx.QueryRowContext(ctx, `
		SELECT work_id FROM feed_fallbacks WHERE id=$1`, id).Scan(&workID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM feed_fallbacks WHERE id=$1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'feed_fallback.delete','work',$3,'')`,
		NewID("a_"), actorID, workID); err != nil {
		return err
	}
	return tx.Commit()
}

/* ---------------- 角色调整 ---------------- */

func (s *Store) SetUserRole(ctx context.Context, userID, actorID, role string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var old string
	if err := tx.QueryRowContext(ctx, `SELECT role FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&old); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	if userID == actorID {
		return "", fmt.Errorf("%w: 不能修改当前登录账号的角色", ErrInvalidState)
	}
	if old == "admin" && role != "admin" {
		var others int
		if err := tx.QueryRowContext(ctx, `
			SELECT count(*) FROM users WHERE role='admin' AND id<>$1`, userID).Scan(&others); err != nil {
			return "", err
		}
		if others == 0 {
			return "", fmt.Errorf("%w: 至少保留一个超级管理员", ErrInvalidState)
		}
	}
	if old == role {
		return old, tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET role=$2 WHERE id=$1`, userID, role); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'user.role','user',$3,$4)`,
		NewID("a_"), actorID, userID, old+" -> "+role); err != nil {
		return "", err
	}
	return old, tx.Commit()
}
