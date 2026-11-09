package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackmengtag/SonicHub_api/internal/model"
)

var (
	ErrNotFound           = errors.New("not found")
	ErrInsufficientCredit = errors.New("insufficient credit")
)

// NewUserCredits 是新用户注册时赠送的初始积分。
const NewUserCredits int64 = 100

type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) DB() *sql.DB { return s.db }

func NewID(prefix string) string {
	return prefix + strings.ReplaceAll(uuid.NewString(), "-", "")
}

func decodeStrings(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		return []string{}
	}
	return out
}

func decodeMap(raw []byte) map[string]any {
	out := map[string]any{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func timePtr(v sql.NullTime) *time.Time {
	if v.Valid {
		t := v.Time
		return &t
	}
	return nil
}

func (s *Store) FindUserByPhone(ctx context.Context, phone string) (*model.User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, phone, display_name, avatar, bio, role, status, genres, created_at
		FROM users WHERE phone=$1`, phone)
	return scanUser(row)
}

func (s *Store) GetUser(ctx context.Context, id string) (*model.User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, phone, display_name, avatar, bio, role, status, genres, created_at
		FROM users WHERE id=$1`, id)
	return scanUser(row)
}

func scanUser(row interface{ Scan(...any) error }) (*model.User, error) {
	var u model.User
	var genres []byte
	err := row.Scan(&u.ID, &u.Phone, &u.DisplayName, &u.Avatar, &u.Bio, &u.Role, &u.Status, &genres, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.Genres = decodeStrings(genres)
	return &u, nil
}

type CreateUserResult struct {
	User     *model.User
	TenantID string
}

func (s *Store) CreateUser(ctx context.Context, phone, displayName string) (*CreateUserResult, error) {
	if displayName == "" {
		if len(phone) >= 4 {
			displayName = "用户" + phone[len(phone)-4:]
		} else {
			displayName = "新用户"
		}
	}
	avatars := []string{"c1", "c2", "c3", "c4", "c5", "c6", "c7", "c8"}
	avatar := avatars[len(phone)%len(avatars)]
	userID := NewID("u_")
	tenantID := NewID("t_")

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO users(id, phone, display_name, avatar, bio, role, status, genres)
		VALUES($1,$2,$3,$4,'','user','active','[]'::jsonb)`,
		userID, phone, displayName, avatar); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tenants(id, type, name, owner_user_id) VALUES($1,'personal',$2,$3)`,
		tenantID, displayName+"的空间", userID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO memberships(tenant_id, user_id, role) VALUES($1,$2,'owner')`, tenantID, userID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credit_accounts(tenant_id, available, reserved) VALUES($1,$2,0)`, tenantID, NewUserCredits); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credit_ledger(id, tenant_id, entry_type, amount, balance_after, idempotency_key, ref_type, ref_id)
		VALUES($1,$2,'grant',$3,$4,$5,'grant',$6)`,
		NewID("cl_"), tenantID, NewUserCredits, NewUserCredits, "grant:"+userID, userID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	u, err := s.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &CreateUserResult{User: u, TenantID: tenantID}, nil
}

func (s *Store) UpdateProfile(ctx context.Context, userID, displayName, bio string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET display_name=$2, bio=$3 WHERE id=$1`, userID, displayName, bio)
	return err
}

func (s *Store) SaveInterests(ctx context.Context, userID string, genres []string) error {
	b, _ := json.Marshal(genres)
	_, err := s.db.ExecContext(ctx, `UPDATE users SET genres=$2::jsonb WHERE id=$1`, userID, string(b))
	return err
}

func (s *Store) GetMusician(ctx context.Context, userID string) (*model.Musician, error) {
	var m model.Musician
	var genres []byte
	var verified sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT user_id, stage_name, status, bio, genres, applied_at, verified_at
		FROM musicians WHERE user_id=$1`, userID).
		Scan(&m.UserID, &m.StageName, &m.Status, &m.Bio, &genres, &m.AppliedAt, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.Genres = decodeStrings(genres)
	m.VerifiedAt = timePtr(verified)
	return &m, nil
}

func (s *Store) ApplyMusician(ctx context.Context, userID, stageName, bio string, genres []string) error {
	b, _ := json.Marshal(genres)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO musicians(user_id, stage_name, status, bio, genres, applied_at, updated_at)
		VALUES($1,$2,'pending',$3,$4::jsonb,now(),now())
		ON CONFLICT (user_id) DO UPDATE
		SET stage_name=EXCLUDED.stage_name, status='pending', bio=EXCLUDED.bio,
		    genres=EXCLUDED.genres, applied_at=now(), updated_at=now()`,
		userID, stageName, bio, string(b))
	return err
}

func (s *Store) TenantOf(ctx context.Context, userID string) (string, error) {
	var tenantID string
	err := s.db.QueryRowContext(ctx, `
		SELECT tenant_id FROM memberships WHERE user_id=$1 ORDER BY joined_at LIMIT 1`, userID).Scan(&tenantID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return tenantID, err
}

func (s *Store) CreateSession(ctx context.Context, id, userID, tenantID, hash string, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions(id, user_id, tenant_id, refresh_hash, expires_at)
		VALUES($1,$2,$3,$4,$5)`, id, userID, tenantID, hash, expires)
	return err
}

type Session struct {
	ID        string
	UserID    string
	TenantID  string
	ExpiresAt time.Time
	Revoked   sql.NullTime
}

func (s *Store) FindSession(ctx context.Context, hash string) (*Session, error) {
	var x Session
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, tenant_id, expires_at, revoked_at
		FROM sessions WHERE refresh_hash=$1`, hash).
		Scan(&x.ID, &x.UserID, &x.TenantID, &x.ExpiresAt, &x.Revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &x, err
}

func (s *Store) RevokeSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at=now() WHERE id=$1`, id)
	return err
}

func (s *Store) RevokeSessionByHash(ctx context.Context, hash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at=now() WHERE refresh_hash=$1`, hash)
	return err
}

/* ---------------- social ---------------- */

func (s *Store) Follow(ctx context.Context, follower, followee string) error {
	if follower == followee {
		return errors.New("cannot follow yourself")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO relations(follower_id, followee_id) VALUES($1,$2)
		ON CONFLICT DO NOTHING`, follower, followee)
	return err
}

func (s *Store) Unfollow(ctx context.Context, follower, followee string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM relations WHERE follower_id=$1 AND followee_id=$2`, follower, followee)
	return err
}

func (s *Store) IsFollowing(ctx context.Context, follower, followee string) bool {
	var ok bool
	_ = s.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM relations WHERE follower_id=$1 AND followee_id=$2)`,
		follower, followee).Scan(&ok)
	return ok
}

func (s *Store) IsFriend(ctx context.Context, a, b string) bool {
	var ok bool
	_ = s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM relations r1
			JOIN relations r2 ON r2.follower_id=r1.followee_id AND r2.followee_id=r1.follower_id
			WHERE r1.follower_id=$1 AND r1.followee_id=$2)`, a, b).Scan(&ok)
	return ok
}

func (s *Store) IsBlocked(ctx context.Context, a, b string) bool {
	var ok bool
	_ = s.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM blocks
			WHERE (user_id=$1 AND blocked_user_id=$2) OR (user_id=$2 AND blocked_user_id=$1))`,
		a, b).Scan(&ok)
	return ok
}

func (s *Store) Block(ctx context.Context, userID, blocked string) error {
	if userID == blocked {
		return errors.New("cannot block yourself")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO blocks(user_id, blocked_user_id) VALUES($1,$2) ON CONFLICT DO NOTHING`,
		userID, blocked); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM relations WHERE (follower_id=$1 AND followee_id=$2) OR (follower_id=$2 AND followee_id=$1)`,
		userID, blocked); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Unblock(ctx context.Context, userID, blocked string) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM blocks WHERE user_id=$1 AND blocked_user_id=$2`, userID, blocked)
	return err
}

const userCardSelect = `
	SELECT u.id, u.display_name, u.avatar, u.bio,
	       COALESCE(m.stage_name,''), COALESCE(m.status='verified',false),
	       EXISTS(SELECT 1 FROM relations r WHERE r.follower_id=$1 AND r.followee_id=u.id),
	       EXISTS(
	         SELECT 1 FROM relations r1
	         JOIN relations r2 ON r2.follower_id=r1.followee_id AND r2.followee_id=r1.follower_id
	         WHERE r1.follower_id=$1 AND r1.followee_id=u.id),
	       EXISTS(SELECT 1 FROM relations r WHERE r.follower_id=u.id AND r.followee_id=$1),
	       EXISTS(SELECT 1 FROM blocks b WHERE b.user_id=$1 AND b.blocked_user_id=u.id)
	FROM users u LEFT JOIN musicians m ON m.user_id=u.id`

func scanUserCard(row interface{ Scan(...any) error }) (*model.UserCard, error) {
	var c model.UserCard
	err := row.Scan(&c.ID, &c.Name, &c.Avatar, &c.Bio, &c.StageName, &c.Verified,
		&c.Following, &c.Friend, &c.Follower, &c.Blocked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

func (s *Store) GetUserCard(ctx context.Context, viewerID, targetID string) (*model.UserCard, error) {
	return scanUserCard(s.db.QueryRowContext(ctx, userCardSelect+` WHERE u.id=$2`, viewerID, targetID))
}

func (s *Store) listUsers(ctx context.Context, viewerID, joinWhere string, args ...any) ([]model.UserCard, error) {
	q := userCardSelect + " " + joinWhere
	rows, err := s.db.QueryContext(ctx, q, append([]any{viewerID}, args...)...)
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

func (s *Store) Following(ctx context.Context, viewerID, userID string) ([]model.UserCard, error) {
	return s.listUsers(ctx, viewerID, `JOIN relations r ON r.followee_id=u.id WHERE r.follower_id=$2 ORDER BY r.created_at DESC`, userID)
}

func (s *Store) Followers(ctx context.Context, viewerID, userID string) ([]model.UserCard, error) {
	return s.listUsers(ctx, viewerID, `JOIN relations r ON r.follower_id=u.id WHERE r.followee_id=$2 ORDER BY r.created_at DESC`, userID)
}

func (s *Store) Friends(ctx context.Context, viewerID, userID string) ([]model.UserCard, error) {
	return s.listUsers(ctx, viewerID, `
		WHERE EXISTS(
			SELECT 1 FROM relations r1
			JOIN relations r2 ON r2.follower_id=r1.followee_id AND r2.followee_id=r1.follower_id
			WHERE r1.follower_id=$2 AND r1.followee_id=u.id)
		ORDER BY u.display_name`, userID)
}

func (s *Store) Counts(ctx context.Context, userID string) (works, following, followers, friends int, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM works WHERE user_id=$1 AND status='published'),
			(SELECT count(*) FROM relations WHERE follower_id=$1),
			(SELECT count(*) FROM relations WHERE followee_id=$1),
			(SELECT count(*) FROM relations r1
			 JOIN relations r2 ON r2.follower_id=r1.followee_id AND r2.followee_id=r1.follower_id
			 WHERE r1.follower_id=$1)`, userID).
		Scan(&works, &following, &followers, &friends)
	return
}

func (s *Store) Notify(ctx context.Context, userID, typ, actorID, title, body, targetType, targetID, aggregateKey string) error {
	if userID == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO notifications(id, user_id, type, actor_id, title, body, target_type, target_id, aggregate_key)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		NewID("n_"), userID, typ, actorID, title, body, targetType, targetID, aggregateKey)
	return err
}

func (s *Store) Audit(ctx context.Context, actorID, action, targetType, targetID, detail string) {
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,$3,$4,$5,$6)`, NewID("a_"), actorID, action, targetType, targetID, detail)
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, q, args...)
}

func (s *Store) queryRow(ctx context.Context, q string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, q, args...)
}

func (s *Store) query(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, q, args...)
}

func wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", op, err)
}
