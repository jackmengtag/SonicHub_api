package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jackmengtag/SonicHub_api/internal/model"
)

// DailyCheckinCredits 是每日签到固定赠送的积分。
const DailyCheckinCredits int64 = 5

// Checkin 记录 userID 在 date 当天（YYYY-MM-DD）的签到，并赠送 credits 积分。
// 同一用户同一天重复签到不会重复发放积分。
func (s *Store) Checkin(ctx context.Context, tenantID, userID, date string, credits int64) (*model.CheckinResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var inserted string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO checkins(user_id, tenant_id, checkin_date, credits)
		VALUES($1,$2,$3::date,$4)
		ON CONFLICT (user_id, checkin_date) DO NOTHING
		RETURNING checkin_date::text`, userID, tenantID, date, credits).Scan(&inserted)

	already := errors.Is(err, sql.ErrNoRows)
	if err != nil && !already {
		return nil, err
	}

	gained := int64(0)
	if !already {
		gained = credits
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO credit_accounts(tenant_id, available, reserved) VALUES($1,$2,0)
			ON CONFLICT (tenant_id) DO UPDATE SET available=credit_accounts.available+$2, updated_at=now()`,
			tenantID, gained); err != nil {
			return nil, err
		}
		var balance int64
		if err := tx.QueryRowContext(ctx, `
			SELECT available FROM credit_accounts WHERE tenant_id=$1`, tenantID).Scan(&balance); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO credit_ledger(id, tenant_id, entry_type, amount, balance_after, idempotency_key, ref_type, ref_id)
			VALUES($1,$2,'checkin',$3,$4,$5,'checkin',$6)`,
			NewID("cl_"), tenantID, gained, balance, "checkin:"+tenantID+":"+date, date); err != nil {
			return nil, err
		}
		return &model.CheckinResult{Date: date, Already: false, Credits: gained, Balance: balance}, tx.Commit()
	}

	var balance int64
	if err := tx.QueryRowContext(ctx, `
		SELECT available FROM credit_accounts WHERE tenant_id=$1`, tenantID).Scan(&balance); err != nil {
		return nil, err
	}
	return &model.CheckinResult{Date: date, Already: true, Credits: 0, Balance: balance}, tx.Commit()
}

// CheckinStatus 返回用户在 date 当天是否已签到及当前积分余额。
func (s *Store) CheckinStatus(ctx context.Context, tenantID, userID, date string) (*model.CheckinStatus, error) {
	var checked bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM checkins WHERE user_id=$1 AND checkin_date=$2::date)`,
		userID, date).Scan(&checked); err != nil {
		return nil, err
	}
	var balance int64
	if err := s.db.QueryRowContext(ctx, `
		SELECT available FROM credit_accounts WHERE tenant_id=$1`, tenantID).Scan(&balance); err != nil {
		return nil, err
	}
	return &model.CheckinStatus{
		Date: date, CheckedIn: checked, CreditsPerDay: DailyCheckinCredits, Balance: balance,
	}, nil
}
