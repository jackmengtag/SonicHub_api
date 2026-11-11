CREATE TABLE IF NOT EXISTS sms_codes (
    phone       TEXT PRIMARY KEY,
    code        TEXT NOT NULL,
    attempts    INT NOT NULL DEFAULT 0,
    expires_at  TIMESTAMPTZ NOT NULL,
    sent_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
