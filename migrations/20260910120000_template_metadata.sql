-- migrate:up
SET LOCAL lock_timeout = '5s';
ALTER TABLE message_templates
    ADD COLUMN metadata JSONB,
    ADD COLUMN systems JSONB,
    ADD COLUMN channels JSONB;

-- migrate:down
SET LOCAL lock_timeout = '5s';
ALTER TABLE message_templates
    DROP COLUMN channels,
    DROP COLUMN systems,
    DROP COLUMN metadata;
