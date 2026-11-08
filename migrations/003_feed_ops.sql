CREATE TABLE IF NOT EXISTS feed_lists (
    id          TEXT PRIMARY KEY,
    list_type   TEXT NOT NULL,
    value       TEXT NOT NULL,
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (list_type, value)
);
CREATE INDEX IF NOT EXISTS feed_lists_type_idx ON feed_lists(list_type, created_at DESC);

CREATE TABLE IF NOT EXISTS feed_fallbacks (
    id          TEXT PRIMARY KEY,
    work_id     TEXT NOT NULL UNIQUE,
    sort_order  INT NOT NULL DEFAULT 0,
    created_by  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS feed_fallbacks_sort_idx ON feed_fallbacks(sort_order, created_at);
