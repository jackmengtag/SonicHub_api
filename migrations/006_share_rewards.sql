CREATE TABLE IF NOT EXISTS share_rewards (
    user_id      TEXT NOT NULL,
    work_id      TEXT NOT NULL,
    reward_date  DATE NOT NULL,
    credits      INT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, work_id, reward_date)
);
CREATE INDEX IF NOT EXISTS share_rewards_date_idx ON share_rewards(reward_date);
