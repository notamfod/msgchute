-- migrate:up
ALTER TABLE notification_stop_list ADD COLUMN subscriptions JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE notification_stop_list ADD COLUMN blocked_all BOOLEAN NOT NULL DEFAULT true;
UPDATE notification_stop_list SET subscriptions = CASE kind WHEN 'email' THEN '["email.order"]'::jsonb ELSE '["sms.order"]'::jsonb END, blocked_all = true;
ALTER TABLE messages ADD COLUMN tag VARCHAR(16) NOT NULL DEFAULT '';

-- migrate:down
ALTER TABLE messages DROP COLUMN tag;
ALTER TABLE notification_stop_list DROP COLUMN blocked_all;
ALTER TABLE notification_stop_list DROP COLUMN subscriptions;
