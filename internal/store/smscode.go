package store

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"time"
)

// ErrTooFrequent 表示同一手机号发送验证码过于频繁。
var ErrTooFrequent = errors.New("sms code requested too frequently")

// SaveSMSCode 保存自建验证码；同一号码 60 秒内只允许发送一次。
func (s *Store) SaveSMSCode(ctx context.Context, phone, code string, ttl time.Duration) error {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO sms_codes(phone, code, attempts, expires_at, sent_at)
		VALUES($1,$2,0,now() + make_interval(secs => $3::double precision), now())
		ON CONFLICT (phone) DO UPDATE
		SET code=EXCLUDED.code, attempts=0, expires_at=EXCLUDED.expires_at, sent_at=now()
		WHERE sms_codes.sent_at < now() - interval '60 seconds'`,
		phone, code, ttl.Seconds())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTooFrequent
	}
	return nil
}

// VerifySMSCode 校验自建验证码；成功后立即失效，最多尝试 5 次。
func (s *Store) VerifySMSCode(ctx context.Context, phone, code string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var stored string
	var attempts int
	var expires time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT code, attempts, expires_at FROM sms_codes WHERE phone=$1 FOR UPDATE`, phone).
		Scan(&stored, &attempts, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if time.Now().After(expires) || attempts >= 5 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sms_codes WHERE phone=$1`, phone); err != nil {
			return false, err
		}
		return false, tx.Commit()
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(code)) == 1 {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sms_codes WHERE phone=$1`, phone); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sms_codes SET attempts=attempts+1 WHERE phone=$1`, phone); err != nil {
		return false, err
	}
	return false, tx.Commit()
}
