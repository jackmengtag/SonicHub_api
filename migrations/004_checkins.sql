CREATE TABLE IF NOT EXISTS checkins (
    user_id      TEXT NOT NULL,
    tenant_id    TEXT NOT NULL,
    checkin_date DATE NOT NULL,
    credits      INT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, checkin_date)
);
CREATE INDEX IF NOT EXISTS checkins_tenant_date_idx ON checkins(tenant_id, checkin_date DESC);
