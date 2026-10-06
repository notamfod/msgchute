-- migrate:up
ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS body_source VARCHAR(16) NOT NULL DEFAULT '';

-- migrate:down
ALTER TABLE messages
    DROP COLUMN IF EXISTS body_source;
