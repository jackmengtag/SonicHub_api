package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackmengtag/SonicHub_api/internal/model"
)

func (s *Store) Account(ctx context.Context, tenantID string) (model.CreditAccount, error) {
	var a model.CreditAccount
	err := s.db.QueryRowContext(ctx, `
		SELECT available, reserved FROM credit_accounts WHERE tenant_id=$1`, tenantID).
		Scan(&a.Available, &a.Reserved)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

func (s *Store) Ledger(ctx context.Context, tenantID string, limit int) ([]model.LedgerEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, entry_type, amount, balance_after, ref_type, ref_id, created_at
		FROM credit_ledger WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.LedgerEntry{}
	for rows.Next() {
		var e model.LedgerEntry
		if err := rows.Scan(&e.ID, &e.EntryType, &e.Amount, &e.BalanceAfter, &e.RefType, &e.RefID, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) Packages(ctx context.Context) ([]model.Package, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, credits, price_cents, tag FROM credit_packages ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Package{}
	for rows.Next() {
		var p model.Package
		if err := rows.Scan(&p.ID, &p.Credits, &p.PriceCents, &p.Tag); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) CreateQuote(ctx context.Context, tenantID, userID string, base, qualityFee, stemsFee, durationFee int, params any) (*model.Quote, error) {
	total := base + qualityFee + stemsFee + durationFee
	id := NewID("q_")
	expires := time.Now().Add(10 * time.Minute)
	raw, _ := json.Marshal(params)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO credit_quotes(id, tenant_id, user_id, base, quality_fee, stems_fee, duration_fee,
			total, params, status, expires_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,'open',$10)`,
		id, tenantID, userID, base, qualityFee, stemsFee, durationFee, total, string(raw), expires)
	if err != nil {
		return nil, err
	}
	return &model.Quote{
		ID: id, TenantID: tenantID, Base: base, QualityFee: qualityFee, StemsFee: stemsFee,
		DurationFee: durationFee, Total: total, Status: "open", ExpiresAt: expires,
	}, nil
}

func (s *Store) GetQuote(ctx context.Context, quoteID string) (*model.Quote, error) {
	var q model.Quote
	var expires time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, base, quality_fee, stems_fee, duration_fee, total, status, expires_at
		FROM credit_quotes WHERE id=$1`, quoteID).
		Scan(&q.ID, &q.TenantID, &q.Base, &q.QualityFee, &q.StemsFee, &q.DurationFee, &q.Total, &q.Status, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	q.ExpiresAt = expires
	return &q, err
}

func (s *Store) Reserve(ctx context.Context, generationID, quoteID, tenantID string, amount int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var available, reserved int64
	if err := tx.QueryRowContext(ctx, `
		SELECT available, reserved FROM credit_accounts WHERE tenant_id=$1 FOR UPDATE`, tenantID).
		Scan(&available, &reserved); err != nil {
		return err
	}
	if available < int64(amount) {
		return ErrInsufficientCredit
	}
	after := available - int64(amount)
	if _, err := tx.ExecContext(ctx, `
		UPDATE credit_accounts SET available=$2, reserved=reserved+$3, updated_at=now()
		WHERE tenant_id=$1`, tenantID, after, amount); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credit_reservations(id, quote_id, generation_id, tenant_id, amount, status)
		VALUES($1,$2,$3,$4,$5,'held')`, NewID("cr_"), quoteID, generationID, tenantID, amount); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credit_ledger(id, tenant_id, entry_type, amount, balance_after, idempotency_key, ref_type, ref_id)
		VALUES($1,$2,'reserve',$3,$4,$5,'generation',$6)`,
		NewID("cl_"), tenantID, -int64(amount), after, "reserve:"+generationID, generationID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE credit_quotes SET status='used' WHERE id=$1 AND status='open'`, quoteID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Capture(ctx context.Context, generationID, tenantID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var amount int
	var status string
	err = tx.QueryRowContext(ctx, `
		SELECT amount, status FROM credit_reservations WHERE generation_id=$1 FOR UPDATE`, generationID).
		Scan(&amount, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status == "captured" {
		return nil
	}
	if status != "held" {
		return errors.New("reservation is not held")
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE credit_accounts SET reserved=GREATEST(reserved-$2,0), updated_at=now()
		WHERE tenant_id=$1`, tenantID, amount); err != nil {
		return err
	}
	var available int64
	if err := tx.QueryRowContext(ctx, `
		SELECT available FROM credit_accounts WHERE tenant_id=$1`, tenantID).Scan(&available); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credit_ledger(id, tenant_id, entry_type, amount, balance_after, idempotency_key, ref_type, ref_id)
		VALUES($1,$2,'capture',0,$3,$4,'generation',$5)
		ON CONFLICT (idempotency_key) DO NOTHING`,
		NewID("cl_"), tenantID, available, "capture:"+generationID, generationID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE credit_reservations SET status='captured', updated_at=now() WHERE generation_id=$1`, generationID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Release(ctx context.Context, generationID, tenantID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var amount int
	var status string
	err = tx.QueryRowContext(ctx, `
		SELECT amount, status FROM credit_reservations WHERE generation_id=$1 FOR UPDATE`, generationID).
		Scan(&amount, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status == "released" {
		return nil
	}
	if status != "held" {
		return errors.New("reservation is not held")
	}
	var available int64
	if err := tx.QueryRowContext(ctx, `
		UPDATE credit_accounts SET available=available+$2, reserved=GREATEST(reserved-$2,0), updated_at=now()
		WHERE tenant_id=$1 RETURNING available`, tenantID, amount).Scan(&available); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credit_ledger(id, tenant_id, entry_type, amount, balance_after, idempotency_key, ref_type, ref_id)
		VALUES($1,$2,'release',$3,$4,$5,'generation',$6)
		ON CONFLICT (idempotency_key) DO NOTHING`,
		NewID("cl_"), tenantID, int64(amount), available, "release:"+generationID, generationID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE credit_reservations SET status='released', updated_at=now() WHERE generation_id=$1`, generationID); err != nil {
		return err
	}
	return tx.Commit()
}

// CreditsPerYuan 是充值换算比例：每 1 元获得 10 积分。
const CreditsPerYuan int64 = 10

// RechargeRewardPerYuan 是下单支付奖励：每消费 1 元额外奖励 1 积分。
const RechargeRewardPerYuan int64 = 1

func (s *Store) Recharge(ctx context.Context, tenantID, userID, packageID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var price int
	if err := tx.QueryRowContext(ctx, `
		SELECT price_cents FROM credit_packages WHERE id=$1`, packageID).
		Scan(&price); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	// 积分数量由金额决定，保证「1 元 = 10 积分」在充值链路上始终成立。
	credits := int(int64(price) * CreditsPerYuan / 100)
	// 下单支付奖励：每 1 元奖励 1 积分。
	bonus := int64(price) * RechargeRewardPerYuan / 100
	orderID := NewID("o_")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO orders(id, tenant_id, user_id, package_id, credits, amount_cents, status)
		VALUES($1,$2,$3,$4,$5,$6,'paid')`,
		orderID, tenantID, userID, packageID, credits, price); err != nil {
		return err
	}
	var available int64
	if _, err := tx.ExecContext(ctx, `
		UPDATE credit_accounts SET available=available+$2, updated_at=now()
		WHERE tenant_id=$1`, tenantID, credits); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `
		SELECT available FROM credit_accounts WHERE tenant_id=$1`, tenantID).Scan(&available); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO credit_ledger(id, tenant_id, entry_type, amount, balance_after, idempotency_key, ref_type, ref_id)
		VALUES($1,$2,'recharge',$3,$4,$5,'order',$6)`,
		NewID("cl_"), tenantID, int64(credits), available, "recharge:"+orderID, orderID); err != nil {
		return err
	}
	if bonus > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE credit_accounts SET available=available+$2, updated_at=now()
			WHERE tenant_id=$1`, tenantID, bonus); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `
			SELECT available FROM credit_accounts WHERE tenant_id=$1`, tenantID).Scan(&available); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO credit_ledger(id, tenant_id, entry_type, amount, balance_after, idempotency_key, ref_type, ref_id)
			VALUES($1,$2,'recharge_bonus',$3,$4,$5,'order',$6)`,
			NewID("cl_"), tenantID, bonus, available, "recharge_bonus:"+orderID, orderID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
