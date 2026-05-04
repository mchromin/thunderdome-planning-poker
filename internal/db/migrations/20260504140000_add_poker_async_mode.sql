-- +goose Up
-- +goose StatementBegin

-- Async mode for poker games
ALTER TABLE thunderdome.poker
    ADD COLUMN IF NOT EXISTS session_mode VARCHAR(16) NOT NULL DEFAULT 'sync',
    ADD COLUMN IF NOT EXISTS deadline TIMESTAMPTZ;

-- Per-user, per-story comments for async poker sessions
CREATE TABLE IF NOT EXISTS thunderdome.poker_story_comment (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    poker_id     UUID NOT NULL REFERENCES thunderdome.poker(id) ON DELETE CASCADE,
    story_id     UUID NOT NULL REFERENCES thunderdome.poker_story(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES thunderdome.users(id) ON DELETE CASCADE,
    comment      TEXT NOT NULL DEFAULT '',
    created_date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT poker_story_comment_unique UNIQUE (story_id, user_id)
);

CREATE INDEX IF NOT EXISTS poker_story_comment_story_idx
    ON thunderdome.poker_story_comment (story_id);
CREATE INDEX IF NOT EXISTS poker_story_comment_poker_idx
    ON thunderdome.poker_story_comment (poker_id);

-- Allow org/dept/team default to indicate session_mode
ALTER TABLE thunderdome.poker_settings
    ADD COLUMN IF NOT EXISTS session_mode VARCHAR(16) NOT NULL DEFAULT 'sync';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE thunderdome.poker_settings
    DROP COLUMN IF EXISTS session_mode;

DROP TABLE IF EXISTS thunderdome.poker_story_comment;

ALTER TABLE thunderdome.poker
    DROP COLUMN IF EXISTS deadline,
    DROP COLUMN IF EXISTS session_mode;

-- +goose StatementEnd
