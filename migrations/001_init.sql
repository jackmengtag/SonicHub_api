CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    phone         TEXT NOT NULL UNIQUE,
    display_name  TEXT NOT NULL,
    avatar        TEXT NOT NULL DEFAULT 'c6',
    bio           TEXT NOT NULL DEFAULT '',
    role          TEXT NOT NULL DEFAULT 'user',
    status        TEXT NOT NULL DEFAULT 'active',
    genres        JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tenants (
    id             TEXT PRIMARY KEY,
    type           TEXT NOT NULL DEFAULT 'personal',
    name           TEXT NOT NULL,
    owner_user_id  TEXT NOT NULL REFERENCES users(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS memberships (
    tenant_id   TEXT NOT NULL REFERENCES tenants(id),
    user_id     TEXT NOT NULL REFERENCES users(id),
    role        TEXT NOT NULL DEFAULT 'owner',
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id)
);

CREATE TABLE IF NOT EXISTS sessions (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL REFERENCES users(id),
    tenant_id     TEXT NOT NULL,
    refresh_hash  TEXT NOT NULL UNIQUE,
    expires_at    TIMESTAMPTZ NOT NULL,
    revoked_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS musicians (
    user_id       TEXT PRIMARY KEY REFERENCES users(id),
    stage_name    TEXT NOT NULL UNIQUE,
    status        TEXT NOT NULL DEFAULT 'pending',
    bio           TEXT NOT NULL DEFAULT '',
    genres        JSONB NOT NULL DEFAULT '[]'::jsonb,
    applied_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    verified_at   TIMESTAMPTZ,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS relations (
    follower_id  TEXT NOT NULL REFERENCES users(id),
    followee_id  TEXT NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, followee_id),
    CHECK (follower_id <> followee_id)
);
CREATE INDEX IF NOT EXISTS relations_followee_idx ON relations(followee_id, created_at DESC);

CREATE TABLE IF NOT EXISTS blocks (
    user_id          TEXT NOT NULL REFERENCES users(id),
    blocked_user_id  TEXT NOT NULL REFERENCES users(id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, blocked_user_id)
);

CREATE TABLE IF NOT EXISTS works (
    id                TEXT PRIMARY KEY,
    tenant_id         TEXT NOT NULL,
    user_id           TEXT NOT NULL REFERENCES users(id),
    title             TEXT NOT NULL,
    cover             TEXT NOT NULL DEFAULT 'c1',
    description       TEXT NOT NULL DEFAULT '',
    visibility        TEXT NOT NULL DEFAULT 'public',
    status            TEXT NOT NULL DEFAULT 'draft',
    reject_reason     TEXT NOT NULL DEFAULT '',
    duration_seconds  INT NOT NULL DEFAULT 30,
    genre             TEXT NOT NULL DEFAULT '',
    tags              JSONB NOT NULL DEFAULT '[]'::jsonb,
    audio_url         TEXT NOT NULL DEFAULT '',
    play_count        INT NOT NULL DEFAULT 0,
    like_count        INT NOT NULL DEFAULT 0,
    comment_count     INT NOT NULL DEFAULT 0,
    share_count       INT NOT NULL DEFAULT 0,
    is_ai             BOOLEAN NOT NULL DEFAULT true,
    generation_id     TEXT,
    published_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS works_feed_idx ON works(status, visibility, published_at DESC);
CREATE INDEX IF NOT EXISTS works_owner_idx ON works(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS works_hot_idx ON works(status, visibility, like_count DESC);

CREATE TABLE IF NOT EXISTS assets (
    id                TEXT PRIMARY KEY,
    work_id           TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    kind              TEXT NOT NULL DEFAULT 'master',
    object_key        TEXT NOT NULL,
    content_type      TEXT NOT NULL DEFAULT 'audio/wav',
    duration_seconds  INT NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS work_likes (
    work_id     TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (work_id, user_id)
);

CREATE TABLE IF NOT EXISTS work_collects (
    work_id     TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (work_id, user_id)
);

CREATE TABLE IF NOT EXISTS comments (
    id          TEXT PRIMARY KEY,
    work_id     TEXT NOT NULL REFERENCES works(id) ON DELETE CASCADE,
    user_id     TEXT NOT NULL REFERENCES users(id),
    content     TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'published',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS comments_work_idx ON comments(work_id, created_at DESC);

CREATE TABLE IF NOT EXISTS credit_accounts (
    tenant_id   TEXT PRIMARY KEY,
    available   BIGINT NOT NULL DEFAULT 0,
    reserved    BIGINT NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS credit_quotes (
    id            TEXT PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    user_id       TEXT NOT NULL,
    base          INT NOT NULL,
    quality_fee   INT NOT NULL,
    stems_fee     INT NOT NULL,
    duration_fee  INT NOT NULL,
    total         INT NOT NULL,
    params        JSONB NOT NULL DEFAULT '{}'::jsonb,
    status        TEXT NOT NULL DEFAULT 'open',
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS credit_reservations (
    id             TEXT PRIMARY KEY,
    quote_id       TEXT NOT NULL UNIQUE,
    generation_id  TEXT NOT NULL UNIQUE,
    tenant_id      TEXT NOT NULL,
    amount         INT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'held',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS credit_ledger (
    id               TEXT PRIMARY KEY,
    tenant_id        TEXT NOT NULL,
    entry_type       TEXT NOT NULL,
    amount           BIGINT NOT NULL,
    balance_after    BIGINT NOT NULL,
    idempotency_key  TEXT NOT NULL UNIQUE,
    ref_type         TEXT NOT NULL DEFAULT '',
    ref_id           TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS credit_ledger_tenant_idx ON credit_ledger(tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS credit_packages (
    id          TEXT PRIMARY KEY,
    credits     INT NOT NULL,
    price_cents INT NOT NULL,
    tag         TEXT NOT NULL DEFAULT '',
    sort_order  INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS orders (
    id                 TEXT PRIMARY KEY,
    tenant_id          TEXT NOT NULL,
    user_id            TEXT NOT NULL,
    package_id         TEXT NOT NULL,
    credits            INT NOT NULL,
    amount_cents       INT NOT NULL,
    status             TEXT NOT NULL DEFAULT 'paid',
    provider_order_id  TEXT NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS generations (
    id                TEXT PRIMARY KEY,
    tenant_id         TEXT NOT NULL,
    user_id           TEXT NOT NULL,
    quote_id          TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'queued',
    stage             TEXT NOT NULL DEFAULT 'queued',
    progress          INT NOT NULL DEFAULT 0,
    prompt            TEXT NOT NULL DEFAULT '',
    lyrics            TEXT NOT NULL DEFAULT '',
    styles            JSONB NOT NULL DEFAULT '[]'::jsonb,
    mood              TEXT NOT NULL DEFAULT '',
    duration_seconds  INT NOT NULL DEFAULT 30,
    vocal             TEXT NOT NULL DEFAULT '女声',
    quality           TEXT NOT NULL DEFAULT '标准',
    stems             BOOLEAN NOT NULL DEFAULT false,
    title             TEXT NOT NULL DEFAULT '',
    visibility        TEXT NOT NULL DEFAULT 'public',
    auto_publish      BOOLEAN NOT NULL DEFAULT false,
    model_version     TEXT NOT NULL DEFAULT 'bigmusic-0.9',
    error             TEXT NOT NULL DEFAULT '',
    work_id           TEXT NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at        TIMESTAMPTZ,
    finished_at       TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS generations_queue_idx ON generations(status, created_at);

CREATE TABLE IF NOT EXISTS notifications (
    id             TEXT PRIMARY KEY,
    user_id        TEXT NOT NULL REFERENCES users(id),
    type           TEXT NOT NULL,
    actor_id       TEXT NOT NULL DEFAULT '',
    title          TEXT NOT NULL,
    body           TEXT NOT NULL DEFAULT '',
    target_type    TEXT NOT NULL DEFAULT '',
    target_id      TEXT NOT NULL DEFAULT '',
    aggregate_key  TEXT NOT NULL DEFAULT '',
    read_at        TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS notifications_user_idx ON notifications(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS conversations (
    id               TEXT PRIMARY KEY,
    last_message_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS conversation_members (
    conversation_id  TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id          TEXT NOT NULL REFERENCES users(id),
    unread_count     INT NOT NULL DEFAULT 0,
    muted            BOOLEAN NOT NULL DEFAULT false,
    pinned           BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (conversation_id, user_id)
);

CREATE TABLE IF NOT EXISTS messages (
    id               TEXT PRIMARY KEY,
    conversation_id  TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_id        TEXT NOT NULL,
    type             TEXT NOT NULL DEFAULT 'text',
    text             TEXT NOT NULL DEFAULT '',
    work_id          TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'sent',
    recalled_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS messages_conversation_idx ON messages(conversation_id, created_at DESC);

CREATE TABLE IF NOT EXISTS share_links (
    code         TEXT PRIMARY KEY,
    work_id      TEXT NOT NULL,
    sharer_id    TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'active',
    expires_at   TIMESTAMPTZ,
    open_count   INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS share_events (
    id          TEXT PRIMARY KEY,
    code        TEXT NOT NULL,
    channel     TEXT NOT NULL DEFAULT '',
    event_type  TEXT NOT NULL,
    actor_id    TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS moderation_cases (
    id              TEXT PRIMARY KEY,
    target_type     TEXT NOT NULL,
    target_id       TEXT NOT NULL,
    decision        TEXT NOT NULL,
    reason          TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'open',
    reviewer_id     TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at     TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS reports (
    id           TEXT PRIMARY KEY,
    reporter_id  TEXT NOT NULL,
    target_type  TEXT NOT NULL,
    target_id    TEXT NOT NULL,
    reason       TEXT NOT NULL,
    detail       TEXT NOT NULL DEFAULT '',
    status       TEXT NOT NULL DEFAULT 'open',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at  TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS appeals (
    id           TEXT PRIMARY KEY,
    work_id      TEXT NOT NULL,
    user_id      TEXT NOT NULL,
    reason       TEXT NOT NULL,
    status       TEXT NOT NULL DEFAULT 'open',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at  TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS feed_feedback (
    user_id      TEXT NOT NULL,
    target_type  TEXT NOT NULL,
    target_id    TEXT NOT NULL,
    type         TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, target_type, target_id, type)
);

CREATE TABLE IF NOT EXISTS feed_policies (
    id            TEXT PRIMARY KEY,
    version       TEXT NOT NULL,
    weights       JSONB NOT NULL DEFAULT '{}'::jsonb,
    diversity     JSONB NOT NULL DEFAULT '{}'::jsonb,
    status        TEXT NOT NULL DEFAULT 'active',
    effective_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by    TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS feed_impressions (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL DEFAULT '',
    work_id         TEXT NOT NULL,
    policy_version  TEXT NOT NULL DEFAULT '',
    reason          TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id          TEXT PRIMARY KEY,
    actor_id    TEXT NOT NULL DEFAULT '',
    action      TEXT NOT NULL,
    target_type TEXT NOT NULL DEFAULT '',
    target_id   TEXT NOT NULL DEFAULT '',
    detail      TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
