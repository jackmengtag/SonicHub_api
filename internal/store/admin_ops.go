package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrSelfReview        = errors.New("申请人不能自行复核，请由其他管理员审批")
	ErrInsufficientFunds = errors.New("账户可用积分不足")
	ErrInvalidState      = errors.New("当前状态不允许该操作")
)

/* ---------------- 用户管理 ---------------- */

type AdminUserItem struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Phone          string    `json:"phone"`
	Avatar         string    `json:"avatar"`
	Bio            string    `json:"bio"`
	Role           string    `json:"role"`
	Status         string    `json:"status"`
	Verified       bool      `json:"verified"`
	StageName      string    `json:"stage_name"`
	WorksCount     int       `json:"works_count"`
	FollowersCount int       `json:"followers_count"`
	CreatedAt      time.Time `json:"created_at"`
}

const adminUserSelect = `
	SELECT u.id, u.display_name, u.phone, u.avatar, u.bio, u.role, u.status,
	       COALESCE(m.status='verified',false), COALESCE(m.stage_name,''),
	       (SELECT count(*) FROM works w WHERE w.user_id=u.id),
	       (SELECT count(*) FROM relations r WHERE r.followee_id=u.id),
	       u.created_at
	FROM users u LEFT JOIN musicians m ON m.user_id=u.id`

func scanAdminUser(row interface{ Scan(...any) error }) (*AdminUserItem, error) {
	var u AdminUserItem
	err := row.Scan(&u.ID, &u.Name, &u.Phone, &u.Avatar, &u.Bio, &u.Role, &u.Status,
		&u.Verified, &u.StageName, &u.WorksCount, &u.FollowersCount, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

func (s *Store) ListAdminUsers(ctx context.Context, query, status string, limit int) ([]AdminUserItem, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if status == "normal" {
		status = "active"
	}
	rows, err := s.db.QueryContext(ctx, adminUserSelect+`
		WHERE ($1='' OR u.display_name ILIKE '%'||$1||'%' OR u.phone ILIKE '%'||$1||'%' OR u.id ILIKE '%'||$1||'%')
		  AND ($2='' OR u.status=$2)
		ORDER BY u.created_at DESC LIMIT $3`, query, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminUserItem{}
	for rows.Next() {
		u, err := scanAdminUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

func (s *Store) GetAdminUser(ctx context.Context, id string) (*AdminUserItem, error) {
	return scanAdminUser(s.db.QueryRowContext(ctx, adminUserSelect+` WHERE u.id=$1`, id))
}

type UserDisposition struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	Action       string    `json:"action"`
	Reason       string    `json:"reason"`
	OperatorID   string    `json:"operator_id"`
	OperatorName string    `json:"operator_name"`
	PrevStatus   string    `json:"prev_status"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Store) ListUserDispositions(ctx context.Context, userID string, limit int) ([]UserDisposition, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id, d.user_id, d.action, d.reason, d.operator_id, COALESCE(o.display_name,''), d.prev_status, d.created_at
		FROM user_dispositions d LEFT JOIN users o ON o.id=d.operator_id
		WHERE d.user_id=$1 ORDER BY d.created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserDisposition{}
	for rows.Next() {
		var d UserDisposition
		if err := rows.Scan(&d.ID, &d.UserID, &d.Action, &d.Reason, &d.OperatorID, &d.OperatorName, &d.PrevStatus, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DecideUser 处置用户账号，返回处置后的状态。
func (s *Store) DecideUser(ctx context.Context, userID, operatorID, action, reason string) (string, error) {
	next := map[string]string{"warn": "warned", "limit": "limited", "ban": "banned", "unban": "active"}[action]
	if next == "" {
		return "", ErrInvalidState
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	var prev, role string
	if err := tx.QueryRowContext(ctx, `SELECT status, role FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&prev, &role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	if role != "user" {
		return "", errors.New("不能处置后台账号")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET status=$2 WHERE id=$1`, userID, next); err != nil {
		return "", err
	}
	if action == "ban" {
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM relations WHERE follower_id=$1 OR followee_id=$1`, userID); err != nil {
			return "", err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_dispositions(id, user_id, action, reason, operator_id, prev_status)
		VALUES($1,$2,$3,$4,$5,$6)`,
		NewID("ud_"), userID, action, reason, operatorID, prev); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,$3,'user',$4,$5)`,
		NewID("a_"), operatorID, "user."+action, userID, prev+" -> "+next+":"+reason); err != nil {
		return "", err
	}
	return next, tx.Commit()
}

/* ---------------- 作品管理 ---------------- */

type AdminWorkItem struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Cover        string     `json:"cover"`
	Description  string     `json:"description"`
	OwnerID      string     `json:"owner_id"`
	Author       string     `json:"author"`
	Avatar       string     `json:"avatar"`
	Status       string     `json:"status"`
	Visibility   string     `json:"visibility"`
	RejectReason string     `json:"reject_reason"`
	Genre        string     `json:"genre"`
	Tags         []string   `json:"tags"`
	Duration     int        `json:"duration_seconds"`
	PlayCount    int        `json:"play_count"`
	LikeCount    int        `json:"like_count"`
	CommentCount int        `json:"comment_count"`
	ShareCount   int        `json:"share_count"`
	IsPinned     bool       `json:"is_pinned"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

func scanAdminWork(row interface{ Scan(...any) error }) (*AdminWorkItem, error) {
	var w AdminWorkItem
	var tags []byte
	var published sql.NullTime
	err := row.Scan(&w.ID, &w.Title, &w.Cover, &w.Description, &w.OwnerID, &w.Author, &w.Avatar,
		&w.Status, &w.Visibility, &w.RejectReason, &w.Genre, &tags, &w.Duration,
		&w.PlayCount, &w.LikeCount, &w.CommentCount, &w.ShareCount, &w.IsPinned, &published, &w.CreatedAt)
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

const adminWorkSelect = `
	SELECT w.id, w.title, w.cover, w.description, w.user_id, u.display_name, u.avatar,
	       w.status, w.visibility, w.reject_reason, w.genre, w.tags, w.duration_seconds,
	       w.play_count, w.like_count, w.comment_count, w.share_count, w.is_pinned,
	       w.published_at, w.created_at
	FROM works w JOIN users u ON u.id=w.user_id`

func (s *Store) ListAdminWorks(ctx context.Context, query, status, visibility string, limit int) ([]AdminWorkItem, error) {
	if limit <= 0 || limit > 300 {
		limit = 200
	}
	if status == "pending" {
		status = "pending_review"
	}
	rows, err := s.db.QueryContext(ctx, adminWorkSelect+`
		WHERE ($1='' OR w.title ILIKE '%'||$1||'%' OR w.id ILIKE '%'||$1||'%' OR u.display_name ILIKE '%'||$1||'%')
		  AND ($2='' OR w.status=$2)
		  AND ($3='' OR w.visibility=$3)
		ORDER BY w.is_pinned DESC, w.created_at DESC LIMIT $4`, query, status, visibility, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminWorkItem{}
	for rows.Next() {
		w, err := scanAdminWork(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

func (s *Store) GetAdminWork(ctx context.Context, id string) (*AdminWorkItem, error) {
	return scanAdminWork(s.db.QueryRowContext(ctx, adminWorkSelect+` WHERE w.id=$1`, id))
}

type AdminWorkDetail struct {
	Work        AdminWorkItem `json:"work"`
	OpenReports int           `json:"open_reports"`
	OpenAppeals int           `json:"open_appeals"`
}

func (s *Store) GetAdminWorkDetail(ctx context.Context, id string) (*AdminWorkDetail, error) {
	w, err := s.GetAdminWork(ctx, id)
	if err != nil {
		return nil, err
	}
	var d AdminWorkDetail
	d.Work = *w
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM reports WHERE target_type='work' AND target_id=$1 AND status='open'`, id).Scan(&d.OpenReports)
	_ = s.db.QueryRowContext(ctx, `SELECT count(*) FROM appeals WHERE work_id=$1 AND status='open'`, id).Scan(&d.OpenAppeals)
	return &d, nil
}

// DecideWorkAction 对作品执行下架/恢复/置顶/删除，返回作者与标题用于通知。
func (s *Store) DecideWorkAction(ctx context.Context, workID, actorID, action, reason string) (string, string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()

	var owner, title string
	if err := tx.QueryRowContext(ctx, `SELECT user_id, title FROM works WHERE id=$1 FOR UPDATE`, workID).Scan(&owner, &title); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", ErrNotFound
		}
		return "", "", err
	}

	var q string
	switch action {
	case "unpublish":
		q = `UPDATE works SET status='removed', reject_reason=$2 WHERE id=$1`
		_, err = tx.ExecContext(ctx, q, workID, reason)
	case "restore":
		q = `UPDATE works SET status='published', reject_reason='', published_at=COALESCE(published_at, now()) WHERE id=$1`
		_, err = tx.ExecContext(ctx, q, workID)
	case "pin":
		_, err = tx.ExecContext(ctx, `UPDATE works SET is_pinned=true WHERE id=$1`, workID)
	case "unpin":
		_, err = tx.ExecContext(ctx, `UPDATE works SET is_pinned=false WHERE id=$1`, workID)
	case "delete":
		_, err = tx.ExecContext(ctx, `DELETE FROM works WHERE id=$1`, workID)
	default:
		return "", "", ErrInvalidState
	}
	if err != nil {
		return "", "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,$3,'work',$4,$5)`,
		NewID("a_"), actorID, "work."+action, workID, reason); err != nil {
		return "", "", err
	}
	return owner, title, tx.Commit()
}

/* ---------------- 积分运营 ---------------- */

type CreditSummary struct {
	Accounts           int   `json:"accounts"`
	Circulating        int64 `json:"circulating"`
	Reserved           int64 `json:"reserved"`
	ConsumedToday      int64 `json:"consumed_today"`
	PendingAdjustments int   `json:"pending_adjustments"`
}

func (s *Store) AdminCreditSummary(ctx context.Context) (*CreditSummary, error) {
	var x CreditSummary
	err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM credit_accounts),
			COALESCE((SELECT sum(available) FROM credit_accounts),0),
			COALESCE((SELECT sum(reserved) FROM credit_accounts),0),
			COALESCE((SELECT -sum(amount) FROM credit_ledger WHERE amount<0 AND created_at>=current_date),0),
			(SELECT count(*) FROM credit_adjustments WHERE status='pending')`).
		Scan(&x.Accounts, &x.Circulating, &x.Reserved, &x.ConsumedToday, &x.PendingAdjustments)
	return &x, err
}

type AdminLedgerEntry struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	UserID       string    `json:"user_id"`
	UserName     string    `json:"user_name"`
	EntryType    string    `json:"entry_type"`
	Amount       int64     `json:"amount"`
	BalanceAfter int64     `json:"balance_after"`
	RefType      string    `json:"ref_type"`
	RefID        string    `json:"ref_id"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Store) ListAdminLedger(ctx context.Context, query string, limit int) ([]AdminLedgerEntry, error) {
	if limit <= 0 || limit > 300 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id, l.tenant_id, COALESCE(u.id,''), COALESCE(u.display_name,''),
		       l.entry_type, l.amount, l.balance_after, l.ref_type, l.ref_id, l.created_at
		FROM credit_ledger l
		LEFT JOIN LATERAL (
			SELECT m.user_id FROM memberships m WHERE m.tenant_id=l.tenant_id ORDER BY m.joined_at LIMIT 1
		) mm ON true
		LEFT JOIN users u ON u.id=mm.user_id
		WHERE ($1='' OR u.display_name ILIKE '%'||$1||'%' OR u.id ILIKE '%'||$1||'%'
		       OR l.ref_id ILIKE '%'||$1||'%' OR l.id ILIKE '%'||$1||'%')
		ORDER BY l.created_at DESC LIMIT $2`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminLedgerEntry{}
	for rows.Next() {
		var e AdminLedgerEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.UserID, &e.UserName, &e.EntryType, &e.Amount,
			&e.BalanceAfter, &e.RefType, &e.RefID, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type CreditAdjustment struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	UserName      string     `json:"user_name"`
	TenantID      string     `json:"tenant_id"`
	Amount        int64      `json:"amount"`
	Kind          string     `json:"kind"`
	Reason        string     `json:"reason"`
	Status        string     `json:"status"`
	RequesterID   string     `json:"requester_id"`
	RequesterName string     `json:"requester_name"`
	ReviewerID    string     `json:"reviewer_id"`
	ReviewerName  string     `json:"reviewer_name"`
	ReviewedAt    *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

const creditAdjustmentSelect = `
	SELECT a.id, a.user_id, COALESCE(u.display_name,''), a.tenant_id, a.amount, a.kind, a.reason, a.status,
	       a.requester_id, COALESCE(ru.display_name,''), a.reviewer_id, COALESCE(rv.display_name,''),
	       a.reviewed_at, a.created_at
	FROM credit_adjustments a
	LEFT JOIN users u ON u.id=a.user_id
	LEFT JOIN users ru ON ru.id=a.requester_id
	LEFT JOIN users rv ON rv.id=a.reviewer_id`

func scanCreditAdjustment(row interface{ Scan(...any) error }) (*CreditAdjustment, error) {
	var a CreditAdjustment
	var reviewed sql.NullTime
	err := row.Scan(&a.ID, &a.UserID, &a.UserName, &a.TenantID, &a.Amount, &a.Kind, &a.Reason, &a.Status,
		&a.RequesterID, &a.RequesterName, &a.ReviewerID, &a.ReviewerName, &reviewed, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.ReviewedAt = timePtr(reviewed)
	return &a, nil
}

func (s *Store) ListCreditAdjustments(ctx context.Context, status string, limit int) ([]CreditAdjustment, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, creditAdjustmentSelect+`
		WHERE ($1='' OR a.status=$1) ORDER BY a.created_at DESC LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CreditAdjustment{}
	for rows.Next() {
		a, err := scanCreditAdjustment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (s *Store) GetCreditAdjustment(ctx context.Context, id string) (*CreditAdjustment, error) {
	return scanCreditAdjustment(s.db.QueryRowContext(ctx, creditAdjustmentSelect+` WHERE a.id=$1`, id))
}

func (s *Store) CreateCreditAdjustment(ctx context.Context, requesterID, userID string, amount int64, kind, reason string) (*CreditAdjustment, error) {
	if amount == 0 {
		return nil, errors.New("积分数量不能为 0")
	}
	var tenantID string
	if err := s.db.QueryRowContext(ctx, `
		SELECT tenant_id FROM memberships WHERE user_id=$1 ORDER BY joined_at LIMIT 1`, userID).Scan(&tenantID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	id := NewID("ca_")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credit_adjustments(id, tenant_id, user_id, amount, kind, reason, status, requester_id)
		VALUES($1,$2,$3,$4,$5,$6,'pending',$7)`,
		id, tenantID, userID, amount, kind, reason, requesterID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'credit.request','adjustment',$3,$4)`,
		NewID("a_"), requesterID, id, reason); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetCreditAdjustment(ctx, id)
}

func (s *Store) DecideCreditAdjustment(ctx context.Context, id, reviewerID, decision, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var tenantID, status, requesterID string
	var amount int64
	if err := tx.QueryRowContext(ctx, `
		SELECT tenant_id, amount, status, requester_id
		FROM credit_adjustments WHERE id=$1 FOR UPDATE`, id).
		Scan(&tenantID, &amount, &status, &requesterID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if status != "pending" {
		return ErrInvalidState
	}
	if requesterID == reviewerID {
		return ErrSelfReview
	}

	next := "approved"
	if decision == "reject" {
		next = "rejected"
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE credit_adjustments SET status=$2, reviewer_id=$3, reviewed_at=now() WHERE id=$1`,
		id, next, reviewerID); err != nil {
		return err
	}
	if next == "approved" {
		var balance int64
		err := tx.QueryRowContext(ctx, `
			UPDATE credit_accounts SET available=available+$2, updated_at=now()
			WHERE tenant_id=$1 AND available+$2 >= 0
			RETURNING available`, tenantID, amount).Scan(&balance)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInsufficientFunds
		}
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO credit_ledger(id, tenant_id, entry_type, amount, balance_after, idempotency_key, ref_type, ref_id)
			VALUES($1,$2,'adjust',$3,$4,$5,'adjustment',$6)
			ON CONFLICT (idempotency_key) DO NOTHING`,
			NewID("cl_"), tenantID, amount, balance, "adjust:"+id, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,$3,'adjustment',$4,$5)`,
		NewID("a_"), reviewerID, "credit."+next, id, reason); err != nil {
		return err
	}
	return tx.Commit()
}

/* ---------------- 消息运营 ---------------- */

type Announcement struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Audience  string     `json:"audience"`
	Channels  []string   `json:"channels"`
	Link      string     `json:"link"`
	Status    string     `json:"status"`
	CreatedBy string     `json:"created_by"`
	Author    string     `json:"author"`
	SentAt    *time.Time `json:"sent_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

const announcementSelect = `
	SELECT a.id, a.title, a.body, a.audience, a.channels, a.link, a.status, a.created_by,
	       COALESCE(u.display_name,''), a.sent_at, a.created_at
	FROM announcements a LEFT JOIN users u ON u.id=a.created_by`

func scanAnnouncement(row interface{ Scan(...any) error }) (*Announcement, error) {
	var a Announcement
	var channels []byte
	var sent sql.NullTime
	err := row.Scan(&a.ID, &a.Title, &a.Body, &a.Audience, &channels, &a.Link, &a.Status, &a.CreatedBy,
		&a.Author, &sent, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.Channels = decodeStrings(channels)
	a.SentAt = timePtr(sent)
	return &a, nil
}

func (s *Store) ListAnnouncements(ctx context.Context, status string, limit int) ([]Announcement, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, announcementSelect+`
		WHERE ($1='' OR a.status=$1) ORDER BY a.created_at DESC LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Announcement{}
	for rows.Next() {
		a, err := scanAnnouncement(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (s *Store) CreateAnnouncement(ctx context.Context, actorID, title, body, audience, link string, channels []string) (*Announcement, error) {
	raw, _ := json.Marshal(channels)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	id := NewID("ann_")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO announcements(id, title, body, audience, channels, link, status, created_by)
		VALUES($1,$2,$3,$4,$5::jsonb,$6,'draft',$7)`,
		id, title, body, audience, string(raw), link, actorID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'message.draft','announcement',$3,$4)`,
		NewID("a_"), actorID, id, title); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return scanAnnouncement(s.db.QueryRowContext(ctx, announcementSelect+` WHERE a.id=$1`, id))
}

// SendAnnouncement 将公告发送给目标人群，返回触达人数。
func (s *Store) SendAnnouncement(ctx context.Context, id, actorID string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var title, body, audience, status string
	if err := tx.QueryRowContext(ctx, `
		SELECT title, body, audience, status FROM announcements WHERE id=$1 FOR UPDATE`, id).
		Scan(&title, &body, &audience, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}
	if status == "sent" {
		return 0, ErrInvalidState
	}

	userQuery := `SELECT id FROM users`
	if audience == "creators" {
		userQuery = `SELECT u.id FROM users u JOIN musicians m ON m.user_id=u.id WHERE m.status='verified'`
	} else if audience == "inactive" {
		userQuery = `SELECT id FROM users WHERE created_at < now() - interval '7 days'`
	}
	rows, err := tx.QueryContext(ctx, userQuery)
	if err != nil {
		return 0, err
	}
	ids := []string{}
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, uid)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, uid := range ids {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO notifications(id, user_id, type, actor_id, title, body, target_type, target_id, aggregate_key)
			VALUES($1,$2,'system',$3,$4,$5,'announcement',$6,$7)`,
			NewID("n_"), uid, actorID, title, body, id, "announce:"+id); err != nil {
			return 0, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE announcements SET status='sent', sent_at=now() WHERE id=$1`, id); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'message.send','announcement',$3,$4)`,
		NewID("a_"), actorID, id, title); err != nil {
		return 0, err
	}
	return len(ids), tx.Commit()
}

type MessageTemplate struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Scene     string    `json:"scene"`
	Channel   string    `json:"channel"`
	Content   string    `json:"content"`
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Store) ListMessageTemplates(ctx context.Context) ([]MessageTemplate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, scene, channel, content, enabled, updated_at
		FROM message_templates ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MessageTemplate{}
	for rows.Next() {
		var t MessageTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.Scene, &t.Channel, &t.Content, &t.Enabled, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetMessageTemplate(ctx context.Context, id string) (*MessageTemplate, error) {
	var t MessageTemplate
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, scene, channel, content, enabled, updated_at
		FROM message_templates WHERE id=$1`, id).
		Scan(&t.ID, &t.Name, &t.Scene, &t.Channel, &t.Content, &t.Enabled, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &t, err
}

func (s *Store) CreateMessageTemplate(ctx context.Context, actorID, name, scene, channel, content string, enabled bool) (*MessageTemplate, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	id := NewID("tpl_")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO message_templates(id, name, scene, channel, content, enabled)
		VALUES($1,$2,$3,$4,$5,$6)`, id, name, scene, channel, content, enabled); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'template.create','template',$3,$4)`,
		NewID("a_"), actorID, id, name); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetMessageTemplate(ctx, id)
}

func (s *Store) UpdateMessageTemplate(ctx context.Context, actorID, id, name, scene, channel, content string, enabled bool) (*MessageTemplate, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		UPDATE message_templates SET name=$2, scene=$3, channel=$4, content=$5, enabled=$6, updated_at=now()
		WHERE id=$1`, id, name, scene, channel, content, enabled)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'template.update','template',$3,$4)`,
		NewID("a_"), actorID, id, name); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetMessageTemplate(ctx, id)
}

/* ---------------- 广场运营 ---------------- */

type PlazaPlacement struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Position  string    `json:"position"`
	WorkID    string    `json:"work_id"`
	WorkTitle string    `json:"work_title"`
	WorkCover string    `json:"work_cover"`
	Schedule  string    `json:"schedule"`
	Status    string    `json:"status"`
	Exposure  int       `json:"exposure"`
	Clicks    int       `json:"clicks"`
	Owner     string    `json:"owner"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Store) ListPlazaPlacements(ctx context.Context) ([]PlazaPlacement, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.name, p.position, p.work_id, COALESCE(w.title,''), COALESCE(w.cover,''),
		       p.schedule, p.status, p.exposure, p.clicks, p.owner, p.created_at, p.updated_at
		FROM plaza_placements p LEFT JOIN works w ON w.id=p.work_id
		ORDER BY p.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlazaPlacement{}
	for rows.Next() {
		var p PlazaPlacement
		if err := rows.Scan(&p.ID, &p.Name, &p.Position, &p.WorkID, &p.WorkTitle, &p.WorkCover,
			&p.Schedule, &p.Status, &p.Exposure, &p.Clicks, &p.Owner, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetPlazaPlacement(ctx context.Context, id string) (*PlazaPlacement, error) {
	var p PlazaPlacement
	err := s.db.QueryRowContext(ctx, `
		SELECT p.id, p.name, p.position, p.work_id, COALESCE(w.title,''), COALESCE(w.cover,''),
		       p.schedule, p.status, p.exposure, p.clicks, p.owner, p.created_at, p.updated_at
		FROM plaza_placements p LEFT JOIN works w ON w.id=p.work_id WHERE p.id=$1`, id).
		Scan(&p.ID, &p.Name, &p.Position, &p.WorkID, &p.WorkTitle, &p.WorkCover,
			&p.Schedule, &p.Status, &p.Exposure, &p.Clicks, &p.Owner, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &p, err
}

func (s *Store) CreatePlazaPlacement(ctx context.Context, actorID, name, position, workID, schedule, status, owner string) (*PlazaPlacement, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	id := NewID("pl_")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO plaza_placements(id, name, position, work_id, schedule, status, owner)
		VALUES($1,$2,$3,$4,$5,$6,$7)`, id, name, position, workID, schedule, status, owner); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'plaza.placement.create','placement',$3,$4)`,
		NewID("a_"), actorID, id, name); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetPlazaPlacement(ctx, id)
}

func (s *Store) UpdatePlazaPlacement(ctx context.Context, actorID, id, name, position, workID, schedule, status, owner string) (*PlazaPlacement, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		UPDATE plaza_placements SET name=$2, position=$3, work_id=$4, schedule=$5, status=$6, owner=$7, updated_at=now()
		WHERE id=$1`, id, name, position, workID, schedule, status, owner)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'plaza.placement.update','placement',$3,$4)`,
		NewID("a_"), actorID, id, name+":"+status); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetPlazaPlacement(ctx, id)
}

type PlazaTopic struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	WorksCount   int       `json:"works_count"`
	Participants int       `json:"participants"`
	Schedule     string    `json:"schedule"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (s *Store) ListPlazaTopics(ctx context.Context) ([]PlazaTopic, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, works_count, participants, schedule, status, created_at, updated_at
		FROM plaza_topics ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlazaTopic{}
	for rows.Next() {
		var t PlazaTopic
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.WorksCount, &t.Participants,
			&t.Schedule, &t.Status, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetPlazaTopic(ctx context.Context, id string) (*PlazaTopic, error) {
	var t PlazaTopic
	err := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, works_count, participants, schedule, status, created_at, updated_at
		FROM plaza_topics WHERE id=$1`, id).
		Scan(&t.ID, &t.Name, &t.Description, &t.WorksCount, &t.Participants,
			&t.Schedule, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &t, err
}

func (s *Store) CreatePlazaTopic(ctx context.Context, actorID, name, description, schedule, status string) (*PlazaTopic, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	id := NewID("tp_")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO plaza_topics(id, name, description, schedule, status)
		VALUES($1,$2,$3,$4,$5)`, id, name, description, schedule, status); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'plaza.topic.create','topic',$3,$4)`,
		NewID("a_"), actorID, id, name); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetPlazaTopic(ctx, id)
}

func (s *Store) UpdatePlazaTopic(ctx context.Context, actorID, id, name, description, schedule, status string) (*PlazaTopic, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `
		UPDATE plaza_topics SET name=$2, description=$3, schedule=$4, status=$5, updated_at=now()
		WHERE id=$1`, id, name, description, schedule, status)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs(id, actor_id, action, target_type, target_id, detail)
		VALUES($1,$2,'plaza.topic.update','topic',$3,$4)`,
		NewID("a_"), actorID, id, name+":"+status); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetPlazaTopic(ctx, id)
}

type RankItem struct {
	Rank       int    `json:"rank"`
	ID         string `json:"id"`
	Title      string `json:"title"`
	Cover      string `json:"cover"`
	Author     string `json:"author"`
	Genre      string `json:"genre"`
	PlayCount  int    `json:"play_count"`
	LikeCount  int    `json:"like_count"`
	ShareCount int    `json:"share_count"`
	IsPinned   bool   `json:"is_pinned"`
}

func (s *Store) ListPlazaRanking(ctx context.Context, limit int) ([]RankItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT w.id, w.title, w.cover, u.display_name, w.genre, w.play_count, w.like_count, w.share_count, w.is_pinned
		FROM works w JOIN users u ON u.id=w.user_id
		WHERE w.status='published' AND w.visibility='public'
		ORDER BY w.play_count DESC, w.like_count DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RankItem{}
	for rows.Next() {
		var r RankItem
		if err := rows.Scan(&r.ID, &r.Title, &r.Cover, &r.Author, &r.Genre,
			&r.PlayCount, &r.LikeCount, &r.ShareCount, &r.IsPinned); err != nil {
			return nil, err
		}
		r.Rank = len(out) + 1
		out = append(out, r)
	}
	return out, rows.Err()
}

/* ---------------- 审计日志 ---------------- */

type AuditLogItem struct {
	ID         string    `json:"id"`
	ActorID    string    `json:"actor_id"`
	ActorName  string    `json:"actor_name"`
	Action     string    `json:"action"`
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	Detail     string    `json:"detail"`
	CreatedAt  time.Time `json:"created_at"`
}

const auditSelect = `
	SELECT a.id, a.actor_id, COALESCE(u.display_name,''), a.action, a.target_type, a.target_id, a.detail, a.created_at
	FROM audit_logs a LEFT JOIN users u ON u.id=a.actor_id`

func scanAudit(row interface{ Scan(...any) error }) (*AuditLogItem, error) {
	var x AuditLogItem
	err := row.Scan(&x.ID, &x.ActorID, &x.ActorName, &x.Action, &x.TargetType, &x.TargetID, &x.Detail, &x.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &x, err
}

func (s *Store) ListAuditLogs(ctx context.Context, query, kind string, limit int) ([]AuditLogItem, error) {
	if limit <= 0 || limit > 300 {
		limit = 150
	}
	where := ` WHERE ($1='' OR u.display_name ILIKE '%'||$1||'%' OR a.action ILIKE '%'||$1||'%'
		OR a.target_id ILIKE '%'||$1||'%' OR a.detail ILIKE '%'||$1||'%')`
	switch kind {
	case "review":
		where += ` AND (a.action LIKE 'moderation.%' OR a.action LIKE 'report.%' OR a.action LIKE 'appeal.%' OR a.action LIKE 'musician.%')`
	case "ops":
		where += ` AND (a.action LIKE 'feed_policy.%' OR a.action LIKE 'work.%' OR a.action LIKE 'message.%' OR a.action LIKE 'template.%' OR a.action LIKE 'plaza.%')`
	case "credit":
		where += ` AND a.action LIKE 'credit.%'`
	case "user":
		where += ` AND a.action LIKE 'user.%'`
	}
	rows, err := s.db.QueryContext(ctx, auditSelect+where+` ORDER BY a.created_at DESC LIMIT $2`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditLogItem{}
	for rows.Next() {
		x, err := scanAudit(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

func (s *Store) GetAuditLog(ctx context.Context, id string) (*AuditLogItem, error) {
	return scanAudit(s.db.QueryRowContext(ctx, auditSelect+` WHERE a.id=$1`, id))
}
