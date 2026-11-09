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

// ShareRewardCredits 是分享作品获得的积分奖励。
const ShareRewardCredits int64 = 5

// RewardShare 发放分享奖励：同一用户同一作品每天只奖励一次。
// 返回本次实际发放的积分，0 表示今天已经领过。
func (s *Store) RewardShare(ctx context.Context, tenantID, userID, workID, date string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO share_rewards(user_id, work_id, reward_date, credits)
		VALUES($1,$2,$3::date,$4)
		ON CONFLICT (user_id, work_id, reward_date) DO NOTHING`,
		userID, workID, date, ShareRewardCredits)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credit_accounts(tenant_id, available, reserved) VALUES($1,$2,0)
		ON CONFLICT (tenant_id) DO UPDATE SET available=credit_accounts.available+$2, updated_at=now()`,
		tenantID, ShareRewardCredits); err != nil {
		return 0, err
	}
	var balance int64
	if err := tx.QueryRowContext(ctx, `
		SELECT available FROM credit_accounts WHERE tenant_id=$1`, tenantID).Scan(&balance); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credit_ledger(id, tenant_id, entry_type, amount, balance_after, idempotency_key, ref_type, ref_id)
		VALUES($1,$2,'share_reward',$3,$4,$5,'work',$6)`,
		NewID("cl_"), tenantID, ShareRewardCredits, balance,
		"share:"+userID+":"+workID+":"+date, workID); err != nil {
		return 0, err
	}
	return ShareRewardCredits, tx.Commit()
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
