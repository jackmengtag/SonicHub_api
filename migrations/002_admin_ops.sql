ALTER TABLE works ADD COLUMN IF NOT EXISTS is_pinned BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS user_dispositions (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id),
    action       TEXT NOT NULL,
    reason       TEXT NOT NULL DEFAULT '',
    operator_id  TEXT NOT NULL DEFAULT '',
    prev_status  TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS user_dispositions_user_idx ON user_dispositions(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS credit_adjustments (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    user_id       TEXT NOT NULL REFERENCES users(id),
    amount        BIGINT NOT NULL,
    kind          TEXT NOT NULL DEFAULT '客诉补偿',
    reason        TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'pending',
    requester_id  TEXT NOT NULL DEFAULT '',
    reviewer_id   TEXT NOT NULL DEFAULT '',
    reviewed_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS credit_adjustments_status_idx ON credit_adjustments(status, created_at DESC);

CREATE TABLE IF NOT EXISTS announcements (
    id          TEXT PRIMARY KEY,
    title       TEXT NOT NULL,
    body        TEXT NOT NULL DEFAULT '',
    audience    TEXT NOT NULL DEFAULT 'all',
    channels    JSONB NOT NULL DEFAULT '[]'::jsonb,
    link        TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'draft',
    created_by  TEXT NOT NULL DEFAULT '',
    sent_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS message_templates (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    scene       TEXT NOT NULL DEFAULT '',
    channel     TEXT NOT NULL DEFAULT '站内',
    content     TEXT NOT NULL DEFAULT '',
    enabled     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS plaza_placements (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    position    TEXT NOT NULL DEFAULT '',
    work_id     TEXT NOT NULL DEFAULT '',
    schedule    TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'scheduled',
    exposure    INT NOT NULL DEFAULT 0,
    clicks      INT NOT NULL DEFAULT 0,
    owner       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS plaza_topics (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    works_count   INT NOT NULL DEFAULT 0,
    participants  INT NOT NULL DEFAULT 0,
    schedule      TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'draft',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
