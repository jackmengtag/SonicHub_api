package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackmengtag/SonicHub_api/internal/model"
)

type AdminStats struct {
	Pending   int `json:"pending"`
	Reports   int `json:"reports"`
	Published int `json:"published_today"`
	Users     int `json:"users"`
}

func (s *Store) AdminStats(ctx context.Context) (AdminStats, error) {
	var x AdminStats
	err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM works WHERE status='pending_review'),
			(SELECT count(*) FROM reports WHERE status='open'),
			(SELECT count(*) FROM works WHERE status='published' AND published_at >= current_date),
			(SELECT count(*) FROM users)`).
		Scan(&x.Pending, &x.Reports, &x.Published, &x.Users)
	return x, err
}

func (s *Store) PendingWorks(ctx context.Context, limit int) ([]model.ModerationItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT w.id, w.title, u.display_name, w.description, w.cover, w.genre, w.duration_seconds, w.visibility, w.created_at
		FROM works w JOIN users u ON u.id=w.user_id
		WHERE w.status='pending_review'
		ORDER BY w.created_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ModerationItem{}
	for rows.Next() {
		var x model.ModerationItem
		x.TargetType = "work"
		x.Decision = "pending"
		x.Status = "open"
		if err := rows.Scan(&x.TargetID, &x.Title, &x.Author, &x.Reason, &x.Cover, &x.Genre, &x.Duration, &x.Visibility, &x.CreatedAt); err != nil {
			return nil, err
		}
		x.ID = x.TargetID
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ListReports(ctx context.Context, limit int) ([]model.Report, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, r.target_type, r.target_id, COALESCE(w.title,''), u.display_name, r.reason, r.detail, r.status, r.created_at
		FROM reports r LEFT JOIN users u ON u.id=r.reporter_id
		LEFT JOIN works w ON r.target_type='work' AND w.id=r.target_id
		ORDER BY r.created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Report{}
	for rows.Next() {
		var r model.Report
		if err := rows.Scan(&r.ID, &r.TargetType, &r.TargetID, &r.TargetTitle, &r.Reporter, &r.Reason, &r.Detail, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) ListApplications(ctx context.Context, limit int) ([]model.ModerationItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.user_id, u.display_name, m.stage_name, m.bio, m.applied_at
		FROM musicians m JOIN users u ON u.id=m.user_id
		WHERE m.status='pending'
		ORDER BY m.applied_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ModerationItem{}
	for rows.Next() {
		var x model.ModerationItem
		var stage, bio string
		x.TargetType = "musician"
		x.Decision = "pending"
		x.Status = "open"
		if err := rows.Scan(&x.TargetID, &x.Author, &stage, &bio, &x.CreatedAt); err != nil {
			return nil, err
		}
		x.ID = x.TargetID
		x.Title = stage
		x.Reason = bio
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ListAppeals(ctx context.Context, limit int) ([]model.Appeal, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.work_id, COALESCE(w.title,''), a.user_id, a.reason, a.status, a.created_at
		FROM appeals a LEFT JOIN works w ON w.id=a.work_id
		ORDER BY a.created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Appeal{}
	for rows.Next() {
		var a model.Appeal
		if err := rows.Scan(&a.ID, &a.WorkID, &a.WorkTitle, &a.UserID, &a.Reason, &a.Status, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) MyAppeals(ctx context.Context, userID string) ([]model.Appeal, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.work_id, COALESCE(w.title,''), a.user_id, a.reason, a.status, a.created_at
		FROM appeals a LEFT JOIN works w ON w.id=a.work_id
		WHERE a.user_id=$1 ORDER BY a.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Appeal{}
	for rows.Next() {
		var a model.Appeal
		if err := rows.Scan(&a.ID, &a.WorkID, &a.WorkTitle, &a.UserID, &a.Reason, &a.Status, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) DecideWork(ctx context.Context, workID, reviewer, decision, reason string) (string, error) {
	status := "published"
	if decision == "reject" {
		status = "rejected"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var owner string
	res, err := tx.ExecContext(ctx, `
		UPDATE works SET status=$2, reject_reason=$3,
			published_at=CASE WHEN $2='published' THEN now() ELSE published_at END
		WHERE id=$1`, workID, status, reason)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", ErrNotFound
	}
	if err := tx.QueryRowContext(ctx, `SELECT user_id FROM works WHERE id=$1`, workID).Scan(&owner); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO moderation_cases(id, target_type, target_id, decision, reason, status, reviewer_id, reviewed_at)
		VALUES($1,'work',$2,$3,$4,'closed',$5,now())`,
		NewID("mc_"), workID, decision, reason, reviewer); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'moderation.decide','work',$3,$4)`,
		NewID("a_"), reviewer, workID, decision+":"+reason); err != nil {
		return "", err
	}
	return owner, tx.Commit()
}

func (s *Store) DecideReport(ctx context.Context, reportID, reviewer, decision string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var targetType, targetID string
	if err := tx.QueryRowContext(ctx, `
		SELECT target_type, target_id FROM reports WHERE id=$1`, reportID).Scan(&targetType, &targetID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE reports SET status=$2, reviewed_at=now() WHERE id=$1`, reportID, decision); err != nil {
		return err
	}
	if decision == "upheld" && targetType == "work" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE works SET status='removed', reject_reason='举报成立' WHERE id=$1`, targetID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'report.decide',$3,$4,$5)`,
		NewID("a_"), reviewer, targetType, targetID, decision); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DecideApplication(ctx context.Context, userID, reviewer, decision, reason string) error {
	status := "verified"
	if decision == "reject" {
		status = "rejected"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE musicians SET status=$2,
			verified_at=CASE WHEN $2='verified' THEN now() ELSE verified_at END,
			updated_at=now()
		WHERE user_id=$1`, userID, status); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'musician.decide','musician',$3,$4)`,
		NewID("a_"), reviewer, userID, decision+":"+reason); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DecideAppeal(ctx context.Context, appealID, reviewer, decision, reason string) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var workID, owner string
	if err := tx.QueryRowContext(ctx, `
		SELECT work_id, user_id FROM appeals WHERE id=$1`, appealID).Scan(&workID, &owner); err != nil {
		return "", err
	}
	status := "accepted"
	if decision == "reject" {
		status = "rejected"
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE appeals SET status=$2, reviewed_at=now() WHERE id=$1`, appealID, status); err != nil {
		return "", err
	}
	if decision == "accept" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE works SET status='published', reject_reason='', published_at=now() WHERE id=$1`, workID); err != nil {
			return "", err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'appeal.decide','appeal',$3,$4)`,
		NewID("a_"), reviewer, appealID, decision+":"+reason); err != nil {
		return "", err
	}
	return owner, tx.Commit()
}

func (s *Store) CreateAppeal(ctx context.Context, workID, userID, reason string) (string, error) {
	id := NewID("ap_")
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO appeals(id, work_id, user_id, reason) VALUES($1,$2,$3,$4)`,
		id, workID, userID, reason)
	return id, err
}

func (s *Store) ActiveFeedPolicy(ctx context.Context) (*model.FeedPolicy, error) {
	var p model.FeedPolicy
	var weights, diversity []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, version, weights, diversity, status, effective_at
		FROM feed_policies WHERE status='active' ORDER BY effective_at DESC LIMIT 1`).
		Scan(&p.ID, &p.Version, &weights, &diversity, &p.Status, &p.EffectiveAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.Weights = decodeMap(weights)
	p.Diversity = decodeMap(diversity)
	return &p, nil
}

func (s *Store) SaveFeedPolicy(ctx context.Context, reviewer, version string, weights, diversity map[string]any) (*model.FeedPolicy, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE feed_policies SET status='archived' WHERE status='active'`); err != nil {
		return nil, err
	}
	w, _ := json.Marshal(weights)
	d, _ := json.Marshal(diversity)
	id := NewID("fp_")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO feed_policies(id, version, weights, diversity, status, created_by)
		VALUES($1,$2,$3::jsonb,$4::jsonb,'active',$5)`,
		id, version, string(w), string(d), reviewer); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'feed_policy.publish','policy',$3,$4)`,
		NewID("a_"), reviewer, id, version); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ActiveFeedPolicy(ctx)
}

func (s *Store) TodayPublished(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM works WHERE status='published' AND published_at >= current_date`).Scan(&n)
	return n, err
}

func timeOrNow(t *time.Time) time.Time {
	if t == nil {
		return time.Now()
	}
	return *t
}
