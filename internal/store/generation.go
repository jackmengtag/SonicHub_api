package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackmengtag/SonicHub_api/internal/model"
)

const generationColumns = `
	id, tenant_id, user_id, status, stage, progress, prompt, lyrics, styles, mood, duration_seconds, vocal,
	quality, stems, title, visibility, auto_publish, model_version, error, work_id,
	created_at, finished_at`

func scanGeneration(row interface{ Scan(...any) error }) (*model.Generation, error) {
	var g model.Generation
	var styles []byte
	var finished sql.NullTime
	err := row.Scan(
		&g.ID, &g.TenantID, &g.UserID, &g.Status, &g.Stage, &g.Progress, &g.Prompt, &g.Lyrics, &styles, &g.Mood,
		&g.Duration, &g.Vocal, &g.Quality, &g.Stems, &g.Title, &g.Visibility, &g.AutoPublish,
		&g.ModelVersion, &g.Error, &g.WorkID, &g.CreatedAt, &finished,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	g.Styles = decodeStrings(styles)
	g.FinishedAt = timePtr(finished)
	return &g, nil
}

type CreateGenerationInput struct {
	TenantID     string
	UserID       string
	QuoteID      string
	Prompt       string
	Lyrics       string
	Styles       []string
	Mood         string
	Duration     int
	Vocal        string
	Quality      string
	Stems        bool
	Title        string
	Visibility   string
	AutoPublish  bool
	ModelVersion string
}

func (s *Store) CreateGeneration(ctx context.Context, in CreateGenerationInput) (*model.Generation, error) {
	id := NewID("g_")
	styles, _ := json.Marshal(in.Styles)
	if in.ModelVersion == "" {
		in.ModelVersion = "bigmusic-0.9"
	}
	if in.Visibility == "" {
		in.Visibility = "public"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO generations(id, tenant_id, user_id, quote_id, status, stage, progress,
			prompt, lyrics, styles, mood, duration_seconds, vocal, quality, stems,
			title, visibility, auto_publish, model_version)
		VALUES($1,$2,$3,$4,'pending','pending',0,$5,$6,$7::jsonb,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		id, in.TenantID, in.UserID, in.QuoteID, in.Prompt, in.Lyrics, string(styles), in.Mood,
		in.Duration, in.Vocal, in.Quality, in.Stems, in.Title, in.Visibility, in.AutoPublish, in.ModelVersion)
	if err != nil {
		return nil, err
	}
	return s.GetGeneration(ctx, id, in.UserID)
}

// QueueGeneration 在积分冻结成功后把任务置为 queued，Worker 只会领取 queued 的任务。
func (s *Store) QueueGeneration(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE generations SET status='queued', stage='queued', updated_at=now()
		WHERE id=$1 AND status='pending'`, id)
	return err
}

func (s *Store) GetGeneration(ctx context.Context, id, userID string) (*model.Generation, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+generationColumns+` FROM generations WHERE id=$1 AND user_id=$2`, id, userID)
	return scanGeneration(row)
}

func (s *Store) ClaimGeneration(ctx context.Context) (*model.Generation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, `
		SELECT `+generationColumns+`
		FROM generations WHERE status='queued'
		ORDER BY created_at LIMIT 1 FOR UPDATE SKIP LOCKED`)
	g, err := scanGeneration(row)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE generations SET status='running', stage='generating', progress=12,
			started_at=now(), updated_at=now() WHERE id=$1`, g.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	g.Status = "running"
	g.Stage = "generating"
	g.Progress = 12
	return g, nil
}

func (s *Store) UpdateProgress(ctx context.Context, id, stage string, progress int) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE generations SET stage=$2, progress=$3, updated_at=now() WHERE id=$1`,
		id, stage, progress)
	return err
}

func (s *Store) CompleteGeneration(ctx context.Context, id, workID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE generations SET status='succeeded', stage='completed', progress=100,
			work_id=$2, finished_at=now(), updated_at=now() WHERE id=$1`, id, workID)
	return err
}

func (s *Store) FailGeneration(ctx context.Context, id, msg string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE generations SET status='failed', stage='failed', error=$2,
			finished_at=now(), updated_at=now() WHERE id=$1`, id, msg)
	return err
}

func (s *Store) CancelGeneration(ctx context.Context, id, userID string) (*model.Generation, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE generations SET status='cancelled', stage='cancelled', finished_at=now(), updated_at=now()
		WHERE id=$1 AND user_id=$2 AND status='queued'`, id, userID)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	g, err := s.GetGeneration(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return g, errors.New("only queued generations can be cancelled")
	}
	return g, nil
}

func (s *Store) RequeueStale(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE generations SET status='queued', stage='queued', progress=0, updated_at=now()
		WHERE status='running' AND started_at < now() - interval '15 minutes'`)
	return err
}

func (s *Store) LastGenerationForUser(ctx context.Context, userID string) (*model.Generation, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+generationColumns+` FROM generations
		WHERE user_id=$1 ORDER BY created_at DESC LIMIT 1`, userID)
	return scanGeneration(row)
}

func (s *Store) SaveGenerationWork(ctx context.Context, id, workID string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE generations SET work_id=$2, updated_at=now() WHERE id=$1`, id, workID)
	return err
}

func generationTime(t time.Time) time.Time { return t.UTC() }
