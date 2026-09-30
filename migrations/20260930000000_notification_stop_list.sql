-- migrate:up
CREATE TABLE notification_stop_list
(
    id UUID PRIMARY KEY,
    recipient VARCHAR(320) NOT NULL,
    kind VARCHAR(16) NOT NULL CHECK (kind IN ('email', 'phone')),
    reason VARCHAR(1000) NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (kind, recipient)
);

CREATE INDEX idx_notification_stop_list_recipient ON notification_stop_list (recipient);

-- migrate:down
DROP TABLE notification_stop_list;
